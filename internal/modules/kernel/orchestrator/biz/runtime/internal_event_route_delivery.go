package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// applyDeliveryInternalEvent reduces the SM-4 payload family without gaining
// any authority to rebuild SM-2 or SM-3 state.
func (c *RuntimeController) applyDeliveryInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventReconciliationRequested:
		if event.Delivery.ReconciliationRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.driveDelivery.ensureReconciliationRequest(ctx, *event.Delivery.ReconciliationRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDriveFailureObserved:
		if event.Delivery.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordDriveFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.FailureFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventTerminalDeliveryAcknowledged:
		if event.Delivery.DeliveryAcknowledgement == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, changed, err := c.driveDelivery.recordDeliveryAcknowledgement(ctx, *event.Delivery.DeliveryAcknowledgement)
		result.Aggregate = aggregate
		result.BudgetConsumed = changed
		return result, err
	case RuntimeInternalEventRecoveryBudgetConsumed:
		aggregate, consumed, err := c.driveDelivery.consumeRecoveryBudget(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoveryBudgetKind)
		result.Aggregate = aggregate
		result.BudgetConsumed = consumed
		return result, err
	case RuntimeInternalEventDriveLeaseScheduled:
		if event.Delivery.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, changed, err := c.driveDelivery.scheduleDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.DriveLease)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseClaimed:
		if event.Delivery.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, claimed, err := c.driveDelivery.claimDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.DriveLease, event.Delivery.LeaseNow)
		result.Aggregate, result.LeaseClaimed = aggregate, claimed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseReleased:
		aggregate, changed, err := c.driveDelivery.releaseDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.LeaseGeneration)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		return result, err
	case RuntimeInternalEventDriveLeaseRecovered:
		aggregate, lease, changed, err := c.driveDelivery.recoverDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.LeaseNow)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventRecoverableWaitOpened:
		if event.Delivery.RecoverableWait == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.driveDelivery.openRecoverableWait(ctx, *event.Delivery.RecoverableWait)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRecoverableWaitProbeDue:
		aggregate, claimed, err := c.driveDelivery.claimRecoverableWaitProbe(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoverableWaitID, event.Delivery.LeaseGeneration, event.Delivery.LeaseNow)
		result.Aggregate, result.LeaseClaimed = aggregate, claimed
		return result, err
	case RuntimeInternalEventRecoverableWaitProbeDeferred:
		aggregate, err := c.driveDelivery.deferRecoverableWaitProbe(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoverableWaitID, event.Delivery.LeaseNow, event.Delivery.RecoverableWaitProbeResultHash)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRecoverableWaitExhausted:
		aggregate, err := c.driveDelivery.exhaustRecoverableWait(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoverableWaitID, event.Delivery.RecoverableWaitProbeResultHash, event.Delivery.Reason)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRecoverableWaitRetired:
		aggregate, err := c.driveDelivery.retireRecoverableWait(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoverableWaitID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventTerminationRequested:
		aggregate, err := c.driveDelivery.requestTermination(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.TerminationKind, event.Delivery.RequestedBy, event.Delivery.Reason)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}
