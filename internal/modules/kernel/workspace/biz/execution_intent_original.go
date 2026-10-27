package workspace

import (
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func unresolvedFreshExecutionIntent(p preparedWorkspaceExecutionIntent, original workspacemodel.WorkspaceFreshIntentReservation) workspaceevents.WorkspaceExecutionIntentResult {
	return workspaceevents.WorkspaceExecutionIntentResult{
		WorkspaceID: p.workspace.ID, RequestID: p.command.RequestID, Kind: p.command.Kind,
		Outcome: runtimecontract.RuntimeExecutionIntentFreshUnresolved, Reason: runtimecontract.RuntimeExecutionIntentReasonFreshResultUnknown,
		PendingRequestID: original.RequestID, PendingStartupInputHash: original.StartupInputHash, PendingPayloadHash: original.PayloadHash,
	}
}

// An existing identity is handled exclusively by the read-only original Run
// receipt consumer and Workspace settlement. Never call Restart replay here.
func (m *Biz) resolveOriginalFreshExecutionIntent(p preparedWorkspaceExecutionIntent) (workspaceevents.WorkspaceExecutionIntentResult, bool, error) {
	if p.command.Kind != runtimecontract.RuntimeExecutionIntentRestart || p.workspace.PreparationFence == nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, false, nil
	}
	fence := p.workspace.PreparationFence
	if fence.Validate(p.workspace.ID) != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, false, common.ErrConflict
	}
	var original *workspacemodel.WorkspaceFreshIntentReservation
	for _, settled := range fence.Settled {
		if settled.Reservation.RequestID == p.command.RequestID {
			value := settled.Reservation
			original = &value
			break
		}
	}
	if original == nil && fence.Pending != nil {
		if fence.Pending.RequestID != p.command.RequestID {
			return unresolvedFreshExecutionIntent(p, *fence.Pending), true, nil
		}
		original = fence.Pending
	}
	if original == nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, false, nil
	}
	receipt, err := m.reconcileWorkspaceFreshIntent(p.ctx, p.command)
	if err != nil {
		return workspaceevents.WorkspaceExecutionIntentResult{}, true, err
	}
	if receipt == nil {
		return unresolvedFreshExecutionIntent(p, *original), true, nil
	}
	return workspaceevents.WorkspaceExecutionIntentResult(receipt.Admission), true, nil
}
