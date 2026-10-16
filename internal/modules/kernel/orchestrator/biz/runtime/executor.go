package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationExecutionRequest struct {
	WorkspaceID                      common.WorkspaceID
	RunID                            common.RunID
	DispatchID                       string
	Transaction                      runtimecontract.AuthorityTransaction
	Operation                        runtimecontract.RuntimeOperation
	Execution                        runtimecontract.RuntimeOperationExecutionRecord
	GovernanceGrant                  *runtimecontract.RuntimeOperationGovernanceGrant
	EvidenceKind                     string
	HistoricalEffectProofContractRef string
	TargetRole                       string
	EvidencePolicies                 []runtimecontract.RuntimeEvidencePolicy
	ValidationRecipes                []runtimecontract.RuntimeValidationRecipe
	ValidationScopeTargetRefs        []string
	// ValidationProofBinding is the exact owner-native check mapping already
	// sealed by SM-2 for a managed-validation CWI. It is selected only from the
	// persisted CoverageEvidenceBinding and its persisted owner catalog; the
	// candidate parameter payload cannot supply or alter it.
	ValidationProofBinding             runtimecontract.RuntimeValidationProofRequirementBinding
	CapabilityStates                   []runtimecontract.RuntimeCapabilityStateSnapshot
	WorkspaceTreeObservation           *runtimecontract.RuntimePrerequisiteObservation
	CompletedRequiredOperationReceipts []runtimecontract.RuntimeRequiredOperationReceiptBinding
	// OwnerCatalog is the exact persisted owner-authored admissibility fact named
	// by Execution.AdmissibilitySnapshotID and Execution.AcquisitionBinding.
	// Protocol adapters may narrow it into a sealed typed authorization; Runtime
	// core does not interpret owner-specific metadata.
	OwnerCatalog *runtimecontract.RuntimeOperationAdmissibility
	// OwnerCatalogObservation is the exact persisted observation whose content
	// hash and freshness dependency bind OwnerCatalog. It is carried only so a
	// capability owner can return a typed stale receipt before any side effect.
	OwnerCatalogObservation *runtimecontract.RuntimePrerequisiteObservation
}

type RuntimeOperationExecutionResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Failure      *runtimecontract.FailureFact
	Evidence     runtimecontract.RuntimeOperationEvidence
	Receipt      runtimecontract.RuntimeOperationReceipt
	// Governance is set when the owner blocked side effects and requires
	// decision/approval before execution (INV-06 / RVC-R11). Must not be treated
	// as ordinary execution_rejected repair.
	Governance *runtimecontract.RuntimeGovernanceRequirement
}

type RuntimeOperationExecutor interface {
	ExecuteRuntimeOperation(context.Context, RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error)
}

func completedRequiredOperationReceiptBindings(dispatch SealedDispatch, receipts []runtimecontract.RuntimeOperationReceipt, completed map[string]struct{}) []runtimecontract.RuntimeRequiredOperationReceiptBinding {
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRequiredOperationReceiptBinding, 0, len(operations))
	for _, operation := range operations {
		if operation.RequiredOperationRef == "" {
			continue
		}
		if _, ok := completed[operation.ID]; !ok {
			continue
		}
		execution, ok := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
		if !ok || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
			continue
		}
		receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(receipts, execution.ExecutionKey, execution.Fence)
		if !ok {
			continue
		}
		contract, known := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
		if !known {
			continue
		}
		variant, variantErr := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
		if variantErr != nil {
			continue
		}
		ret = append(ret, runtimecontract.RuntimeRequiredOperationReceiptBinding{
			RequirementID: operation.RequiredOperationRef, OperationID: operation.ID, Kind: operation.Kind, VariantID: variant.ID,
			ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: execution.OwnerReceiptHash,
			Outcome: receipt.Outcome, ObservedScope: receipt.ObservedEffectScope, Changed: receipt.Changed,
		})
	}
	return runtimecontract.NormalizeRuntimeRequiredOperationReceiptBindings(ret)
}

func runtimeOperationEffectMismatchFailure(decision runtimecontract.RuntimeDecision, execution runtimecontract.RuntimeOperationExecutionRecord, operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) *runtimecontract.FailureFact {
	receipt = receipt.Normalized()
	expected := runtimecontract.NormalizeRuntimeEffectScope(execution.ExpectedEffectScope)
	observed := runtimecontract.NormalizeRuntimeEffectScope(receipt.ObservedEffectScope)
	reason := ""
	policy, policyKnown := runtimecontract.RuntimeConvergenceEffectPolicyFor(decision.State)
	switch {
	case decision.State == runtimecontract.ConvergenceStateNeedStateChange && receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp:
		reason = "owner completed the operation without a durable state change"
	case policyKnown && (receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp) && !policy.AllowsAcceptedReceiptEffect(observed):
		reason = fmt.Sprintf("convergence state %s cannot commit owner observed effect %q", decision.State, observed)
	case decision.State == runtimecontract.ConvergenceStateNeedStateChange && runtimecontract.RuntimeEffectIsDurable(expected) && !runtimecontract.RuntimeEffectIsDurable(observed):
		reason = fmt.Sprintf("owner observed effect %q cannot satisfy expected durable effect %q", observed, expected)
	case decision.State == runtimecontract.ConvergenceStateNeedValidation && runtimecontract.RuntimeEffectIsDurable(observed):
		reason = fmt.Sprintf("validation operation observed forbidden durable effect %q", observed)
	}
	if reason == "" {
		return nil
	}
	code := "operation_effect_mismatch"
	if decision.State == runtimecontract.ConvergenceStateNeedStateChange && receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp {
		// A no-op is not an effect mismatch that can be repaired by changing the
		// same file again.  It means the owner observed an already-current state
		// but this Run has no durable provenance for it.  Keep the neutral class
		// for receipt/effect audit, while giving SM-2 a typed branch that can
		// inspect the current workspace once instead of creating random deltas.
		code = runtimeStateChangeNoOpWithoutProvenanceCode
	}
	return &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassEffectMismatch, Code: code,
		Signature: "effect-mismatch-" + operation.ID, Message: reason,
	}
}

// recordOperationGovernanceRequired persists a governance_required failure fact so the
// reducer opens request_governance instead of same-focus execution repair (INV-06).
func (c *candidateDispatchComponent) recordOperationGovernanceRequired(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string, result RuntimeOperationExecutionResult) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if result.Governance == nil {
		return RuntimeAggregate{}, fmt.Errorf("operation governance result requires governance requirement")
	}
	if err := result.Governance.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	// Mark operation outcome without committing ordinary rejected evidence.
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation governance requires active dispatch", common.ErrConflict)
	}
	finding, findingErr := runtimeOperationSuspensionFinding(aggregate, *aggregate.ActiveDispatch, operationID, result.Receipt, *result.Governance)
	if findingErr != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation governance receipt mismatch: %v", common.ErrConflict, findingErr)
	}
	expectedRevision := aggregate.StateRevision
	// Preserve the exact operation identity before closing its dispatch. The later
	// ticket bind may attach governance dispatch/ticket information, but it must
	// never reconstruct an ExecutionKey or fence from kind-level data.
	aggregate.OperationGovernanceBindings = append(aggregate.OperationGovernanceBindings, finding.Binding)
	failure := runtimecontract.FailureFact{
		Class:                runtimecontract.FailureClassGovernanceRequired,
		Code:                 "approval_required",
		Signature:            "governance-" + finding.Dependency.Kind + "-" + finding.Dependency.SubjectRef,
		Message:              finding.Dependency.Reason,
		TargetRefs:           []string{finding.Dependency.SubjectRef},
		GovernanceDependency: &finding.Dependency,
	}
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetainInterlude, nil); err != nil {
		return RuntimeAggregate{}, err
	}
	// Close active dispatch as committed-without-evidence so reduce can open governance.
	if aggregate.ActiveDispatch != nil {
		dispatch := *aggregate.ActiveDispatch
		dispatch.State = DispatchStateCommitted
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
	}
	facts, decision, reduceErr := reduceRuntimeAggregateWithDirective(aggregate, aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if reduceErr != nil {
		return RuntimeAggregate{}, reduceErr
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// 两条私有事务共享同一 exact persisted receipt / execution 校验，仍由调用方在原 CAS 提交写集。
func runtimeOperationSuspensionFinding(aggregate RuntimeAggregate, dispatch SealedDispatch, operationID string, response runtimecontract.RuntimeOperationReceipt, requirement runtimecontract.RuntimeGovernanceRequirement) (governancecheck.OperationSuspensionFinding, error) {
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
	if !found {
		return governancecheck.OperationSuspensionFinding{}, fmt.Errorf("%w: operation governance requires sealed execution", common.ErrConflict)
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found {
		return governancecheck.OperationSuspensionFinding{}, fmt.Errorf("%w: operation governance requires persisted governance receipt", common.ErrConflict)
	}
	return governancecheck.EvaluateOperationSuspension(dispatch.ID, execution, persisted, response, requirement)
}

func operationExecutionState(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionState, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, value := range values {
		if value.Normalized().OperationID == operationID {
			return value.State.Normalized(), true
		}
	}
	return "", false
}

func operationExecutionRecord(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, value := range values {
		value = value.Normalized()
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func operationEvidenceFromPersistedReceipt(aggregate RuntimeAggregate, dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) (ExecutionEvidence, error) {
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
	if !found {
		return ExecutionEvidence{}, fmt.Errorf("operation %q has no sealed execution record", operation.ID)
	}
	// The capability response may be the pre-CAS receipt shape (for example,
	// before Runtime binds its execution fence). Evidence identity and owner
	// diagnostic authority must instead come from the exact receipt already
	// persisted by ExecutionKey+Fence in this aggregate.
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || persisted.OperationID != operation.ID {
		return ExecutionEvidence{}, fmt.Errorf("operation %q has no exact persisted owner receipt", operation.ID)
	}
	receipt = persisted
	status, failure, err := runtimeOperationReceiptEvidenceJudgment(aggregate.Decision, execution, operation, receipt)
	if err != nil {
		return ExecutionEvidence{}, err
	}
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	return ExecutionEvidence{
		EventID: "runtime-receipt|" + hash, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Status: status, SourceRef: "runtime-receipt/" + receipt.ExecutionKey,
		Failure: failure, Evidence: receipt.Evidence, ObservedEffectScope: receipt.ObservedEffectScope, NoOp: receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp,
		OwnerReceiptRef: "runtime-receipt/" + receipt.ExecutionKey, OwnerReceiptHash: hash, Receipt: &receipt,
	}, nil
}

// runtimeOperationReceiptEvidenceJudgment is the only ordinary receipt-to-
// evidence judgment used by both the live owner-response path and fresh-load
// receipt recovery. Receipt outcome is authoritative and is evaluated before
// Orchestrator-owned effect/quality checks: a rejected owner receipt therefore
// retains its exact diagnostic and can never be rewritten as an effect
// mismatch merely because rejected receipts conservatively observe unknown.
func runtimeOperationReceiptEvidenceJudgment(decision runtimecontract.RuntimeDecision, execution runtimecontract.RuntimeOperationExecutionRecord, operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationExecutionStatus, *runtimecontract.FailureFact, error) {
	receipt = receipt.Normalized()
	switch receipt.Outcome {
	case runtimecontract.RuntimeOperationOutcomeRejected:
		failure := failureFactFromOperationReceiptFeedback(receipt)
		if failure == nil {
			return "", nil, fmt.Errorf("rejected operation receipt %q is missing owner diagnostic feedback", receipt.OperationID)
		}
		return ExecutionEvidenceStatusRejected, failure, nil
	case runtimecontract.RuntimeOperationOutcomeAccepted, runtimecontract.RuntimeOperationOutcomeNoOp:
		if failure := runtimeOperationEffectMismatchFailure(decision, execution, operation, receipt); failure != nil {
			return ExecutionEvidenceStatusRejected, failure, nil
		}
		if failure := runtimeOperationEvidenceQualityFailure(operation, receipt.Evidence); failure != nil {
			return ExecutionEvidenceStatusRejected, failure, nil
		}
		return ExecutionEvidenceStatusAccepted, nil, nil
	default:
		return "", nil, fmt.Errorf("operation receipt outcome %q cannot commit ordinary evidence", receipt.Outcome)
	}
}

func runtimeOperationEvidenceQualityFailure(operation runtimecontract.RuntimeOperation, evidence runtimecontract.RuntimeOperationEvidence) *runtimecontract.FailureFact {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession && operation.Kind != runtimecontract.StepKindShellCommand {
		return nil
	}
	quality := common.EvidenceQuality(strings.TrimSpace(string(evidence.Quality)))
	if !evidence.ZeroWork && quality != common.EvidenceQualityInconclusive && quality != common.EvidenceQualityInvalid && quality != common.EvidenceQualitySkipped {
		return nil
	}
	reason := strings.TrimSpace(evidence.EvidenceReason)
	if reason == "" {
		reason = "validation operation completed without acceptable execution evidence"
	}
	qualityLabel := strings.TrimSpace(string(quality))
	if qualityLabel == "" {
		qualityLabel = "zero_work"
	}
	bindings := append([]string(nil), operation.BindingRefs...)
	return &runtimecontract.FailureFact{
		Class:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
		Code:      "validation_evidence_inconclusive",
		Signature: strings.Join([]string{"validation-evidence", operation.Kind, qualityLabel, strings.Join(bindings, ",")}, "/"),
		Message:   reason, TargetRefs: append([]string(nil), operation.TargetRefs...),
	}
}

func (c *candidateDispatchComponent) claimOperationExecution(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: no active dispatch for claim", common.ErrConflict)
	}
	if err := validateRuntimeExecutionPurposeFence(aggregate); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation claim binding fence: %v", common.ErrConflict, err)
	}
	expectedRevision := aggregate.StateRevision
	found := false
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		exec := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if exec.OperationID != operationID {
			continue
		}
		found = true
		switch exec.State {
		case runtimecontract.OperationExecutionSealed:
			nextFence := exec.Fence + 1
			if nextFence == 0 {
				nextFence = 1
			}
			if err := reserveRuntimeOperationExecutionBudget(&aggregate, *exec, nextFence); err != nil {
				return RuntimeAggregate{}, err
			}
			exec.State = runtimecontract.OperationExecutionClaimed
			exec.Fence = nextFence
			exec.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerInitial
		case runtimecontract.OperationExecutionClaimed:
			return RuntimeAggregate{}, fmt.Errorf("operation %q is already claimed; query receipt before recovery", operationID)
		case runtimecontract.OperationExecutionOwnerCommitted, runtimecontract.OperationExecutionEvidenceCommitted:
			return aggregate, nil
		case runtimecontract.OperationExecutionOutcomeUnknown:
			return RuntimeAggregate{}, operationOutcomeUnknown(operationID, "execution cannot be reclaimed for replay")
		}
	}
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation %q execution ledger is missing", common.ErrConflict, operationID)
	}
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// markGovernanceResumeExecutionStarted is the persisted pre-effect edge for a
// governance continuation. The resolution CAS authorizes fence N+1; this CAS
// proves that the one unbudgeted owner call for that grant has begun. Any later
// re-drive therefore enters the ordinary SM-4 recovery-budget path.
func (c *candidateDispatchComponent) markGovernanceResumeExecutionStarted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, *runtimecontract.RuntimeOperationGovernanceGrant, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, nil, err
	}
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, nil, fmt.Errorf("%w: governance resume requires active dispatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	found := false
	var grant *runtimecontract.RuntimeOperationGovernanceGrant
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		execution := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if execution.OperationID != operationID {
			continue
		}
		found = true
		grant = governancecheck.GrantForExecution(aggregate.OperationGovernanceBindings, *execution)
		if execution.State != runtimecontract.OperationExecutionClaimed || execution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted || grant == nil {
			return RuntimeAggregate{}, nil, fmt.Errorf("%w: governance resume execution is not exactly granted", common.ErrConflict)
		}
		execution.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted
		break
	}
	if !found {
		return RuntimeAggregate{}, nil, fmt.Errorf("%w: governance resume operation %q is missing", common.ErrConflict, operationID)
	}
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, nil, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, nil, err
	}
	return aggregate, grant, nil
}

func (c *candidateDispatchComponent) markOperationExecutionOutcomeUnknown(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	found := false
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		exec := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if exec.OperationID != operationID {
			continue
		}
		found = true
		operation, ok := candidateOperationByID(aggregate.ActiveDispatch.Candidate, operationID)
		if !ok {
			return RuntimeAggregate{}, fmt.Errorf("%w: operation outcome has no sealed candidate operation", common.ErrConflict)
		}
		if _, err := joinRuntimeOperationExecutionBudget(&aggregate, operation, *exec, nil, true); err != nil {
			return RuntimeAggregate{}, err
		}
		exec.State = runtimecontract.OperationExecutionOutcomeUnknown
		if exec.Fence == 0 {
			exec.Fence = 1
		}
	}
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation %q execution ledger is missing", common.ErrConflict, operationID)
	}
	if aggregate.ActiveDispatch.ImplementationWorkCommitment != nil {
		dispatch := *aggregate.ActiveDispatch
		matched := false
		for i := range aggregate.ImplementationWork.Rounds {
			round := &aggregate.ImplementationWork.Rounds[i]
			if round.DispatchID != dispatch.ID {
				continue
			}
			execution, _ := operationExecutionRecord(dispatch.OperationExecutions, operationID)
			round.OutcomeUnknownExecutionKey, round.OutcomeUnknownFence = execution.ExecutionKey, execution.Fence
			round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(*round)
			matched = true
		}
		if !matched {
			return RuntimeAggregate{}, fmt.Errorf("implementation unknown claim has no original round")
		}
		dispatch.State = DispatchStateInvalidated
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
		retireImplementationWorkRecoveryAuthorizations(&aggregate, dispatch.ID)
		failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassOperationOutcomeUnknown, Code: "implementation_work_outcome_unknown", Signature: "implementation-work-unknown/" + dispatch.ID + "/" + operationID, Message: "started owner operation has no trustworthy terminal receipt; physical consumption remains unknown"}
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return RuntimeAggregate{}, err
		}
		var err error
		aggregate.Facts, aggregate.Decision, err = reduceRuntimeAggregateWithDirective(aggregate, aggregate.Facts, nil)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func evidenceKindForDecision(decision runtimecontract.RuntimeDecision) string {
	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return runtimecontract.EvidenceKindStateChange
	case runtimecontract.ConvergenceStateNeedValidation:
		return runtimecontract.EvidenceKindValidation
	default:
		return ""
	}
}

func orderedCandidateOperations(candidate runtimecontract.RuntimeCandidate) ([]runtimecontract.RuntimeOperation, error) {
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 {
		return nil, fmt.Errorf("runtime dispatch candidate has no executable operations")
	}
	byID := make(map[string]runtimecontract.RuntimeOperation, len(operations))
	for _, value := range operations {
		operation := value.Normalized()
		if operation.ID == "" {
			return nil, fmt.Errorf("runtime operation requires id")
		}
		byID[operation.ID] = operation
	}
	completed := map[string]struct{}{}
	ret := make([]runtimecontract.RuntimeOperation, 0, len(byID))
	for len(ret) < len(byID) {
		ready := make([]string, 0)
		for id, operation := range byID {
			if _, ok := completed[id]; ok {
				continue
			}
			dependenciesReady := true
			for _, dependency := range operation.DependsOn {
				if _, ok := byID[dependency]; !ok {
					return nil, fmt.Errorf("runtime operation %q depends on unknown operation %q", id, dependency)
				}
				if _, ok := completed[dependency]; !ok {
					dependenciesReady = false
					break
				}
			}
			if dependenciesReady {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("runtime operation dependency graph contains a cycle")
		}
		sort.Strings(ready)
		for _, id := range ready {
			ret = append(ret, byID[id])
			completed[id] = struct{}{}
		}
	}
	return ret, nil
}

func candidateRuntimeOperations(candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeOperation {
	switch {
	case candidate.StateChange != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.StateChange.Operations))
		for _, operation := range candidate.StateChange.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret
	case candidate.Validation != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.Validation.Operations))
		for _, operation := range candidate.Validation.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret
	default:
		return nil
	}
}

func validateRuntimeOperationExecutionResult(decision runtimecontract.RuntimeDecision, operation runtimecontract.RuntimeOperation, result RuntimeOperationExecutionResult) error {
	if strings.TrimSpace(result.EventID) == "" || strings.TrimSpace(result.OperationID) != operation.ID {
		return fmt.Errorf("runtime operation result requires matching event and operation ids")
	}
	wantKind := ""
	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		wantKind = runtimecontract.EvidenceKindStateChange
	case runtimecontract.ConvergenceStateNeedValidation:
		wantKind = runtimecontract.EvidenceKindValidation
	default:
		return fmt.Errorf("runtime decision state %s cannot execute operations", decision.State)
	}
	if strings.TrimSpace(result.EvidenceKind) != wantKind {
		return fmt.Errorf("runtime operation result evidence kind %q does not match convergence state %s", result.EvidenceKind, decision.State)
	}
	if result.Status != ExecutionEvidenceStatusAccepted && result.Status != ExecutionEvidenceStatusRejected {
		return fmt.Errorf("runtime operation result has invalid status %q", result.Status)
	}
	if result.Status == ExecutionEvidenceStatusRejected && result.Failure == nil {
		return fmt.Errorf("rejected runtime operation result requires failure facts")
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	ret := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = struct{}{}
		}
	}
	return ret
}
