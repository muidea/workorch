package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *driveDeliveryComponent) requestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	kind = kind.Normalized()
	requestedBy = strings.TrimSpace(requestedBy)
	reason = strings.TrimSpace(reason)
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || requestedBy == "" || reason == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Termination != nil {
		if isTerminalConvergenceState(aggregate.Decision.State) || aggregate.Decision.Terminal != nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: convergence and administrative terminals cannot coexist", common.ErrConflict)
		}
		current := aggregate.Termination.Normalized()
		if current.Kind == kind && current.RequestedBy == requestedBy && current.Reason == reason {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: termination already recorded with a different exact intent", common.ErrConflict)
	}
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept administrative termination", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	if err := retireRuntimeRecoverableWaitForTermination(&aggregate, kind); err != nil {
		return RuntimeAggregate{}, err
	}
	// Any prior scheduled/running drive is superseded by this administrative
	// delivery handoff. Its stale worker is generation-fenced; if its in-flight
	// owner delivery subsequently fails, only the canonical terminal-delivery
	// path may replace the retired generation with G+1.
	if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning {
		aggregate.Facts.Recovery.RetiredTerminalDeliveryLease = aggregate.DriveLease.Clone()
		aggregate.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID = ""
	} else {
		aggregate.Facts.Recovery.RetiredTerminalDeliveryLease = runtimecontract.RuntimeDriveLease{}
		aggregate.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID = ""
	}
	aggregate.DriveLease = nil
	for idx := range aggregate.OperationRecoveryAuthorizations {
		if aggregate.OperationRecoveryAuthorizations[idx].State == runtimecontract.RuntimeOperationRecoveryAuthorizationIssued {
			aggregate.OperationRecoveryAuthorizations[idx].State = runtimecontract.RuntimeOperationRecoveryAuthorizationRetired
		}
		if aggregate.OperationRecoveryAuthorizations[idx].State == runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight {
			aggregate.OperationRecoveryAuthorizations[idx].State = runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain
		}
	}
	term := runtimecontract.RuntimeTerminationRecord{
		Kind: kind, RequestedBy: requestedBy, Reason: reason,
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

func retireRuntimeRecoverableWaitForTermination(aggregate *RuntimeAggregate, kind runtimecontract.RuntimeTerminationKind) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	idx, wait, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits)
	if !found {
		return nil
	}
	allocation, found := runtimecontract.RuntimeBudgetAllocationForID(aggregate.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if !found {
		return fmt.Errorf("%w: active recoverable wait has no budget allocation", common.ErrConflict)
	}
	ownerFact := "administrative-termination/" + string(kind) + "/" + wait.WaitID
	switch allocation.State {
	case runtimecontract.RuntimeBudgetAllocationReserved:
		if wait.Status == runtimecontract.RuntimeRecoverableWaitWaiting {
			ledger, _, err := runtimecontract.ReleaseRuntimeBudgetAllocation(aggregate.ExecutionBudget, wait.BudgetAllocationID, ownerFact)
			if err != nil {
				return err
			}
			aggregate.ExecutionBudget = ledger
			aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExpired
			return nil
		}
		// A claimed probe may already have crossed the owner boundary. Preserve
		// conservative consumption before closing its reconciliation; cancellation
		// cannot locally prove that the physical call did not occur.
		unknownHash := runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash)
		if err := markRecoverableWaitProbeUnknown(aggregate, wait, unknownHash); err != nil {
			return err
		}
		if err := failRecoverableWaitBudgetReconciliation(aggregate, wait, unknownHash); err != nil {
			return err
		}
		aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExhausted
		return nil
	case runtimecontract.RuntimeBudgetAllocationUnknown:
		unknownHash := runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash)
		if err := failRecoverableWaitBudgetReconciliation(aggregate, wait, unknownHash); err != nil {
			return err
		}
		aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExhausted
		return nil
	default:
		return fmt.Errorf("%w: active recoverable wait has terminal budget allocation %q", common.ErrConflict, allocation.State)
	}
}
