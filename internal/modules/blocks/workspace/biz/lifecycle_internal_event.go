package workspace

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceLifecycleInternalEventKind string

const (
	workspaceLifecycleInternalEventTerminalDeliveryObserved workspaceLifecycleInternalEventKind = "terminal_delivery_observed"
	workspaceLifecycleInternalEventRunProjectionObserved    workspaceLifecycleInternalEventKind = "run_projection_observed"
)

type workspaceLifecycleInternalEvent struct {
	Kind             workspaceLifecycleInternalEventKind
	TerminalDelivery *workspaceevents.RuntimeTerminalDeliveryFact
	RunProjection    *workspaceevents.RunProjectionFact
}

// submitWorkspaceLifecycleInternalEvent is the only owner-local dispatcher
// allowed to select a Workspace lifecycle reducer. Cross-owner handlers first
// validate an intent/fact and then translate it into one of these private
// events; callers cannot submit a target Workspace state.
func (m *Biz) submitWorkspaceLifecycleInternalEvent(ctx context.Context, event workspaceLifecycleInternalEvent) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	if m == nil || m.workspaces == nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceLifecycleInternalEventTerminalDeliveryObserved:
		if event.TerminalDelivery == nil || event.RunProjection != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		return m.applyTerminalDelivery(ctx, *event.TerminalDelivery)
	case workspaceLifecycleInternalEventRunProjectionObserved:
		if event.RunProjection == nil || event.TerminalDelivery != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		return m.applyRunProjection(ctx, *event.RunProjection)
	default:
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
}
