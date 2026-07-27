package runtimev2

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
	repository RuntimeStateRepository
	dispatch   *DispatchCoordinator
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
	dispatch, err := NewDispatchCoordinator(repository)
	if err != nil {
		return nil, err
	}
	return &RuntimeController{repository: repository, dispatch: dispatch}, nil
}

// Initialize 建立一个 Run 唯一的 Runtime V2 aggregate。
// 已初始化的 Run 只允许幂等读取，不会因为 Start/Resume/restart 再次执行 reducer。
func (c *RuntimeController) Initialize(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []RuntimeRequirement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	facts = facts.Normalized()
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
		if !isTerminalRuntimeAction(existing.Decision.Action) {
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
			return RuntimeAggregate{}, fmt.Errorf("%w: runtime v2 state already exists with a different definition", common.ErrConflict)
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
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate := RuntimeAggregate{
		StateRevision: 1,
		Definition:    definition,
		Facts:         facts,
		Decision:      decision,
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, 0); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func isTerminalRuntimeAction(action runtimecontract.RuntimeAction) bool {
	return action == runtimecontract.RuntimeActionComplete || action == runtimecontract.RuntimeActionFail
}

func (c *RuntimeController) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.repository.Load(ctx, workspaceID, runID)
}

// ReconcileAuthorityBeforeCandidate CAS-persists the deterministic SM-2
// cleanup required when a loaded aggregate still contains a non-current
// focus-scoped prerequisite/resource. It is intentionally narrower than
// candidate admission: no provider request, candidate attempt, repair budget,
// or owner operation is involved.
func (c *RuntimeController) ReconcileAuthorityBeforeCandidate(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || isTerminalRuntimeAction(aggregate.Decision.Action) || len(InspectRuntimeAuthorityInvariants(aggregate).Violations) == 0 {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	// A provider ceiling is activation-sealed by bootstrap. If an owner resource
	// was cleared or became insufficient while its prerequisite still appears
	// active, return to the existing typed acquisition rather than allowing the
	// LLM boundary to discover the defect after an SM-3 attempt exists. Do not
	// synthesize a new/default requirement here: legacy owner-local tests without
	// a sealed provider contract are deliberately not production candidate paths.
	facts := aggregate.Facts
	if requirements := activeProviderCeilingRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// RecordFeasibilityFact CAS-persists the pre-candidate capability gate and
// immediately re-reduces. A missing operation is therefore a formal SM-2 fact,
// never a provider error or consumed candidate attempt.
func (c *RuntimeController) RecordFeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.RuntimeFeasibilityFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || (aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange && aggregate.Decision.Action != runtimecontract.RuntimeActionProduceValidation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept feasibility facts", common.ErrConflict)
	}
	fact.FocusID = strings.TrimSpace(fact.FocusID)
	if fact.FocusID == "" {
		fact.FocusID = aggregate.Facts.Focus.ID
	}
	if fact.FocusID != aggregate.Facts.Focus.ID || fact.Action != aggregate.Decision.Action || fact.Reason == "" {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact does not match current authority", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.Facts.Feasibility = &fact
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// RecordFailureFact persists an SM-2 failure input and lets the reducer choose
// repair, governance, or terminal. Drive/adapter errors must never project a
// Run terminal without this authority transition.
func (c *RuntimeController) RecordFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || fact.Class == "" || strings.TrimSpace(fact.Message) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action == runtimecontract.RuntimeActionComplete || aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	fact.Code = strings.TrimSpace(fact.Code)
	fact.Signature = strings.TrimSpace(fact.Signature)
	fact.Message = strings.TrimSpace(fact.Message)
	aggregate.Facts.Failure = &fact
	// A persisted FailureTarget is scoped to the current typed failure identity.
	// Re-recording a failure with a different signature/target set must return to
	// the existing local prerequisite, never leave the former mapping as an
	// implicit repair surface until a later drive happens to preflight it.
	facts := aggregate.Facts
	if requirements := activeFailureTargetRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot)
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func activeFailureTargetRequirements(facts runtimecontract.RuntimeFacts) []RuntimeRequirement {
	facts = facts.Normalized()
	values := make([]RuntimeRequirement, 0, 1)
	seen := make(map[string]struct{})
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...) {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindFailureTarget {
			continue
		}
		requirement := RuntimeRequirement{
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

// RequestTermination CAS-persists an administrative termination record on an active V2
// aggregate (INV-08). SM-1 may project cancelled/superseded only after this record exists.
func (c *RuntimeController) RequestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || reason == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action == runtimecontract.RuntimeActionComplete || aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept administrative termination", common.ErrConflict)
	}
	if aggregate.Termination != nil {
		if aggregate.Termination.Kind == kind {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: termination already recorded as %s", common.ErrConflict, aggregate.Termination.Kind)
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) CommitPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result PrerequisiteResult) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept prerequisite results", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
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
	// A single-operation durable dispatch may have completed its current
	// coverage before its receipt invalidated mutable prerequisites. Once the
	// last affected prerequisite has been reacquired, preserve the accepted
	// evidence and advance the satisfied focus exactly as a non-invalidated
	// dispatch would. Partial dispatches keep FreshnessReacquireCoverage and
	// therefore cannot skip their required fresh replacement work.
	if len(facts.Prerequisites.Pending) == 0 && len(facts.FreshnessReacquireCoverage) == 0 {
		facts = advanceSatisfiedCoverageFocus(facts)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) BeginCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
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
				if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
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
		if first.Failure == nil || !runtimecontract.RuntimeCandidateFailureIsCorrectableOnce(first.Failure) {
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// AdmitCandidateProposal atomically persists a candidate proposal or closes the
// pending attempt with its typed admission rejection. A restart therefore never
// observes a rejected provider candidate as an open SM-3 transaction.
func (c *RuntimeController) AdmitCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
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
		if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// RecordCandidateFeasibility persists one completed, side-effect-free owner
// probe. The operation must belong to the already persisted proposal, making
// this ledger a restart fence rather than a second candidate authority.
func (c *RuntimeController) RecordCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (RuntimeAggregate, error) {
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
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, attemptRecord, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok || attemptRecord.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal", common.ErrConflict, attempt)
	}
	if !candidateProposalContainsOperation(*attemptRecord.Proposal, record.OperationID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q is absent from the persisted proposal", common.ErrConflict, record.OperationID)
	}
	for _, persisted := range attemptRecord.Feasibilities {
		if strings.TrimSpace(persisted.OperationID) != strings.TrimSpace(record.OperationID) {
			continue
		}
		if persisted.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || persisted.AcquisitionBinding != record.AcquisitionBinding || persisted.Feasible != record.Feasible || !reflect.DeepEqual(persisted.ReplayProof, record.ReplayProof) || !reflect.DeepEqual(persisted.Failure, record.Failure) || !reflect.DeepEqual(persisted.Stale, record.Stale) || !reflect.DeepEqual(persisted.PrerequisiteRequest, record.PrerequisiteRequest) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility result for operation %q changed", common.ErrConflict, record.OperationID)
		}
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	copyRecord := record
	if record.Failure != nil {
		failure := *record.Failure
		failure.TargetRefs = append([]string(nil), record.Failure.TargetRefs...)
		copyRecord.Failure = &failure
	}
	if record.Stale != nil {
		stale := *record.Stale
		copyRecord.Stale = &stale
	}
	if record.ReplayProof != nil {
		proof := record.ReplayProof.Clone()
		copyRecord.ReplayProof = &proof
	}
	if record.PrerequisiteRequest != nil {
		request := record.PrerequisiteRequest.Normalized()
		copyRecord.PrerequisiteRequest = &request
	}
	aggregate.CandidateAttempts[idx].Feasibilities = append(aggregate.CandidateAttempts[idx].Feasibilities, copyRecord)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
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

// AdmitAndSealCandidateAttempt is the final SM-3 admission fence. It atomically
// closes an expected rejection or atomically accepts the attempt and seals its
// dispatch. Feasibility/invariant defects remain ordinary errors because they
// are not provider-correctable candidate outcomes.
func (c *RuntimeController) AdmitAndSealCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
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
		if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	if err := validateCandidateAdmissionFeasibility(aggregate.Facts, candidate, record); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	dispatch, err := SealDispatchWithActivationAndFeasibilities(aggregate.Decision, candidate, aggregate.Facts.Activation, record.Feasibilities)
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// SealGovernanceDispatch seals the reducer-authored external dependency without
// creating a provider request or candidate attempt. Governance is an SM-2
// fact route, not a synthetic LLM correction path; therefore it must never
// consume the complete-document/session ledger used by SM-3 provider output.
func (c *RuntimeController) SealGovernanceDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime authority cannot seal governance", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources)); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance manifest does not match sealed authority: %v", common.ErrConflict, err)
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
	if admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate); !admission.Accepted() {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate rejected by manifest", common.ErrConflict)
	}
	dispatch, err := SealDispatchWithActivationAndFeasibilities(aggregate.Decision, candidate, aggregate.Facts.Activation, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
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
	target := facts.Resources.ProviderCeiling.Target.Normalized()
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

// validateCandidateAdmissionFeasibility is the final write-side fence for
// SM-3. Mainline records probes, but this controller-level validation prevents
// any alternate caller from sealing an executable candidate without every
// required persisted owner feasibility result.
func validateCandidateAdmissionFeasibility(facts runtimecontract.RuntimeFacts, candidate runtimecontract.RuntimeCandidate, attempt runtimecontract.RuntimeCandidateAttemptRecord) error {
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 || !runtimeOperationAdmissibilityRequired(facts.Prerequisites) {
		return nil
	}
	for _, operation := range operations {
		operation = operation.Normalized()
		descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !known || descriptor.OwnerModuleID == "" {
			return fmt.Errorf("candidate operation %q has no canonical owner", operation.Kind)
		}
		inventory, found := runtimeOperationAdmissibilityForKind(facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			return fmt.Errorf("candidate operation %q has no persisted owner admissibility inventory", operation.Kind)
		}
		feasibility, found := candidateAttemptFeasibilityForOperation(attempt.Feasibilities, operation.ID)
		if !found || feasibility.AdmissibilitySnapshotID != inventory.SnapshotID {
			return fmt.Errorf("candidate operation %q has no current persisted feasibility result", operation.ID)
		}
		if !feasibility.Feasible {
			return fmt.Errorf("candidate operation %q has persisted infeasible result", operation.ID)
		}
		if feasibility.ReplayProof != nil {
			proof := feasibility.ReplayProof.Normalized()
			if err := proof.Validate(); err != nil || proof.OwnerModuleID != descriptor.OwnerModuleID || proof.OperationID != operation.ID || proof.AdmissibilitySnapshotID != inventory.SnapshotID || proof.AcquisitionBinding != inventory.AcquisitionBinding || !sameStringSet(proof.TargetRefs, operation.TargetRefs) {
				return fmt.Errorf("candidate operation %q has invalid persisted replay proof", operation.ID)
			}
		}
	}
	return nil
}

func runtimeOperationAdmissibilityRequired(prerequisites runtimecontract.PrerequisiteState) bool {
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), prerequisites.Pending...), prerequisites.Satisfied...) {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			return true
		}
	}
	return false
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

func (c *RuntimeController) FailCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
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
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
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
	diagnosticClass := ""
	if failure.ValidationDiagnostic != nil {
		diagnostic := failure.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &diagnostic
		diagnosticClass = diagnostic.Class
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
		case runtimecontract.RuntimeCandidateFailureClassInfeasible:
			failure.Code = "operation_target_not_regular"
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
	if sealed, directive, sealErr := runtimecontract.SealRuntimeCandidateRejection(&failure, manifest); sealErr == nil && sealed != nil {
		// Attempt 0 must keep the first-failure code stable for diagnostics and
		// attempt-1 gating. Seal may rewrite an incomplete correctable into
		// runtime_contract_defect; that rewrite is only forced onto the attempt
		// record for attempt 1 / already-final codes. Attempt 0 still persists
		// objectives (or the lack of them) so IsCorrectableOnce can refuse retry.
		if attempt == 1 || sealed.Code == originalCode || originalCode == "runtime_contract_defect" ||
			runtimecontract.RuntimeCandidateFailureIsRejectedFinal(&runtimecontract.RuntimeCandidateFailure{Code: originalCode}) {
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
	needsInfeasibleRepair := failure.Class == runtimecontract.RuntimeCandidateFailureClassInfeasible &&
		aggregate.Decision.Action == runtimecontract.RuntimeActionProduceValidation &&
		runtimecontract.RuntimeValidationFailureRequiresStateChange(aggregate.Facts.Activation.ExecutionContract.ValidationPolicies, runtimecontract.EvidenceKindValidation, diagnosticClass)
	if needsInfeasibleRepair {
		gap := runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			TargetRefs:           append([]string(nil), failure.TargetRefs...),
			Reason:               failure.Reason,
			FailureClass:         runtimecontract.FailureClassCandidateInfeasible,
			FailureCode:          failure.Code,
			FailureSignature:     failure.Signature,
		}
		if len(gap.TargetRefs) == 0 {
			gap.TargetRefs = append([]string(nil), aggregate.Facts.Focus.TargetRefs...)
		}
		gap = gap.Normalized()
		if strings.TrimSpace(failure.Signature) == "" {
			failure.Signature = runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, &gap)
			gap.FailureSignature = failure.Signature
		}
		previousCount := repairFailureSignatureCount(aggregate.Facts.Repair, failure.Signature)
		record := aggregate.CandidateAttempts[idx]
		sourceFactID := fmt.Sprintf("candidate/%s/%d", strings.TrimSpace(record.AuthorityContractID), attempt)
		cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, failure.Signature, sourceFactID)
		ledger, _, err := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, failure.Signature, aggregate.Facts.ConvergencePolicy)
		if err != nil {
			return err
		}
		ledger.ActiveGap = &gap
		if previousCount > 0 {
			ledger = noteRepairNoProgress(ledger)
		}
		aggregate.Facts.Repair = ledger
	}
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	needsReduce := false
	if needsInfeasibleRepair {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCandidateInfeasible, Code: failure.Code, Signature: failure.Signature,
			Message: failure.Reason, TargetRefs: append([]string(nil), failure.TargetRefs...), ValidationDiagnostic: failure.ValidationDiagnostic,
		}
		needsReduce = true
	} else if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
		needsReduce = true
	} else if attempt == 1 {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
			Signature: "candidate-retry-exhausted", Message: failure.Reason,
		}
		needsReduce = true
	} else if attempt == 0 && runtimecontract.RuntimeCandidateFailureIsRejectedFinal(&failure) {
		// N6: explicit rejected_final attempt 0 seals terminal in the same CAS.
		// Non-matrix / incomplete-correctable failures still rely on the attempt-1
		// BeginCandidateAttempt + mainline fences so same-contract repair paths
		// (for example validation infeasible without state-change policy) are not
		// collapsed into an immediate terminal here.
		code := runtimecontract.RuntimeCandidateRejectionTerminalCode(failure.Code)
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
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
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
	if aggregate == nil || (aggregate.Decision.Action != runtimecontract.RuntimeActionFail && aggregate.Decision.Action != runtimecontract.RuntimeActionComplete && aggregate.Decision.Terminal == nil) {
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
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateFailed || record.Failure == nil {
			continue
		}
		if record.Proposal != nil {
			if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != candidateHash {
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

func (c *RuntimeController) CommitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.dispatch == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return c.dispatch.CommitEvidence(ctx, evidence)
}

func (c *RuntimeController) CommitAssessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Assessment == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime dispatch does not carry assessment", common.ErrConflict)
	}
	assessment := aggregate.ActiveDispatch.Candidate.Assessment.Normalized()
	disposition, err := runtimecontract.ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	assessment.Disposition = disposition
	known := stringSet(runtimecontract.AcceptedEvidenceRefsForFocus(aggregate.Facts))
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		for _, ref := range assessment.EvidenceRefs {
			if _, ok := known[ref]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("assessment evidence ref %q is not accepted current-focus evidence", ref)
			}
		}
		if len(assessment.EvidenceRefs) == 0 {
			return RuntimeAggregate{}, fmt.Errorf("satisfied assessment requires accepted current-focus evidence refs")
		}
	}
	// Item facts must stay inside current focus/slice coverage.
	requiredCoverage := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range aggregate.Facts.Focus.CoverageWorkItemIDs {
		requiredCoverage[id] = struct{}{}
	}
	for idx := range assessment.Items {
		item := &assessment.Items[idx]
		if len(requiredCoverage) > 0 {
			if _, ok := requiredCoverage[item.CoverageWorkItemID]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("assessment item %q is outside current slice coverage", item.CoverageWorkItemID)
			}
		}
		if item.Satisfied {
			refs := item.EvidenceRefs
			if len(refs) == 0 {
				refs = assessment.EvidenceRefs
			}
			if len(refs) == 0 {
				return RuntimeAggregate{}, fmt.Errorf("satisfied assessment item %q requires current-focus evidence refs", item.CoverageWorkItemID)
			}
			for _, ref := range refs {
				if _, ok := known[ref]; !ok {
					return RuntimeAggregate{}, fmt.Errorf("assessment item %q evidence ref %q is not accepted current-focus evidence", item.CoverageWorkItemID, ref)
				}
			}
			item.EvidenceRefs = append([]string(nil), refs...)
		}
	}
	assessment = applyAssessmentEvidenceGate(aggregate, assessment)
	assessment.CoverageWorkItemIDs = runtimeAssessmentCoverageIDs(assessment.CoverageWorkItemIDs, aggregate.Facts.Focus.CoverageWorkItemIDs)
	aggregate.Facts.Coverage = applyAssessmentCoverage(aggregate.Facts.Coverage, assessment, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, aggregate.ActiveDispatch.ID)
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		if open := openCoverageIDs(aggregate.Facts.Coverage, aggregate.Facts.Focus.CoverageWorkItemIDs); len(open) > 0 {
			assessment.Disposition = runtimecontract.AssessmentDispositionRepairableGap
			assessment.Gap = &runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs: open,
				Reason:              "assessment did not close every current slice coverage item",
			}
		}
	}
	expectedRevision := aggregate.StateRevision
	fact := runtimecontract.RuntimeAssessmentFact{
		ID: aggregate.ActiveDispatch.ID, FocusID: assessment.FocusID, SliceID: firstNonEmpty(assessment.SliceID, aggregate.Facts.Focus.SliceID),
		Scope: aggregate.Facts.Focus.AssessmentScope, RequirementID: aggregate.Facts.Focus.AssessmentID,
		RequirementText: aggregate.Facts.Focus.AssessmentText, CoverageWorkItemIDs: assessment.CoverageWorkItemIDs,
		Disposition: assessment.Disposition, Items: append([]runtimecontract.RuntimeAssessmentItemFact(nil), assessment.Items...),
		Summary: assessment.Summary, EvidenceRefs: assessment.EvidenceRefs, ArtifactRefs: assessment.ArtifactRefs,
	}
	if assessment.Gap != nil {
		gap := assessment.Gap.Normalized()
		fact.Gap = &gap
	}
	aggregate.Facts.Assessments = append(aggregate.Facts.Assessments, fact)
	if assessment.Disposition == runtimecontract.AssessmentDispositionRepairableGap {
		priorGap := aggregate.Facts.Repair.ActiveGap
		signature := runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, assessment.Disposition, assessment.Gap)
		cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, fact.ID)
		ledger, _, cycleErr := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
		if cycleErr != nil {
			return RuntimeAggregate{}, cycleErr
		}
		if assessment.Gap != nil {
			gap := assessment.Gap.Normalized()
			ledger.ActiveGap = &gap
			// A repairable assessment only resets the same-focus progress windows
			// when its persisted neutral gap set strictly narrows. The comparison
			// itself is a pure contract helper; SM-2 alone decides to apply it.
			// Repeated or expanded gaps retain their signatures/shapes and must not
			// create a fresh retry window merely because an assessment was accepted.
			if priorGap != nil && runtimecontract.RuntimeAssessmentGapStrictlyNarrows(*priorGap, gap) {
				ledger = noteRepairGapProgress(ledger, gap, aggregate.Facts.EvidenceRevision)
			} else {
				// An accepted operation is not harmless merely because its receipt
				// succeeded. Once assessment reports the same (or a wider) persisted
				// gap, retain every concrete current-focus operation shape that led
				// here. Candidate admission consumes this ledger on restart as well,
				// so changing prose cannot replay an accepted no-progress repair.
				ledger = recordAcceptedNoProgressShapes(aggregate, ledger, signature, assessment.Gap)
			}
		}
		if assessment.Gap != nil {
			hasStateChangeGap := false
			hasValidationGap := false
			for _, kind := range assessment.Gap.MissingEvidenceKinds {
				if kind == runtimecontract.EvidenceKindStateChange {
					hasStateChangeGap = true
				}
				if kind == runtimecontract.EvidenceKindValidation {
					hasValidationGap = true
				}
			}
			if hasValidationGap && !hasStateChangeGap {
				ledger = markValidationOnlyRetry(ledger)
			}
		}
		aggregate.Facts.Repair = ledger
	}
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, aggregate.Facts.EvidenceRevision)
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
	facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func recordAcceptedNoProgressShapes(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeRepairLedger, signature string, gap *runtimecontract.RuntimeAssessmentGap) runtimecontract.RuntimeRepairLedger {
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if !runtimecontract.EvidenceAppliesToCurrentFocus(aggregate.Facts, evidence) {
			continue
		}
		switch evidence.Kind {
		case runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation:
		default:
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

func runtimeAssessmentCoverageIDs(ids []common.CoverageWorkItemID, fallback []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	if len(ids) == 0 {
		ids = fallback
	}
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

// applyAssessmentCoverage is deliberately ledger-first: an assessment can only
// close the concrete items it names, and later assessments cannot reopen a
// satisfied item. This preserves partial acceptance across repair/restart.
func applyAssessmentCoverage(coverage []runtimecontract.RuntimeCoverageItemFact, assessment runtimecontract.AssessmentCandidate, focusID, sliceID, assessmentID string) []runtimecontract.RuntimeCoverageItemFact {
	itemClaims := map[common.CoverageWorkItemID]runtimecontract.RuntimeAssessmentItemFact{}
	for _, item := range assessment.Items {
		itemClaims[item.CoverageWorkItemID] = item
	}
	for idx := range coverage {
		item := &coverage[idx]
		if item.SliceID != "" && sliceID != "" && item.SliceID != sliceID {
			continue
		}
		claim, named := itemClaims[item.ID]
		if !named && assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
			for _, id := range assessment.CoverageWorkItemIDs {
				if id == item.ID {
					claim = runtimecontract.RuntimeAssessmentItemFact{CoverageWorkItemID: item.ID, Satisfied: true, EvidenceRefs: assessment.EvidenceRefs}
					named = true
					break
				}
			}
		}
		if named && claim.Satisfied && len(claim.EvidenceRefs) == 0 {
			claim.EvidenceRefs = append([]string(nil), assessment.EvidenceRefs...)
		}
		if !named || item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		item.LastAssessmentID = assessmentID
		item.EvidenceRefs = append(item.EvidenceRefs, claim.EvidenceRefs...)
		if claim.Satisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateSatisfied
		} else {
			item.State = runtimecontract.RuntimeCoverageItemStateUnsatisfied
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
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
		ret = append(ret, runtimecontract.RuntimeDispatchRecord{
			ID: value.ID, WorkspaceID: value.WorkspaceID, RunID: value.RunID,
			Transaction: value.Transaction, CandidateHash: value.CandidateHash, Candidate: value.Candidate,
			State: runtimecontract.RuntimeDispatchState(value.State), CompletedOperationIDs: append([]string(nil), value.CompletedOperationIDs...),
		})
	}
	return ret
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
