package runtimev2

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RefreshCapabilityStateAfterStaleReceipt atomically abandons the sealed
// dispatch that observed external owner-state drift and returns to the
// read-only prerequisite path. It intentionally does not create evidence,
// candidate retry, or focus-repair debt.
func (c *RuntimeController) RefreshCapabilityStateAfterStaleReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if err := receipt.Validate(); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("capability-state refresh requires a valid stale receipt: %w", err)
	}
	if receipt.CapabilityStateStale == nil {
		return RuntimeAggregate{}, fmt.Errorf("capability-state refresh requires a stale receipt detail")
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != strings.TrimSpace(dispatchID) {
		// The stale receipt may be delivered again after the first delivery
		// atomically abandoned the dispatch and reopened its prerequisite.  The
		// owner receipt is already durable in that case, so treat the exact same
		// receipt as an idempotent diagnostic instead of charging a second
		// refresh budget or attempting to invalidate a newer snapshot.
		if capabilityStateStaleRefreshAlreadyApplied(aggregate, dispatchID, receipt) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt does not match active dispatch", common.ErrConflict)
	}
	if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence); !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt is not owner-persisted", common.ErrConflict)
	}
	detail := *receipt.CapabilityStateStale
	var matched *runtimecontract.RuntimeCapabilityStateSnapshot
	kept := make([]runtimecontract.RuntimeCapabilityStateSnapshot, 0, len(aggregate.Facts.Resources.CapabilityStates))
	for _, snapshot := range aggregate.Facts.Resources.CapabilityStates {
		if snapshot.SnapshotID == detail.SnapshotID && snapshot.ContentHash == detail.ExpectedHash &&
			snapshot.OwnerModuleID == detail.StateRef.OwnerModuleID && snapshot.CapabilityStateKind == detail.StateRef.CapabilityStateKind && snapshot.SubjectRef == detail.StateRef.SubjectRef {
			value := snapshot
			matched = &value
			continue
		}
		kept = append(kept, snapshot)
	}
	if matched == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt does not match an active capability snapshot", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	ledger, consumed, err := aggregate.Facts.Recovery.Consume(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, aggregate.Facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !consumed {
		aggregate.Facts.Resources.CapabilityStates = kept
		aggregate.Facts = aggregate.Facts.Normalized()
		decision, sealErr := runtimecontract.SealRuntimeDecision(aggregate.Facts, terminalDecision(runtimecontract.RuntimeDecision{Focus: aggregate.Facts.Focus}, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "capability_state_refresh_exhausted", "capability-state refresh budget exhausted"), 0)
		if sealErr != nil {
			return RuntimeAggregate{}, sealErr
		}
		aggregate.Decision = decision
		abandoned := *aggregate.ActiveDispatch
		abandoned.State = DispatchStateCommitted
		abandoned.CompletedOperationIDs = appendNormalizedString(abandoned.CompletedOperationIDs, receipt.OperationID)
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, abandoned)
		aggregate.ActiveDispatch = nil
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return RuntimeAggregate{}, err
		}
		if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
			return RuntimeAggregate{}, err
		}
		return aggregate, nil
	}
	aggregate.Facts.Resources.CapabilityStates = kept
	aggregate.Facts.Recovery = ledger
	aggregate.Facts.Prerequisites.Pending = append(aggregate.Facts.Prerequisites.Pending, runtimecontract.RuntimePrerequisite{
		ID: matched.RequirementID, Kind: runtimecontract.PrerequisiteKindCapabilityState,
		OwnerModuleID: matched.OwnerModuleID, CapabilityStateKind: matched.CapabilityStateKind, SubjectRef: matched.SubjectRef,
		Reason: "capability state refresh required after owner-observed stale snapshot",
	})
	aggregate.Facts.Prerequisites.Satisfied = removeSatisfiedCapabilityStatePrerequisite(aggregate.Facts.Prerequisites.Satisfied, matched.RequirementID)
	aggregate.Facts = aggregate.Facts.Normalized()
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	abandoned := *aggregate.ActiveDispatch
	abandoned.State = DispatchStateCommitted
	abandoned.CompletedOperationIDs = appendNormalizedString(abandoned.CompletedOperationIDs, receipt.OperationID)
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, abandoned)
	aggregate.ActiveDispatch = nil
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func capabilityStateStaleRefreshAlreadyApplied(aggregate RuntimeAggregate, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) bool {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return false
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
	if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, receipt) {
		return false
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID != dispatchID || dispatch.State != DispatchStateCommitted {
			continue
		}
		for _, execution := range dispatch.OperationExecutions {
			if execution.OperationID == receipt.OperationID && execution.ExecutionKey == receipt.ExecutionKey && execution.Fence == receipt.Fence {
				return true
			}
		}
	}
	return false
}

func removeSatisfiedCapabilityStatePrerequisite(values []runtimecontract.RuntimePrerequisite, prerequisiteID string) []runtimecontract.RuntimePrerequisite {
	ret := make([]runtimecontract.RuntimePrerequisite, 0, len(values))
	for _, value := range values {
		if value.ID == prerequisiteID && value.Kind == runtimecontract.PrerequisiteKindCapabilityState {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}
