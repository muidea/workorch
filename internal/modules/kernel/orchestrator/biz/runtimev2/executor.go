package runtimev2

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationExecutionRequest struct {
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	DispatchID                         string
	Transaction                        runtimecontract.AuthorityTransaction
	Operation                          runtimecontract.RuntimeOperation
	Execution                          runtimecontract.RuntimeOperationExecutionRecord
	GovernanceGrant                    *runtimecontract.RuntimeOperationGovernanceGrant
	EvidenceKind                       string
	TargetRole                         string
	EvidencePolicies                   []runtimecontract.RuntimeEvidencePolicy
	ValidationRecipes                  []runtimecontract.RuntimeValidationRecipe
	CapabilityStates                   []runtimecontract.RuntimeCapabilityStateSnapshot
	CompletedRequiredOperationReceipts []runtimecontract.RuntimeRequiredOperationReceiptBinding
}

type RuntimeOperationExecutionResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       ExecutionEvidenceStatus
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

func (c *RuntimeController) ExecuteActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || executor == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.State != DispatchStateSealed {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime has no active sealed dispatch", common.ErrConflict)
	}
	dispatch := *aggregate.ActiveDispatch
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	completed := stringSet(dispatch.CompletedOperationIDs)
	for _, operation := range operations {
		if _, ok := completed[operation.ID]; ok {
			continue
		}
		for _, dependency := range operation.DependsOn {
			if _, ok := completed[dependency]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("operation %q dependency %q is not committed", operation.ID, dependency)
			}
		}
		// RVC-R10: claim fence before owner side effect. A claimed operation only
		// re-enters an owner when it is already safe-to-replay or carries an
		// owner-authored proof sealed before the claim; otherwise it becomes
		// outcome_unknown rather than guessing whether a durable effect happened.
		execution, hasExec := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operation.ID)
		if !hasExec {
			_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
			return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "execution ledger is missing")
		}
		var governanceGrant *runtimecontract.RuntimeOperationGovernanceGrant
		switch execution.State {
		case runtimecontract.OperationExecutionEvidenceCommitted:
			completed[operation.ID] = struct{}{}
			continue
		case runtimecontract.OperationExecutionOwnerCommitted:
			receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if !found {
				return RuntimeAggregate{}, fmt.Errorf("operation %q owner_committed without persisted receipt", operation.ID)
			}
			// A prerequisite-stale receipt is persisted before the aggregate
			// invalidation CAS. If that second CAS loses its response, restart
			// must finish the SM-2 invalidation from this durable receipt rather
			// than turn it into ordinary rejected evidence and request another
			// candidate for the stale authority.
			if receipt.PrerequisiteStale != nil {
				return c.InvalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, receipt)
			}
			if receipt.CapabilityStateStale != nil {
				return c.RefreshCapabilityStateAfterStaleReceipt(ctx, workspaceID, runID, dispatch.ID, receipt)
			}
			commit, commitErr := c.commitPersistedOperationReceiptEvidence(ctx, aggregate, dispatch, operation, receipt)
			if commitErr != nil {
				return RuntimeAggregate{}, commitErr
			}
			aggregate = commit.Aggregate
			completed[operation.ID] = struct{}{}
			if commit.DispatchCompleted {
				return aggregate, nil
			}
			continue
		case runtimecontract.OperationExecutionClaimed:
			if receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence); found {
				if receipt.PrerequisiteStale != nil {
					return c.InvalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, receipt)
				}
				if receipt.CapabilityStateStale != nil {
					return c.RefreshCapabilityStateAfterStaleReceipt(ctx, workspaceID, runID, dispatch.ID, receipt)
				}
				commit, commitErr := c.commitPersistedOperationReceiptEvidence(ctx, aggregate, dispatch, operation, receipt)
				if commitErr != nil {
					return RuntimeAggregate{}, commitErr
				}
				aggregate = commit.Aggregate
				completed[operation.ID] = struct{}{}
				if commit.DispatchCompleted {
					return aggregate, nil
				}
				// A restart may find a receipt persisted before its evidence. Once
				// that evidence is committed, continue the remaining sealed work
				// instead of forcing another drive or requesting a new candidate.
				continue
			}
			governanceGrant = governanceGrantForExecution(aggregate, execution)
			if governanceGrant == nil {
				idempotency := runtimecontract.ResolveOperationIdempotencyClass(operation, aggregate.Facts.Activation.MCPCapabilities)
				hasReplayProof := execution.ReplayProof != nil
				if idempotency != runtimecontract.RuntimeOperationIdempotencySafeToReplay && idempotency != runtimecontract.RuntimeOperationIdempotencyOwnerManaged && !hasReplayProof {
					_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
					return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "claimed execution has no receipt or replay authority")
				}
				recovered, consumed, recoverErr := c.ConsumeRecoveryBudget(ctx, workspaceID, runID, runtimecontract.RuntimeRecoveryBudgetOperationRecovery)
				if recoverErr != nil {
					return RuntimeAggregate{}, recoverErr
				}
				if !consumed {
					_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
					return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "safe replay recovery budget exhausted")
				}
				aggregate = recovered
				dispatch = *aggregate.ActiveDispatch
				execution, hasExec = operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
				if !hasExec || execution.State != runtimecontract.OperationExecutionClaimed {
					return RuntimeAggregate{}, fmt.Errorf("operation %q recovery lost claimed execution", operation.ID)
				}
			}
		case runtimecontract.OperationExecutionOutcomeUnknown:
			return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "blind replay is forbidden")
		}
		if execution.State == runtimecontract.OperationExecutionSealed {
			claimed, claimErr := c.claimOperationExecution(ctx, workspaceID, runID, operation.ID)
			if claimErr != nil {
				return RuntimeAggregate{}, claimErr
			}
			aggregate = claimed
			dispatch = *aggregate.ActiveDispatch
			execution, hasExec = operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
			if !hasExec || execution.State != runtimecontract.OperationExecutionClaimed {
				return RuntimeAggregate{}, fmt.Errorf("operation %q claim was not persisted", operation.ID)
			}
		}

		result, executeErr := executor.ExecuteRuntimeOperation(ctx, RuntimeOperationExecutionRequest{
			WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatch.ID,
			Transaction: dispatch.Transaction, Operation: operation, Execution: execution,
			GovernanceGrant:                    governanceGrant,
			EvidenceKind:                       evidenceKindForDecision(aggregate.Decision),
			TargetRole:                         aggregate.Facts.Focus.TargetRole,
			EvidencePolicies:                   aggregate.Facts.Activation.EvidencePolicies,
			ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(dispatch.ValidationRecipes),
			CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), aggregate.Facts.Resources.CapabilityStates...),
			CompletedRequiredOperationReceipts: completedRequiredOperationReceiptBindings(*aggregate.ActiveDispatch, aggregate.OperationReceipts, completed),
		})
		if executeErr != nil {
			// The owner may have committed its terminal receipt while the response was
			// lost. Reload exact ExecutionKey+Fence before declaring the outcome
			// unknown; otherwise a transport failure after receipt commit would erase
			// recoverable owner authority and strand the run.
			if recovered, found, recoverErr := c.recoverOperationReceiptAfterExecuteError(ctx, workspaceID, runID, dispatch, operation, execution); recoverErr != nil {
				return RuntimeAggregate{}, recoverErr
			} else if found {
				return recovered, nil
			}
			if execution.ReplayProof != nil {
				// Reload only persisted aggregate facts; the claimed-operation branch
				// consumes the single bounded recovery allowance before it re-enters
				// the owner with the original key/fence.
				return c.ExecuteActiveDispatch(ctx, workspaceID, runID, executor)
			}
			// No terminal receipt exists. The side effect may still have completed, so
			// fence the execution as outcome_unknown instead of blind replaying it.
			_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
			return RuntimeAggregate{}, executeErr
		}
		result.Receipt = result.Receipt.Normalized()
		if err := result.Receipt.Validate(); err != nil {
			return RuntimeAggregate{}, fmt.Errorf("owner receipt: %w", err)
		}
		if result.Receipt.ExecutionKey != execution.ExecutionKey || result.Receipt.OperationID != operation.ID {
			return RuntimeAggregate{}, fmt.Errorf("owner receipt does not match claimed execution")
		}
		if result.Receipt.PrerequisiteStale != nil {
			return c.InvalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, result.Receipt)
		}
		if result.Receipt.CapabilityStateStale != nil {
			return c.RefreshCapabilityStateAfterStaleReceipt(ctx, workspaceID, runID, dispatch.ID, result.Receipt)
		}
		// RVC-R11: pre-effect governance must not enter ordinary execution repair.
		if result.Governance != nil {
			aggregate, err = c.recordOperationGovernanceRequired(ctx, workspaceID, runID, operation.ID, result)
			if err != nil {
				return RuntimeAggregate{}, err
			}
			return aggregate, nil
		}
		if failure := runtimeOperationEffectMismatchFailure(aggregate.Decision, execution, operation, result.Receipt); failure != nil {
			result.Status = ExecutionEvidenceStatusRejected
			result.Failure = failure
		} else if failure := runtimeOperationEvidenceQualityFailure(operation, result.Receipt.Evidence); failure != nil {
			result.Status = ExecutionEvidenceStatusRejected
			result.Failure = failure
		}
		if err := validateRuntimeOperationExecutionResult(aggregate.Decision, operation, result); err != nil {
			return RuntimeAggregate{}, err
		}
		commit, commitErr := c.CommitEvidence(ctx, ExecutionEvidence{
			EventID: result.EventID, WorkspaceID: workspaceID, RunID: runID,
			DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
			OperationID: operation.ID, Kind: result.EvidenceKind, Status: result.Status,
			SourceRef: result.SourceRef, Failure: result.Failure, Evidence: result.Evidence,
			ObservedEffectScope: result.Receipt.ObservedEffectScope, NoOp: result.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp,
			OwnerReceiptRef:  "runtime-receipt/" + result.Receipt.ExecutionKey,
			OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(result.Receipt), Receipt: &result.Receipt,
		})
		if commitErr != nil {
			return RuntimeAggregate{}, commitErr
		}
		aggregate = commit.Aggregate
		completed[operation.ID] = struct{}{}
		if commit.DispatchCompleted {
			return aggregate, nil
		}
	}
	return aggregate, nil
}

func (c *RuntimeController) recoverOperationReceiptAfterExecuteError(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	dispatch SealedDispatch,
	operation runtimecontract.RuntimeOperation,
	execution runtimecontract.RuntimeOperationExecutionRecord,
) (RuntimeAggregate, bool, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found {
		return aggregate, false, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != dispatch.ID {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: recovered owner receipt has no matching active dispatch", common.ErrConflict)
	}
	persistedExecution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operation.ID)
	if !found || persistedExecution.State != runtimecontract.OperationExecutionOwnerCommitted ||
		persistedExecution.ExecutionKey != receipt.ExecutionKey || persistedExecution.Fence != receipt.Fence {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: recovered owner receipt does not match owner-committed execution", common.ErrConflict)
	}
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired {
		if receipt.Governance == nil {
			return RuntimeAggregate{}, false, fmt.Errorf("recovered governance receipt has no requirement")
		}
		recovered, recoverErr := c.recordOperationGovernanceRequired(ctx, workspaceID, runID, operation.ID, RuntimeOperationExecutionResult{
			OperationID: operation.ID,
			Receipt:     receipt,
			Governance:  receipt.Governance,
		})
		return recovered, true, recoverErr
	}
	commit, commitErr := c.commitPersistedOperationReceiptEvidence(ctx, aggregate, *aggregate.ActiveDispatch, operation, receipt)
	if commitErr != nil {
		return RuntimeAggregate{}, false, commitErr
	}
	return commit.Aggregate, true, nil
}

func governanceGrantForExecution(aggregate RuntimeAggregate, execution runtimecontract.RuntimeOperationExecutionRecord) *runtimecontract.RuntimeOperationGovernanceGrant {
	execution = execution.Normalized()
	for _, raw := range aggregate.OperationGovernanceBindings {
		binding := raw.Normalized()
		if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil || binding.ExecutionKey != execution.ExecutionKey {
			continue
		}
		grant := binding.Grant.Normalized()
		if grant.ResumeFence != execution.Fence {
			continue
		}
		return &grant
	}
	return nil
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
func (c *RuntimeController) recordOperationGovernanceRequired(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string, result RuntimeOperationExecutionResult) (RuntimeAggregate, error) {
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
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation governance requires active dispatch", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation governance requires owner_committed execution", common.ErrConflict)
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || persisted.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || !runtimecontract.SameRuntimeOperationReceipt(persisted, result.Receipt) {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation governance requires persisted governance receipt", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	gov := result.Governance.Normalized()
	// Preserve the exact operation identity before closing its dispatch. The later
	// ticket bind may attach governance dispatch/ticket information, but it must
	// never reconstruct an ExecutionKey or fence from kind-level data.
	aggregate.OperationGovernanceBindings = append(aggregate.OperationGovernanceBindings, runtimecontract.RuntimeOperationGovernanceBinding{
		OperationDispatchID:   aggregate.ActiveDispatch.ID,
		OperationID:           execution.OperationID,
		ExecutionKey:          execution.ExecutionKey,
		Fence:                 execution.Fence,
		ExpectedEffectScope:   execution.ExpectedEffectScope,
		DependencyKind:        string(gov.Kind),
		SubjectRef:            gov.SubjectRef,
		GovernanceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(persisted),
		State:                 runtimecontract.RuntimeOperationGovernanceBindingSuspended,
	})
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class:      runtimecontract.FailureClassGovernanceRequired,
		Code:       "approval_required",
		Signature:  "governance-" + string(gov.Kind) + "-" + gov.SubjectRef,
		Message:    firstNonEmpty(gov.Reason, "operation requires governance before side effects"),
		TargetRefs: []string{gov.SubjectRef},
		GovernanceDependency: &runtimecontract.ExternalDependency{
			Kind: string(gov.Kind), SubjectRef: gov.SubjectRef,
			Reason: firstNonEmpty(gov.Reason, "operation requires governance before side effects"),
		},
	}
	// Close active dispatch as committed-without-evidence so reduce can open governance.
	if aggregate.ActiveDispatch != nil {
		dispatch := *aggregate.ActiveDispatch
		dispatch.State = DispatchStateCommitted
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
	}
	facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if reduceErr != nil {
		return RuntimeAggregate{}, reduceErr
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

func (c *RuntimeController) commitPersistedOperationReceiptEvidence(ctx context.Context, aggregate RuntimeAggregate, dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) (EvidenceCommitResult, error) {
	status := ExecutionEvidenceStatusAccepted
	failure := receipt.Failure
	switch receipt.Outcome {
	case runtimecontract.RuntimeOperationOutcomeRejected:
		status = ExecutionEvidenceStatusRejected
	case runtimecontract.RuntimeOperationOutcomeAccepted, runtimecontract.RuntimeOperationOutcomeNoOp:
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
		if !found {
			return EvidenceCommitResult{}, fmt.Errorf("operation %q has no sealed execution record", operation.ID)
		}
		if effectFailure := runtimeOperationEffectMismatchFailure(aggregate.Decision, execution, operation, receipt); effectFailure != nil {
			status = ExecutionEvidenceStatusRejected
			failure = effectFailure
		} else if qualityFailure := runtimeOperationEvidenceQualityFailure(operation, receipt.Evidence); qualityFailure != nil {
			status = ExecutionEvidenceStatusRejected
			failure = qualityFailure
		}
	default:
		return EvidenceCommitResult{}, fmt.Errorf("operation receipt outcome %q cannot commit ordinary evidence", receipt.Outcome)
	}
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	return c.CommitEvidence(ctx, ExecutionEvidence{
		EventID: "runtime-receipt|" + hash, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Status: status, SourceRef: "runtime-receipt/" + receipt.ExecutionKey,
		Failure: failure, Evidence: receipt.Evidence, ObservedEffectScope: receipt.ObservedEffectScope, NoOp: receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp,
		OwnerReceiptRef: "runtime-receipt/" + receipt.ExecutionKey, OwnerReceiptHash: hash, Receipt: &receipt,
	})
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
	coverage := make([]string, 0, len(operation.CoverageWorkItemIDs))
	for _, id := range operation.CoverageWorkItemIDs {
		coverage = append(coverage, string(id))
	}
	return &runtimecontract.FailureFact{
		Class:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
		Code:      "validation_evidence_inconclusive",
		Signature: strings.Join([]string{"validation-evidence", operation.Kind, qualityLabel, strings.Join(coverage, ",")}, "/"),
		Message:   reason, TargetRefs: append([]string(nil), operation.TargetRefs...),
	}
}

func (c *RuntimeController) claimOperationExecution(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: no active dispatch for claim", common.ErrConflict)
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
			exec.State = runtimecontract.OperationExecutionClaimed
			exec.Fence = exec.Fence + 1
			if exec.Fence == 0 {
				exec.Fence = 1
			}
			exec.ClaimedAt = "claimed"
		case runtimecontract.OperationExecutionClaimed:
			return RuntimeAggregate{}, fmt.Errorf("operation %q is already claimed; query receipt before recovery", operationID)
		case runtimecontract.OperationExecutionOwnerCommitted, runtimecontract.OperationExecutionEvidenceCommitted:
			return aggregate, nil
		case runtimecontract.OperationExecutionOutcomeUnknown:
			return RuntimeAggregate{}, operationOutcomeUnknown(operationID, "execution cannot be reclaimed for replay")
		}
	}
	if !found {
		// A persisted dispatch without an execution ledger is an invariant defect.
		// Fence it as unknown so it can never be silently replayed.
		scope := runtimecontract.RuntimeEffectUnknown
		for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
			if operation.Normalized().ID == operationID {
				scope = runtimecontract.ResolveExpectedEffectScope(operation, aggregate.Facts.Activation.MCPCapabilities)
				break
			}
		}
		aggregate.ActiveDispatch.OperationExecutions = append(aggregate.ActiveDispatch.OperationExecutions, runtimecontract.RuntimeOperationExecutionRecord{
			OperationID:         operationID,
			ExecutionKey:        runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, aggregate.ActiveDispatch.ID, operationID),
			ExpectedEffectScope: scope,
			State:               runtimecontract.OperationExecutionOutcomeUnknown,
			Fence:               1,
		})
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("operation %q execution ledger is missing; outcome is unknown", operationID)
	}
	return aggregate, nil
}

func (c *RuntimeController) markOperationExecutionOwnerCommitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID, eventID string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: no active dispatch for owner commit", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		exec := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if exec.OperationID != operationID {
			continue
		}
		exec.State = runtimecontract.OperationExecutionOwnerCommitted
		if exec.Fence == 0 {
			exec.Fence = 1
		}
		exec.OwnerReceiptRef = "event/" + strings.TrimSpace(eventID)
		exec.OwnerReceiptHash = strings.TrimSpace(eventID)
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

func (c *RuntimeController) markOperationExecutionOutcomeUnknown(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, error) {
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
		exec.State = runtimecontract.OperationExecutionOutcomeUnknown
		if exec.Fence == 0 {
			exec.Fence = 1
		}
	}
	if !found {
		scope := runtimecontract.RuntimeEffectUnknown
		for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
			if operation.Normalized().ID == operationID {
				scope = runtimecontract.ResolveExpectedEffectScope(operation, aggregate.Facts.Activation.MCPCapabilities)
				break
			}
		}
		aggregate.ActiveDispatch.OperationExecutions = append(aggregate.ActiveDispatch.OperationExecutions, runtimecontract.RuntimeOperationExecutionRecord{
			OperationID: operationID, ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, aggregate.ActiveDispatch.ID, operationID),
			ExpectedEffectScope: scope, State: runtimecontract.OperationExecutionOutcomeUnknown, Fence: 1,
		})
	}
	aggregate.StateRevision++
	_ = c.transitions.Commit(ctx, aggregate, expectedRevision)
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
