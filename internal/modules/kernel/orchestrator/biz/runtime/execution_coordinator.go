package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeOperationRecoveryBudgetRequiredError is a read-only handoff from
// SM-3 execution to the SM-4 recovery-budget event. It deliberately carries no
// aggregate or mutation callback: the next event must reload the aggregate and
// own its independent CAS.
type runtimeOperationRecoveryBudgetRequiredError struct {
	OperationID string
}

func (e *runtimeOperationRecoveryBudgetRequiredError) Error() string {
	if e == nil || e.OperationID == "" {
		return "runtime operation recovery budget is required"
	}
	return fmt.Sprintf("runtime operation %q requires recovery budget", e.OperationID)
}

// RuntimeOperationRecoveryBudgetRequired classifies the owner-local handoff
// without exposing a mutation entry or the concrete error type to Biz.
func RuntimeOperationRecoveryBudgetRequired(err error) (string, bool) {
	var required *runtimeOperationRecoveryBudgetRequiredError
	if !errors.As(err, &required) || required == nil || required.OperationID == "" {
		return "", false
	}
	return required.OperationID, true
}

func runtimeOperationRecoveryBudgetAvailable(aggregate RuntimeAggregate) bool {
	policy := aggregate.Facts.RecoveryPolicy.Normalized()
	return aggregate.Facts.Recovery.OperationRecoveryAttempts < policy.MaxOperationRecoveryAttempts
}

func (c *candidateDispatchComponent) executeActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor, recoveryOperationID string, recoveryBudgetAttempt int) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || executor == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.State != DispatchStateSealed {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime has no active sealed dispatch", common.ErrConflict)
	}
	// Owner execution is downstream of the same canonical CWI fence used by
	// candidate, dispatch, receipt, and evidence admission. Validate before any
	// claim/recovery branch can call a capability owner.
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: operation execution binding fence: %v", common.ErrConflict, err)
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
				return c.invalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, receipt)
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
					return c.invalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, receipt)
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
			governanceGrant = governancecheck.GrantForExecution(aggregate.OperationGovernanceBindings, execution)
			if governanceGrant == nil {
				idempotency := runtimecontract.ResolveOperationIdempotencyClass(operation, aggregate.Facts.Activation.MCPCapabilities)
				hasReplayProof := execution.ReplayProof != nil
				if idempotency != runtimecontract.RuntimeOperationIdempotencySafeToReplay && idempotency != runtimecontract.RuntimeOperationIdempotencyOwnerManaged && !hasReplayProof {
					_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
					return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "claimed execution has no receipt or replay authority")
				}
				// The persisted counter is an exact handoff fence.  A later, unrelated
				// recovery charge must not make an old operation token valid again.
				authorized := strings.TrimSpace(recoveryOperationID) == operation.ID && recoveryBudgetAttempt > 0 && aggregate.Facts.Recovery.OperationRecoveryAttempts == recoveryBudgetAttempt
				if authorized {
					recoveryOperationID = ""
					recoveryBudgetAttempt = 0
				} else if !runtimeOperationRecoveryBudgetAvailable(aggregate) {
					_, _ = c.markOperationExecutionOutcomeUnknown(ctx, workspaceID, runID, operation.ID)
					return RuntimeAggregate{}, operationOutcomeUnknown(operation.ID, "safe replay recovery budget exhausted")
				}
				if !authorized {
					return RuntimeAggregate{}, &runtimeOperationRecoveryBudgetRequiredError{OperationID: operation.ID}
				}
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
				return c.executeActiveDispatch(ctx, workspaceID, runID, executor, "", 0)
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
			return c.invalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatch.ID, result.Receipt)
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
		commit, commitErr := c.commitEvidence(ctx, ExecutionEvidence{
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

func (c *candidateDispatchComponent) recoverOperationReceiptAfterExecuteError(
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
