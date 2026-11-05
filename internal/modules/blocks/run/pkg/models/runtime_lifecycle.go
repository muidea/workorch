package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeLifecycleState is the SM-1 identity needed by the Runtime drive and
// its next CAS. It contains no aggregate, bootstrap body or business projection.
type RuntimeLifecycleState struct {
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	State                common.RunState
	RuntimeStateRevision uint64
	BootstrapHash        string
	LifecycleFence       *rc.RuntimeOpaqueLifecycleFence
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	ReconciliationScope  rc.RuntimeCWIReconciliationScope
	ActivationRequestID  string
}

// RuntimeLifecycleFromRun borrows only under the repository read lock and
// returns detached metadata, never the opaque Runtime graph.
func RuntimeLifecycleFromRun(item *Run) (RuntimeLifecycleState, error) {
	if item == nil || item.RuntimeBootstrap == nil {
		return RuntimeLifecycleState{}, common.ErrConflict
	}
	v := RuntimeLifecycleState{WorkspaceID: item.WorkspaceID, RunID: item.ID, State: item.State,
		BootstrapHash: rc.StableRuntimeBootstrapHash(*item.RuntimeBootstrap), BlockingTicketID: item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID, ReconciliationScope: item.ReconciliationScope, ActivationRequestID: item.ActivationRequestID}
	if item.RuntimeState != nil {
		v.RuntimeStateRevision = item.RuntimeState.StateRevision
	}
	if item.RuntimeLifecycleFence != nil {
		f := *item.RuntimeLifecycleFence
		v.LifecycleFence = &f
	}
	return v, nil
}
