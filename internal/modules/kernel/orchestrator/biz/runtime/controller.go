package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeController struct {
	repository  RuntimeStateRepository
	dispatch    *DispatchCoordinator
	transitions *RuntimeTransitionCoordinator
}

// CandidateAdmissionCommitResult couples the persisted aggregate with the
// expected, typed admission outcome. Callers must branch on Admission rather
// than parse an error message.
type CandidateAdmissionCommitResult struct {
	Aggregate RuntimeAggregate
	Admission CandidateAdmissionResult
}

func NewRuntimeController(repository RuntimeStateRepository) (*RuntimeController, error) {
	if repository == nil {
		return nil, common.ErrInvalidArgument
	}
	transitions, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		return nil, err
	}
	dispatch, err := newDispatchCoordinator(repository, transitions)
	if err != nil {
		return nil, err
	}
	return &RuntimeController{repository: repository, dispatch: dispatch, transitions: transitions}, nil
}

// initialize 建立一个 Run 唯一的 Runtime aggregate。
// 已初始化的 Run 只允许幂等读取，不会因为 Start/Resume/restart 再次执行 reducer。
func (c *RuntimeController) initialize(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtimecontract.RuntimeRequirement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	facts = facts.Normalized()
	if facts.CWILedgerSchemaVersion == "" {
		var ledgerErr error
		facts, ledgerErr = runtimecontract.InitializeRuntimeCWILedgerForActivation(facts)
		if ledgerErr != nil {
			return RuntimeAggregate{}, ledgerErr
		}
	}
	definition = definition.Normalized()
	if err := facts.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := definition.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if definition.Revision != facts.DefinitionRevision {
		return RuntimeAggregate{}, fmt.Errorf("runtime definition context revision does not match facts")
	}
	existing, err := c.repository.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err == nil {
		if !isTerminalConvergenceState(existing.Decision.State) {
			if err := runtimecontract.ValidateRuntimeOperationContractRevision(existing.Facts.Activation.ExecutionContract.OperationContractRevision); err != nil {
				return RuntimeAggregate{}, err
			}
			// The controller's direct callers do not own a persisted bootstrap.
			// Align their restart identity with the executable catalog only after
			// checking the persisted aggregate; this never writes a default into
			// existing authority.
			facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
			facts = facts.Normalized()
		}
		if !sameRuntimeDefinition(existing.Facts, facts) || !runtimecontract.SameRuntimeDefinitionContext(existing.Definition, definition) {
			return RuntimeAggregate{}, fmt.Errorf("%w: runtime state already exists with a different definition", common.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return RuntimeAggregate{}, err
	}
	// Bootstrap construction is the only point that seals the operation catalog
	// compiled into this binary. Existing persisted aggregates are never
	// defaulted or rewritten.
	facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	facts = facts.Normalized()
	facts = PreflightRuntimeFacts(facts, requirements)
	semanticPlanning, err := runtimecontract.NewRuntimeSemanticPlanningLedger(facts.DefinitionSeed)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate := RuntimeAggregate{
		StateRevision:    1,
		Definition:       definition,
		Facts:            facts,
		SemanticPlanning: semanticPlanning,
		Decision:         decision,
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, 0); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func isTerminalConvergenceState(state runtimecontract.ConvergenceState) bool {
	return state == runtimecontract.ConvergenceStateComplete || state == runtimecontract.ConvergenceStateFailed
}

// rejectRuntimeProgressAfterTermination fences every SM-2/SM-3 transition
// that could turn an already in-flight external response into new authority or
// a side effect after SM-1 sealed cancel/supersede. The response remains in
// provider/owner audit, but it cannot mutate the terminated aggregate.
func rejectRuntimeProgressAfterTermination(aggregate RuntimeAggregate) error {
	if aggregate.Termination == nil {
		return nil
	}
	return fmt.Errorf("%w: runtime administrative termination %s is already sealed", common.ErrConflict, aggregate.Termination.Kind)
}

func (c *RuntimeController) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.repository.Load(ctx, workspaceID, runID)
}

// reconcileAuthorityBeforeCandidate CAS-persists the deterministic SM-2
// cleanup required when a loaded aggregate still contains a non-current
// focus-scoped prerequisite/resource. It is intentionally narrower than
// candidate admission: no provider request, candidate attempt, repair budget,
// or owner operation is involved.
func (c *RuntimeController) reconcileAuthorityBeforeCandidate(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || isTerminalConvergenceState(aggregate.Decision.State) || len(InspectRuntimeAuthorityInvariants(aggregate).Violations) == 0 {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	// A provider ceiling is activation-sealed by bootstrap. If an owner resource
	// was cleared or became insufficient while its prerequisite still appears
	// active, return to the existing typed acquisition rather than allowing the
	// LLM boundary to discover the defect after an SM-3 attempt exists. Do not
	// synthesize a new/default requirement here; activation is the sole producer
	// of the sealed provider contract.
	facts := aggregate.Facts
	if requirements := activeProviderCeilingRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// recordFeasibilityFact CAS-persists the pre-candidate capability gate and
// immediately re-reduces. A missing operation is therefore a formal SM-2 fact,
// never a provider error or consumed candidate attempt.
func (c *RuntimeController) recordFeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.RuntimeFeasibilityFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || (aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange && aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept feasibility facts", common.ErrConflict)
	}
	fact.FocusID = strings.TrimSpace(fact.FocusID)
	fact.Category = runtimecontract.RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(fact.Category)))
	if fact.FocusID == "" {
		fact.FocusID = aggregate.Facts.Focus.ID
	}
	if fact.FocusID != aggregate.Facts.Focus.ID || fact.State != aggregate.Decision.State || fact.Reason == "" {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact does not match current authority", common.ErrConflict)
	}
	if fact.Category != "" && !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(fact.Category) {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact has an unknown owner category", common.ErrConflict)
	}
	if fact.Category == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable &&
		(fact.State != runtimecontract.ConvergenceStateNeedValidation || len(fact.FeasibleOperationKinds) != 0) {
		return RuntimeAggregate{}, fmt.Errorf("%w: validation recipe unavailability must block the current validation authority", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.Facts.Feasibility = &fact
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// recordFailureFact persists an SM-2 failure input and lets the reducer choose
// repair, governance, or terminal. Drive/adapter errors must never project a
// Run terminal without this authority transition.
func (c *RuntimeController) recordFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	return c.commitFailureFact(ctx, workspaceID, runID, fact, false)
}

// recordDriveFailureFact is the SM-4 error handoff. When a drive defect is
// observed while SM-3 still has an active dispatch, the same aggregate CAS
// invalidates that dispatch before SM-2 reduces the typed failure. This keeps
// generic failure recording narrow while ensuring Resume/restart never sees a
// new authority paired with the former dispatch transaction.
func (c *RuntimeController) recordDriveFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	return c.commitFailureFact(ctx, workspaceID, runID, fact, true)
}

func (c *RuntimeController) commitFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact, closeActiveDispatch bool) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || fact.Class == "" || strings.TrimSpace(fact.Message) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch != nil && !closeActiveDispatch {
		return RuntimeAggregate{}, fmt.Errorf("%w: active dispatch failures require a dispatch/receipt-specific transition", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	fact.Code = strings.TrimSpace(fact.Code)
	fact.Signature = strings.TrimSpace(fact.Signature)
	fact.Message = strings.TrimSpace(fact.Message)
	if aggregate.ActiveDispatch != nil {
		dispatch := *aggregate.ActiveDispatch
		dispatch.State = DispatchStateInvalidated
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
	}
	aggregate.Facts.Failure = &fact
	// A persisted FailureTarget is scoped to the current typed failure identity.
	// Re-recording a failure with a different signature/target set must return to
	// the existing local prerequisite, never leave the former mapping as an
	// implicit repair surface until a later drive happens to preflight it.
	facts := aggregate.Facts
	if requirements := activeFailureTargetRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	closePendingCandidateAttemptsForTerminal(&aggregate, fact)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func activeFailureTargetRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeRequirement {
	facts = facts.Normalized()
	values := make([]runtimecontract.RuntimeRequirement, 0, 1)
	seen := make(map[string]struct{})
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...) {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindFailureTarget {
			continue
		}
		requirement := runtimecontract.RuntimeRequirement{
			Kind: prerequisite.Kind, Scope: prerequisite.Scope, FocusID: prerequisite.FocusID,
			OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef,
			Reason: prerequisite.Reason,
		}
		key := runtimePrerequisiteID(requirement)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, requirement)
	}
	return normalizeRuntimeRequirements(values)
}

// requestTermination CAS-persists an administrative termination record on an active V2
// aggregate (INV-08). SM-1 may project cancelled/superseded only after this record exists.
func (c *RuntimeController) requestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || reason == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Termination != nil {
		if aggregate.Termination.Kind == kind {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: termination already recorded as %s", common.ErrConflict, aggregate.Termination.Kind)
	}
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept administrative termination", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	term := runtimecontract.RuntimeTerminationRecord{
		Kind: kind, RequestedBy: strings.TrimSpace(requestedBy), Reason: strings.TrimSpace(reason),
		Revision: expectedRevision + 1, RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := term.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Termination = &term
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) commitPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result PrerequisiteResult) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept prerequisite results", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	previousFocusID := aggregate.Facts.Focus.ID
	semanticCatalogRefreshPending := semanticCatalogPrerequisitePending(aggregate, result.PrerequisiteID)
	// A prerequisite ID is stable across re-acquisition. It is therefore not
	// sufficient to match only the pending ID here: a response started before a
	// durable resource change can otherwise satisfy the newly-pending instance
	// after that change invalidated the original observation. The persisted
	// invalidation records are the authoritative freshness fence; never accept
	// a result whose owner observation still reports the superseded revision.
	if err := validatePrerequisiteResultFreshness(result, aggregate.PrerequisiteInvalidations); err != nil {
		return RuntimeAggregate{}, err
	}
	facts, err := CommitPrerequisiteResult(aggregate.Facts, result)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	semanticGenerationRetired := false
	// A receipt-driven owner catalog change can invalidate the sealed semantic
	// generation even when the current focus ID remains unchanged (for example
	// a partial dispatch or a target refresh). Reopen semantic planning before
	// any candidate path can consume the reacquired facts; an active dispatch is
	// fenced by resetSemanticPlanningForInvalidatedCatalog itself.
	if aggregate.ActiveDispatch == nil && semanticCatalogRefreshPending {
		retired, resetErr := resetSemanticPlanningForInvalidatedCatalog(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated)
		if resetErr != nil {
			return RuntimeAggregate{}, resetErr
		}
		if !retired {
			// A prerequisite can be invalidated and reacquired without changing
			// the owner catalog identity. In that case the sealed generation is
			// still current and must remain reusable; only a remaining freshness
			// gap is a contract conflict.
			if aggregate.SemanticPlanning.Manifest != nil {
				if gaps, current := semanticPlanningCatalogsCurrent(aggregate, *aggregate.SemanticPlanning.Manifest); !current {
					return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog prerequisite was reacquired without retiring the stale generation (gaps=%d)", common.ErrConflict, len(gaps))
				}
			}
		}
		semanticGenerationRetired = retired
	}
	// A single-operation durable dispatch may have completed its current
	// coverage before its receipt invalidated mutable prerequisites. A changed
	// catalog must first re-seal the current binding and let SM-2 persist its
	// proof-lineage resolution; advancing under the retired binding would skip
	// the equivalent/incompatible gate. When re-acquisition retained the same
	// catalog identity, the existing exact binding remains current and ordinary
	// focus advancement is still valid.
	facts = aggregate.Facts
	if !semanticGenerationRetired && len(facts.Prerequisites.Pending) == 0 && len(facts.FreshnessReacquireCoverage) == 0 {
		advanced, advanceErr := advanceRuntimeCWIFocus(facts, aggregate.CoverageProofLineage)
		if advanceErr != nil {
			return RuntimeAggregate{}, advanceErr
		}
		facts = advanced
		aggregate.Facts = facts
	}
	if aggregate.Facts.Focus.ID != previousFocusID {
		if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
		facts = aggregate.Facts
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) beginCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate request binding fence: %v", common.ErrConflict, err)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	for _, record := range current {
		if record.Attempt != attempt {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStatePending && record.StablePrefixHash == stablePrefixHash && record.Transaction == transaction && record.ManifestHash == manifest.Hash && record.LogicalRequestID == logicalRequestID {
			return aggregate, nil
		}
		// Freshness invalidation does not consume the sealed attempt budget. If
		// prerequisite re-acquisition converges to the same stable authority
		// contract, recycle the persisted attempt 0 into a new pending proposal.
		// The separate prerequisite invalidation ledger retains the stale audit.
		if attempt == 0 && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.StablePrefixHash == stablePrefixHash {
			expectedRevision := aggregate.StateRevision
			for idx := range aggregate.CandidateAttempts {
				candidateAttempt := &aggregate.CandidateAttempts[idx]
				if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, *candidateAttempt) != contractID || candidateAttempt.Attempt != attempt {
					continue
				}
				candidateAttempt.Transaction = transaction
				candidateAttempt.StablePrefixHash = stablePrefixHash
				candidateAttempt.ManifestHash = manifest.Hash
				candidateAttempt.LogicalRequestID = logicalRequestID
				candidateAttempt.State = runtimecontract.RuntimeCandidateAttemptStatePending
				candidateAttempt.Proposal = nil
				candidateAttempt.Feasibilities = nil
				candidateAttempt.Failure = nil
				candidateAttempt.Stale = nil
				aggregate.StateRevision++
				if err := aggregate.Validate(); err != nil {
					return RuntimeAggregate{}, err
				}
				if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
					return RuntimeAggregate{}, err
				}
				return aggregate, nil
			}
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is already %s", common.ErrConflict, attempt, record.State)
	}
	if attempt == 0 {
		if len(current) != 0 {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
		}
	} else {
		first, ok := candidateAttemptRecord(current, 0)
		if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifest.Hash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed same-contract attempt 0", common.ErrConflict)
		}
		// N6: the attempt-1 CAS entry itself is the correction budget fence.
		// rejected_final / non-correctable attempt 0 must not open a second
		// provider session even when callers bypass mainline routing.
		if first.Failure == nil || !CandidateFailureIsCorrectableOnce(first.Failure) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a sealed correctable_once attempt 0 failure", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
	})
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// rejectCompletedCandidateDocument is the single SM-3 transition for a
// complete document that cannot become an executable candidate. It creates
// the attempt record and closes it in one aggregate CAS, so restart never
// observes a newly allocated but semantically meaningless pending attempt.
func (c *RuntimeController) rejectCompletedCandidateDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStateFailed {
			return aggregate, nil
		}
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, record) != contractID || record.State != runtimecontract.RuntimeCandidateAttemptStatePending {
			return RuntimeAggregate{}, fmt.Errorf("%w: completed candidate document attempt %d is already %s", common.ErrConflict, attempt, record.State)
		}
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	idx, _, pending := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !pending {
		if attempt == 0 {
			if len(current) != 0 {
				return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
			}
		} else {
			first, ok := candidateAttemptRecord(current, 0)
			if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifest.Hash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) || first.Failure == nil || !CandidateFailureIsCorrectableOnce(first.Failure) {
				return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed correctable same-contract attempt 0", common.ErrConflict)
			}
		}
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash,
			ManifestHash: manifest.Hash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
		})
		idx = len(aggregate.CandidateAttempts) - 1
	}
	expectedRevision := aggregate.StateRevision
	if err := closeCandidateAttempt(&aggregate, idx, attempt, failure); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// admitCandidateProposal atomically persists a candidate proposal or closes the
// pending attempt with its typed admission rejection. A restart therefore never
// observes a rejected provider candidate as an open SM-3 transaction.
func (c *RuntimeController) admitCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		if duplicate, found := duplicateRejectedCandidateAdmission(aggregate, attempt, manifest.Hash, logicalRequestID, candidate); found {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: duplicate}, nil
		}
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, "", manifest, logicalRequestID); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate proposal binding fence: %v", common.ErrConflict, err)
	}
	if record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate proposal does not match pending request-session binding", common.ErrConflict)
	}
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// A structurally valid rejected proposal is useful durable diagnostics and
		// allows the next attempt to explain exactly what was rejected. Invalid
		// candidates cannot be persisted because state validation must remain
		// schema-safe even after a failed attempt.
		if candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttempt(&aggregate, idx, attempt, *admission.Failure); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	if record.Proposal != nil {
		if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != runtimecontract.StableRuntimeCandidateHash(candidate) {
			return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate attempt proposal changed", common.ErrConflict)
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	expectedRevision := aggregate.StateRevision
	proposal := candidate
	aggregate.CandidateAttempts[idx].Proposal = &proposal
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// recordCandidateFeasibility persists one completed, side-effect-free owner
// probe. The operation must belong to the already persisted proposal, making
// this ledger a restart fence rather than a second candidate authority.
func (c *RuntimeController) recordCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeCandidateFeasibilityRecords([]runtimecontract.RuntimeCandidateFeasibilityRecord{record}); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, attemptRecord, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok || attemptRecord.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal", common.ErrConflict, attempt)
	}
	if !candidateProposalContainsOperation(*attemptRecord.Proposal, record.OperationID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q is absent from the persisted proposal", common.ErrConflict, record.OperationID)
	}
	if record.CandidateManifestHash != attemptRecord.ManifestHash {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q does not match the pending manifest", common.ErrConflict, record.OperationID)
	}
	for _, persisted := range attemptRecord.Feasibilities {
		if strings.TrimSpace(persisted.OperationID) != strings.TrimSpace(record.OperationID) {
			continue
		}
		if persisted.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || persisted.AcquisitionBinding != record.AcquisitionBinding || !reflect.DeepEqual(persisted.ReplayProof, record.ReplayProof) || !reflect.DeepEqual(persisted.Observation, record.Observation) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility result for operation %q changed", common.ErrConflict, record.OperationID)
		}
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	copyRecord := record
	if record.Observation != nil {
		observation := record.Observation.Clone()
		copyRecord.Observation = &observation
	}
	if record.ReplayProof != nil {
		proof := record.ReplayProof.Clone()
		copyRecord.ReplayProof = &proof
	}
	aggregate.CandidateAttempts[idx].Feasibilities = append(aggregate.CandidateAttempts[idx].Feasibilities, copyRecord)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func candidateProposalContainsOperation(candidate runtimecontract.RuntimeCandidate, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return false
	}
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			if value.Operation.Normalized().ID == wanted {
				return true
			}
		}
	}
	if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			if value.Operation.Normalized().ID == wanted {
				return true
			}
		}
	}
	return false
}

// admitAndSealCandidateAttempt is the final SM-3 admission fence. It atomically
// closes an expected rejection or atomically accepts the attempt and seals its
// dispatch. Feasibility/invariant defects remain ordinary errors because they
// are not provider-correctable candidate outcomes.
func (c *RuntimeController) admitAndSealCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		if duplicate, found := duplicateRejectedCandidateAdmission(aggregate, attempt, manifest.Hash, logicalRequestID, candidate); found {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: duplicate}, nil
		}
		if acceptedCandidateAdmissionDuplicate(aggregate, attempt, manifest.Hash, logicalRequestID, candidate) {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: acceptedCandidateAdmission()}, nil
		}
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, "", manifest, logicalRequestID); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate dispatch binding fence: %v", common.ErrConflict, err)
	}
	if record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate seal does not match pending request-session binding", common.ErrConflict)
	}
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// Final admission may be reached directly after a restart or alternate
		// owner probe. Preserve a schema-valid rejected candidate here too, so
		// duplicate delivery has the same durable identity as proposal admission.
		if candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttempt(&aggregate, idx, attempt, *admission.Failure); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, &record, &admission)
	if !completeAdmission.Admitted() {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: complete candidate admission %s: %s", common.ErrConflict, completeAdmission.Code, completeAdmission.Message)
	}
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if record.Transaction != dispatch.Transaction || aggregate.ActiveDispatch != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt does not match current dispatch state", common.ErrConflict)
	}
	if record.Proposal != nil && runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != runtimecontract.StableRuntimeCandidateHash(candidate) {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: accepted candidate does not match the persisted proposal", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// sealGovernanceDispatch seals the reducer-authored external dependency without
// creating a provider request or candidate attempt. Governance is an SM-2
// fact route, not a synthetic LLM correction path; therefore it must never
// consume the complete-document/session ledger used by SM-3 provider output.
func (c *RuntimeController) sealGovernanceDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime authority cannot seal governance", common.ErrConflict)
	}
	if candidate.Governance == nil || candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate does not match sealed authority", common.ErrConflict)
	}
	expectedDependency, found := GovernanceDependencyForAggregate(aggregate)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate has no sealed external dependency source", common.ErrConflict)
	}
	dependency := candidate.Governance.ExternalDependency.Normalized()
	if dependency != expectedDependency {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate dependency does not match the sealed source", common.ErrConflict)
	}
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, nil, nil)
	if !completeAdmission.Admitted() {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance dispatch admission %s: %s", common.ErrConflict, completeAdmission.Code, completeAdmission.Message)
	}
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// admitRuntimeDispatch is the only production dispatcher constructor. Provider
// candidates and reducer-authored governance facts have different source
// ledgers, but both cross the same manifest, active-dispatch and sealed-builder
// fence. It is pure and owner-local; callers commit the returned value only
// through the corresponding private internal event and one aggregate CAS.
func admitRuntimeDispatch(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, attempt *runtimecontract.RuntimeCandidateAttemptRecord, evaluated *CandidateAdmissionResult) (SealedDispatch, runtimecontract.RuntimeCompleteAdmissionDecision, error) {
	if aggregate.ActiveDispatch != nil {
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionStaleConflict,
			Code:        "active_dispatch_conflict",
			Message:     "runtime authority already has an active dispatch",
		}, nil
	}
	admission := evaluated
	if admission == nil {
		value := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
		admission = &value
	}
	if !admission.Accepted() {
		message := "runtime candidate rejected by manifest-bound admission"
		if admission.Failure != nil {
			message = admission.Failure.Reason
		}
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionCorrectableOnce,
			Code:        "candidate_document_rejected",
			Message:     message,
		}, nil
	}
	var feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord
	if attempt != nil {
		complete := runtimecontract.EvaluateRuntimeCompleteAdmission(runtimecontract.RuntimeCompleteAdmissionInput{
			Facts: aggregate.Facts, Decision: aggregate.Decision, SemanticPlanning: aggregate.SemanticPlanning,
			Reconciliation: aggregate.Reconciliation, Manifest: manifest, Candidate: candidate, Attempt: *attempt,
			ActiveDispatch:      sealedDispatchRecord(aggregate.ActiveDispatch),
			CompletedDispatches: sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches),
			OperationReceipts:   aggregate.OperationReceipts,
		})
		if !complete.Admitted() {
			return SealedDispatch{}, complete, nil
		}
		feasibilities = attempt.Feasibilities
	}
	dispatch, err := SealDispatchWithManifestAndFeasibilities(aggregate.Decision, candidate, aggregate.Facts.Activation, manifest, feasibilities)
	if err != nil {
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionTerminalContractFailure,
			Code:        "dispatch_seal_invalid",
			Message:     err.Error(),
		}, err
	}
	return dispatch, runtimecontract.RuntimeCompleteAdmissionDecision{Disposition: runtimecontract.RuntimeCompleteAdmissionAdmitted, Code: "admitted"}, nil
}

// GovernanceDependencyForAggregate returns the only dependency a governed
// dispatch may bind. It accepts facts that were explicitly sealed by an owner
// or prerequisite, plus the provider configuration fact whose subject is the
// sealed provider ceiling. It never invents a generic decision/focus fallback.
func GovernanceDependencyForAggregate(aggregate RuntimeAggregate) (runtimecontract.ExternalDependency, bool) {
	facts := aggregate.Facts.Normalized()
	// An operation-scoped governance receipt is the strongest source. Its exact
	// execution/fence binding is retained separately in the aggregate and later
	// rechecked during ticket resolution.
	for idx := len(aggregate.OperationGovernanceBindings) - 1; idx >= 0; idx-- {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		dependency := runtimecontract.ExternalDependency{
			Kind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
		}
		if facts.Failure != nil && facts.Failure.GovernanceDependency != nil {
			candidate := facts.Failure.GovernanceDependency.Normalized()
			if candidate.Kind != dependency.Kind || candidate.SubjectRef != dependency.SubjectRef {
				return runtimecontract.ExternalDependency{}, false
			}
			dependency.Reason = candidate.Reason
		}
		if dependency.Reason == "" && facts.Failure != nil {
			dependency.Reason = strings.TrimSpace(facts.Failure.Message)
		}
		dependency = dependency.Normalized()
		if dependency.Validate() == nil {
			return dependency, true
		}
		return runtimecontract.ExternalDependency{}, false
	}
	if facts.Failure != nil && facts.Failure.GovernanceDependency != nil {
		dependency := facts.Failure.GovernanceDependency.Normalized()
		if dependency.Validate() == nil {
			return dependency, true
		}
		return runtimecontract.ExternalDependency{}, false
	}
	if facts.Failure == nil || (facts.Failure.Class != runtimecontract.FailureClassProviderAuthentication && facts.Failure.Class != runtimecontract.FailureClassProviderConfiguration) || facts.Resources.ProviderCeiling == nil {
		return runtimecontract.ExternalDependency{}, false
	}
	target := facts.Resources.ProviderCeiling.Selection.Target.Normalized()
	if target.ProviderID == "" || target.ModelID == "" {
		return runtimecontract.ExternalDependency{}, false
	}
	reason := "provider configuration requires attention"
	if facts.Failure.Class == runtimecontract.FailureClassProviderAuthentication {
		reason = "provider authentication requires configuration"
	}
	return runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceConfiguration), SubjectRef: target.ProviderID + "/" + target.ModelID, Reason: reason,
	}, true
}

func sealedDispatchRecord(value *SealedDispatch) *runtimecontract.RuntimeDispatchRecord {
	if value == nil {
		return nil
	}
	record := sealedDispatchToRuntimeRecord(*value)
	return &record
}

// runtimeOperationAdmissibilityRequiredForOperation remains a package-local
// fixture seam. Production complete admission owns the rule in the pure
// runtime contract; do not add an Orchestrator-local variant here.
func runtimeOperationAdmissibilityRequiredForOperation(prerequisites runtimecontract.PrerequisiteState, ownerModuleID, operationKind string) bool {
	return runtimecontract.RuntimeOperationAdmissibilityRequiredForOperation(prerequisites, ownerModuleID, operationKind)
}

func runtimeOperationAdmissibilityForKind(values []runtimecontract.RuntimeOperationAdmissibility, owner, kind string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	canonicalKind := runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, inventory := range values {
		inventory = inventory.Normalized()
		if inventory.OwnerModuleID != owner {
			continue
		}
		for _, available := range inventory.OperationKinds {
			if available == canonicalKind {
				return inventory, true
			}
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func candidateAttemptFeasibilityForOperation(values []runtimecontract.RuntimeCandidateFeasibilityRecord, operationID string) (runtimecontract.RuntimeCandidateFeasibilityRecord, bool) {
	for _, value := range values {
		if strings.TrimSpace(value.OperationID) == strings.TrimSpace(operationID) {
			return value, true
		}
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{}, false
}

func sameStringSet(left, right []string) bool {
	left = normalizeStringSet(left)
	right = normalizeStringSet(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func normalizeStringSet(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	ret := make([]string, 0, len(set))
	for value := range set {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func (c *RuntimeController) failCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, _, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	if err := closeCandidateAttempt(&aggregate, idx, attempt, failure); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// closeCandidateAttempt applies the only failed exit from a pending SM-3
// candidate attempt. Its caller owns the aggregate CAS boundary.
func closeCandidateAttempt(aggregate *RuntimeAggregate, idx, attempt int, failure runtimecontract.RuntimeCandidateFailure) error {
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return common.ErrInvalidArgument
	}
	if failure.ValidationDiagnostic != nil {
		diagnostic := failure.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &diagnostic
	}
	if failure.Diagnostic != nil {
		diagnostic := failure.Diagnostic.Normalized()
		if diagnostic.ManifestHash == "" {
			diagnostic.ManifestHash = aggregate.CandidateAttempts[idx].ManifestHash
		}
		failure.Diagnostic = &diagnostic
	}
	// N4: every complete rejection is sealed as correctable_once or rejected_final
	// before it is persisted on the attempt record. Bare Class/Reason fixtures are
	// normalized to a matrix code first so attempt 0 schema/contract failures stay
	// correctable instead of becoming an internal terminal defect.
	if strings.TrimSpace(failure.Code) == "" {
		switch failure.Class {
		case runtimecontract.RuntimeCandidateFailureClassSchema:
			failure.Code = "candidate_schema_invalid"
		case runtimecontract.RuntimeCandidateFailureClassContract:
			failure.Code = "candidate_contract_invalid"
		case runtimecontract.RuntimeCandidateFailureClassInternal:
			failure.Code = "runtime_contract_defect"
		default:
			// provider/transient classes remain non-correctable finals below.
			failure.Code = strings.TrimSpace(string(failure.Class))
		}
	}
	if strings.TrimSpace(failure.Signature) == "" {
		failure.Signature = failure.Code
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: aggregate.CandidateAttempts[idx].ManifestHash}
	originalCode := strings.TrimSpace(failure.Code)
	originalClass := failure.Class
	if sealed, directive, sealErr := SealCandidateRejection(&failure, manifest); sealErr == nil && sealed != nil {
		// Attempt 0 must keep the first-failure code stable for diagnostics and
		// attempt-1 gating. Seal may rewrite an incomplete correctable into
		// runtime_contract_defect; that rewrite is only forced onto the attempt
		// record for attempt 1 / already-final codes. Attempt 0 still persists
		// objectives (or the lack of them) so IsCorrectableOnce can refuse retry.
		if attempt == 1 || sealed.Code == originalCode || originalCode == "runtime_contract_defect" ||
			CandidateFailureIsRejectedFinal(&runtimecontract.RuntimeCandidateFailure{Code: originalCode}) {
			failure = *sealed
		} else {
			failure.Diagnostic = sealed.Diagnostic
			if failure.Diagnostic == nil && sealed.Diagnostic != nil {
				failure.Diagnostic = sealed.Diagnostic.Clone()
			}
			// Preserve original class/code; empty objectives make attempt 1 refuse.
			_ = directive
			_ = originalClass
		}
	}
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	needsReduce := false
	if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
		needsReduce = true
	} else if attempt == 1 {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
			Signature: "candidate-retry-exhausted", Message: failure.Reason,
		}
		needsReduce = true
	} else if attempt == 0 && CandidateFailureIsRejectedFinal(&failure) {
		// N6: explicit rejected_final attempt 0 seals terminal in the same CAS.
		// Attempt 0 may still have one sealed correction.  A validation candidate
		// infeasibility never fabricates state-change repair authority; exhaustion
		// is terminalized by the normal candidate pipeline.
		code := CandidateRejectionTerminalCode(failure.Code)
		if code == "" {
			code = "runtime_contract_defect"
		}
		message := strings.TrimSpace(failure.Reason)
		if message == "" {
			message = "complete candidate rejection is rejected_final under the sealed manifest"
		}
		class := runtimecontract.FailureClassRuntimeContractMisalignment
		if code == "candidate_retry_exhausted" {
			class = runtimecontract.FailureClassCandidateExhausted
		}
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: class, Code: code, Signature: code, Message: message,
		}
		needsReduce = true
	}
	if needsReduce {
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
		if reduceErr != nil {
			return reduceErr
		}
		aggregate.Facts = facts
		aggregate.Decision = decision
	}
	return nil
}

// closePendingCandidateAttemptsForTerminal closes any in-flight SM-3 request
// when an SM-2/SM-4 failure fact has already sealed terminal authority. This is
// part of the same aggregate CAS: a terminal Run must never persist an open
// provider transaction that a restart could replay.
func closePendingCandidateAttemptsForTerminal(aggregate *RuntimeAggregate, fact runtimecontract.FailureFact) {
	if aggregate == nil || (!isTerminalConvergenceState(aggregate.Decision.State) && aggregate.Decision.Terminal == nil) {
		return
	}
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: strings.TrimSpace(fact.Code),
		Signature: strings.TrimSpace(fact.Signature), Reason: strings.TrimSpace(fact.Message),
	}
	if failure.Code == "" {
		failure.Code = "runtime_terminal_failure"
	}
	if failure.Signature == "" {
		failure.Signature = failure.Code
	}
	for idx := range aggregate.CandidateAttempts {
		if aggregate.CandidateAttempts[idx].State != runtimecontract.RuntimeCandidateAttemptStatePending {
			continue
		}
		failureCopy := failure
		aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
		aggregate.CandidateAttempts[idx].Failure = &failureCopy
	}
}

func candidateAttemptsForContract(values []runtimecontract.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string) []runtimecontract.RuntimeCandidateAttemptRecord {
	ret := make([]runtimecontract.RuntimeCandidateAttemptRecord, 0, 2)
	for _, value := range values {
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, value) == contractID {
			ret = append(ret, value)
		}
	}
	return ret
}

func candidateAttemptRecord(values []runtimecontract.RuntimeCandidateAttemptRecord, attempt int) (runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	for _, value := range values {
		if value.Attempt == attempt {
			return value, true
		}
	}
	return runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

func pendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string, attempt int) (int, runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	for idx, value := range values {
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, value) == contractID && value.Attempt == attempt && value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return idx, value, true
		}
	}
	return -1, runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

// duplicateRejectedCandidateAdmission identifies a completed expected rejection
// by its durable candidate/failure identity. It deliberately performs no write:
// a duplicated provider delivery must observe the original SM-3 result, not
// consume another attempt or reconstruct authority from the current decision.
func duplicateRejectedCandidateAdmission(aggregate RuntimeAggregate, attempt int, manifestHash, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionResult, bool) {
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	candidateShape := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateFailed || record.Failure == nil {
			continue
		}
		if record.Proposal != nil {
			if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != candidateHash &&
				!sameRejectedCandidateSemanticShape(record.Failure, candidateShape) {
				continue
			}
		} else if record.Failure.Code != candidateAdmissionCodeInvalid || candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil || record.Failure.Signature != candidateAdmissionInvalidSignature(candidate) {
			continue
		}
		failure := *record.Failure
		return CandidateAdmissionResult{Disposition: CandidateAdmissionRejectedContract, Failure: &failure}, true
	}
	return CandidateAdmissionResult{}, false
}

// sameRejectedCandidateSemanticShape recognizes an equivalent complete provider
// replay after SM-3 has already recorded its rejection. Provider operation IDs
// and JSON field order are wire details, whereas RejectedShapeHash retains the
// persisted semantic identity. The caller has already fenced this comparison by
// attempt, manifest, and logical request identity, so it cannot merge distinct
// authority transactions.
func sameRejectedCandidateSemanticShape(failure *runtimecontract.RuntimeCandidateFailure, candidateShape string) bool {
	if failure == nil || failure.Diagnostic == nil || strings.TrimSpace(candidateShape) == "" {
		return false
	}
	return strings.TrimSpace(failure.Diagnostic.RejectedShapeHash) == candidateShape
}

// acceptedCandidateAdmissionDuplicate proves that an already sealed candidate
// is the same delivery. The dispatch candidate hash is the durable seal fence.
func acceptedCandidateAdmissionDuplicate(aggregate RuntimeAggregate, attempt int, manifestHash, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) bool {
	wanted := runtimecontract.StableRuntimeCandidateHash(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Transaction == record.Transaction && aggregate.ActiveDispatch.CandidateHash == wanted {
			return true
		}
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.Transaction == record.Transaction && dispatch.CandidateHash == wanted {
				return true
			}
		}
	}
	return false
}

func (c *RuntimeController) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.dispatch == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return c.dispatch.commitEvidence(ctx, evidence)
}

// commitAssessment is invoked only by RuntimeInternalEventAssessmentObserved.
// It is deliberately private so external adapters cannot bypass the
// Orchestrator's single internal-event mutation boundary.
func (c *RuntimeController) commitAssessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Assessment == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime dispatch does not carry assessment", common.ErrConflict)
	}
	assessment := aggregate.ActiveDispatch.Candidate.Assessment.Normalized()
	claimedDisposition, err := runtimecontract.ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// The candidate disposition is a producer observation. Keep it intact for
	// audit/input validation, but never use it to select coverage, repair or a
	// Runtime decision below.
	assessment.Disposition = claimedDisposition
	// The entire assessment remains a scoped producer observation. It must not
	// be rewritten or rejected because its claimed evidence, item coverage, or
	// disposition disagrees with persisted facts; SM-2 establishes canonical
	// coverage from owner receipts below.
	assessment.CoverageWorkItemIDs = runtimeAssessmentCoverageIDs(assessment.CoverageWorkItemIDs)
	canonicalCoverage := applyCanonicalAssessmentCoverage(aggregate, aggregate.ActiveDispatch.ID)
	canonicalGap := canonicalAssessmentCoverageGap(canonicalCoverage, aggregate.Facts.Focus)
	canonicalDisposition := runtimecontract.AssessmentDispositionSatisfied
	if canonicalGap != nil {
		canonicalDisposition = runtimecontract.AssessmentDispositionRepairableGap
	}
	aggregate.Facts.Coverage = canonicalCoverage
	// Coverage credit and its canonical CWI convergence state are one SM-2
	// transaction. Current-schema projection permits exactly one CWI per focus,
	// so an accepted assessment cannot leave a satisfied CWI paired with an
	// unresolved sibling's focus or repair state.
	settledCoverage, settleErr := settleRuntimeCWIsForCurrentFocus(aggregate.Facts)
	if settleErr != nil {
		return RuntimeAggregate{}, settleErr
	}
	aggregate.Facts = settledCoverage
	expectedRevision := aggregate.StateRevision
	fact := runtimecontract.RuntimeAssessmentFact{
		ID: aggregate.ActiveDispatch.ID, FocusID: assessment.FocusID, SliceID: firstNonEmpty(assessment.SliceID, aggregate.Facts.Focus.SliceID),
		Scope: aggregate.Facts.Focus.AssessmentScope, RequirementID: aggregate.Facts.Focus.AssessmentID,
		RequirementText: aggregate.Facts.Focus.AssessmentText, CoverageWorkItemIDs: assessment.CoverageWorkItemIDs,
		ClaimedDisposition: assessment.Disposition, Disposition: canonicalDisposition, Items: append([]runtimecontract.RuntimeAssessmentItemFact(nil), assessment.Items...),
		Summary: assessment.Summary, EvidenceRefs: assessment.EvidenceRefs, ArtifactRefs: assessment.ArtifactRefs,
	}
	if assessment.Gap != nil {
		gap := assessment.Gap.Normalized()
		fact.Gap = &gap
	}
	aggregate.Facts.Assessments = append(aggregate.Facts.Assessments, fact)
	if canonicalDisposition == runtimecontract.AssessmentDispositionRepairableGap {
		priorGap := aggregate.Facts.Repair.ActiveGap
		// The producer's gap stays intact in AssessmentFact for audit. SM-2's
		// ledger receives only an owner-derived coverage/evidence classification:
		// no claimed disposition, missing kind, target, reason or free text can
		// alter its budget, closure or next action.
		signature := runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, canonicalGap)
		canonicalGap.FailureSignature = signature
		cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, fact.ID)
		ledger, _, cycleErr := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
		if cycleErr != nil {
			return RuntimeAggregate{}, cycleErr
		}
		ledger.ActiveGap = canonicalGap
		// A repairable assessment only resets the same-focus progress windows
		// when its persisted neutral gap set strictly narrows. The comparison
		// itself is a pure contract helper; SM-2 alone decides to apply it.
		// Repeated or expanded gaps retain their signatures/shapes and must not
		// create a fresh retry window merely because an assessment was accepted.
		if priorGap != nil && runtimecontract.RuntimeAssessmentGapStrictlyNarrows(*priorGap, *canonicalGap) {
			ledger = noteRepairGapProgress(ledger, *canonicalGap, aggregate.Facts.EvidenceRevision)
		} else {
			// An accepted operation is not harmless merely because its receipt
			// succeeded. Once owner-derived coverage reports the same (or a wider)
			// gap, retain every concrete current-focus operation shape that led
			// here. Candidate admission consumes this ledger on restart as well.
			ledger = recordAcceptedNoProgressShapes(aggregate, ledger, signature)
		}
		aggregate.Facts.Repair = ledger
	}
	if canonicalDisposition == runtimecontract.AssessmentDispositionSatisfied {
		if hasStateChangeNoOpWithoutProvenance(aggregate.Facts) {
			// A satisfied assessment can establish that the observed current state
			// meets this focus, but it must not erase the distinct no-op provenance
			// fact before the reducer consumes it. Retaining the exact rejected
			// operation shape makes the completion branch restart-safe and prevents
			// a later raw missing-state-change fallback from manufacturing a delta.
			aggregate.Facts.Repair = noteRepairProgressPreservingNoOpProvenance(aggregate.Facts.Repair, aggregate.Facts.EvidenceRevision)
		} else {
			aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, aggregate.Facts.EvidenceRevision)
		}
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	previousFocusID := aggregate.Facts.Focus.ID
	advanced, advanceErr := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
	if advanceErr != nil {
		return RuntimeAggregate{}, advanceErr
	}
	aggregate.Facts = advanced
	if aggregate.Facts.Focus.ID != previousFocusID {
		// Reaching an unbound placeholder is a local SM-2 transition, not a
		// continuation chosen by the assessment. Retire only the just-sealed
		// batch working state so SM-4 can request the next scoped proof batch.
		if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
	}
	facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Validation-only accounting belongs to the canonical SM-2 result, never
	// to an assessment candidate's claimed missing evidence kind. Increment
	// after the budget fence selected the action so the next same-focus gap
	// observes the persisted retry that was actually authorized.
	if canonicalDisposition == runtimecontract.AssessmentDispositionRepairableGap && nextDecision.State == runtimecontract.ConvergenceStateNeedValidation {
		facts.Repair = markValidationOnlyRetry(facts.Repair)
	}
	aggregate.Facts = facts
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func recordAcceptedNoProgressShapes(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeRepairLedger, signature string) runtimecontract.RuntimeRepairLedger {
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if !runtimeEvidenceAppliesToCurrentCoverageWithLineage(aggregate.Facts, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		if evidence.Kind != runtimecontract.EvidenceKindStateChange {
			// A repair closure deliberately requires a fresh validation after its
			// durable effect. Keeping a prior validation shape in the generic
			// anti-replay ledger would reject that mandatory follow-up before SM-3
			// can seal it. State-change shapes remain replay-protected here; the
			// directive's evidence barrier governs validation freshness instead.
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, evidence)
		if !found {
			continue
		}
		ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
			Operation:        operation,
			EvidenceID:       evidence.ID,
			FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:      "accepted_operation_same_gap",
			FailureSignature: signature,
			FailureMessage:   "accepted operation did not reduce the persisted assessment gap",
		})
	}
	return ledger
}

func runtimeAssessmentCoverageIDs(ids []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	seen := map[common.CoverageWorkItemID]struct{}{}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	return ret
}

// applyCanonicalAssessmentCoverage closes coverage only from persisted,
// owner-confirmed evidence already bound to the current focus/item. Assessment
// candidate disposition, item satisfaction, missing kinds, targets and prose
// are deliberately absent from this classifier: they remain in the committed
// RuntimeAssessmentFact as producer observation/audit lineage.
func applyCanonicalAssessmentCoverage(aggregate RuntimeAggregate, assessmentID string) []runtimecontract.RuntimeCoverageItemFact {
	coverage := append([]runtimecontract.RuntimeCoverageItemFact(nil), aggregate.Facts.Coverage...)
	focus := aggregate.Facts.Focus.Normalized()
	for idx := range coverage {
		item := &coverage[idx]
		if item.SliceID != "" && focus.SliceID != "" && item.SliceID != focus.SliceID {
			continue
		}
		if !coverageWorkItemIDIncluded(focus.CoverageWorkItemIDs, item.ID) || item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, item.ID)
		item.LastAssessmentID = assessmentID
		item.EvidenceRefs = append(item.EvidenceRefs, refs...)
		if satisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateSatisfied
		} else {
			item.State = runtimecontract.RuntimeCoverageItemStateUnsatisfied
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

func canonicalAssessmentEvidenceForCoverage(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID) ([]string, bool) {
	focus := aggregate.Facts.Focus.Normalized()
	binding, bound := coverageEvidenceBindingForCoverage(focus, coverageID)
	// Current-schema bootstrap and state validation require a binding for every
	// CWI that may receive credit. An unbound focus can retain its action-level
	// requirements for diagnostic/repair presentation, but cannot close coverage.
	needed := stringSet(focus.RequiredEvidenceKinds)
	if bound {
		needed = stringSet(binding.EvidenceKinds())
	}
	seenKinds := make(map[string]struct{}, len(needed))
	refs := make([]string, 0)
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if weakRuntimeEvidence(evidence) ||
			!runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, evidence, coverageID, aggregate.CoverageProofLineage) {
			continue
		}
		if !runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, focus, coverageID, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		if bound || assessmentFocusRequiresOperationEvidence(focus) {
			operation, found := runtimeOperationForEvidence(aggregate, evidence)
			if !found || !runtimeOperationAllowedForFocus(focus, operation) {
				continue
			}
			if focus.RequireDirectEvidence && strings.TrimSpace(evidence.Kind) == "" {
				continue
			}
		}
		refs = append(refs, string(evidence.ID))
		seenKinds[strings.TrimSpace(evidence.Kind)] = struct{}{}
	}
	if len(refs) == 0 {
		return nil, false
	}
	for kind := range needed {
		if _, ok := seenKinds[kind]; !ok {
			return nil, false
		}
	}
	// A binding can deliberately require a durable state-change receipt and a
	// validation receipt. The former proves the effect while the latter proves
	// executable behavior. Evaluate their complete proof set together; judging
	// every individual receipt as an executable behavior proof would reject the
	// state receipt by construction and make a valid conjunction impossible.
	if canonicalAssessmentEvidenceQualityFailure(aggregate, coverageID, refs) != "" {
		return nil, false
	}
	return refs, true
}

func coverageEvidenceBindingForCoverage(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	for _, binding := range focus.Normalized().CoverageEvidenceBindings {
		if binding.CoverageWorkItemID == coverageID {
			return binding.Clone(), true
		}
	}
	return runtimecontract.CoverageEvidenceBinding{}, false
}

func canonicalAssessmentCoverageGap(coverage []runtimecontract.RuntimeCoverageItemFact, focus runtimecontract.CoverageFocus) *runtimecontract.RuntimeAssessmentGap {
	open := openCoverageIDs(coverage, focus.CoverageWorkItemIDs)
	if len(open) == 0 {
		return nil
	}
	return &runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs: open,
		FailureClass:        runtimecontract.FailureClassCoverageAssessmentIncomplete,
		FailureCode:         "assessment_coverage_open",
	}
}

func openCoverageIDs(coverage []runtimecontract.RuntimeCoverageItemFact, ids []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	state := map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemState{}
	for _, item := range coverage {
		state[item.ID] = item.State
	}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if state[id] != runtimecontract.RuntimeCoverageItemStateSatisfied {
			ret = append(ret, id)
		}
	}
	return ret
}

func sameRuntimeDefinition(left runtimecontract.RuntimeFacts, right runtimecontract.RuntimeFacts) bool {
	left = left.Normalized()
	right = right.Normalized()
	// Focus is the persisted SM-2 cursor and legitimately changes as coverage
	// advances. Restart identity is limited to immutable run definition and
	// activation fields; comparing the current cursor with bootstrap focus 0
	// would reject every multi-focus run after its first atomic advance.
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.DefinitionRevision == right.DefinitionRevision &&
		runtimecontract.SameRuntimeActivationContext(left.Activation, right.Activation)
}

func sealedDispatchesToRuntimeRecords(values []SealedDispatch) []runtimecontract.RuntimeDispatchRecord {
	ret := make([]runtimecontract.RuntimeDispatchRecord, 0, len(values))
	for _, value := range values {
		ret = append(ret, sealedDispatchToRuntimeRecord(value))
	}
	return ret
}

func sealedDispatchToRuntimeRecord(value SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: value.ID, WorkspaceID: value.WorkspaceID, RunID: value.RunID,
		Transaction: value.Transaction, CandidateHash: value.CandidateHash, Candidate: value.Candidate,
		ValidationRecipes: runtimecontract.NormalizeRuntimeValidationRecipes(value.ValidationRecipes),
		State:             runtimecontract.RuntimeDispatchState(value.State), CompletedOperationIDs: append([]string(nil), value.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.NormalizeRuntimeOperationExecutionRecords(value.OperationExecutions),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
