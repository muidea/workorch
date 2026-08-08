package workspace

import (
	"context"
	"errors"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) lifecycleTopics() []string {
	return []string{
		workspaceevents.TopicWorkspaceFactTerminalDelivery,
	}
}

func (m *Biz) handleLifecycleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeLifecycleCommand(ev)
	setWorkspaceLifecycleResult(result, response, err)
}

func setWorkspaceLifecycleResult(result magicevent.Result, data workspaceevents.WorkspaceLifecycleCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Biz) executeLifecycleCommand(ev magicevent.Event) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	if m == nil || m.workspaces == nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceFactTerminalDelivery:
		cmd, ok := ev.Data().(workspaceevents.RuntimeTerminalDeliveryFact)
		if !ok || ev.Source() != "kernel/orchestrator" {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		boundCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		return m.submitWorkspaceLifecycleInternalEvent(boundCtx, workspaceLifecycleInternalEvent{
			Kind:             workspaceLifecycleInternalEventTerminalDeliveryObserved,
			TerminalDelivery: &cmd,
		})
	default:
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) applyRunProjection(ctx context.Context, cmd workspaceevents.RunProjectionFact) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		transition, err := reduceRunProjectionFact(current, cmd)
		if err != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		if transition.Noop {
			return workspaceevents.WorkspaceLifecycleCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(current), Transitioned: false}, nil
		}
		next := applyWorkspaceProjection(current, transition)
		if err := m.workspaces.CompareAndSwap(ctx, next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		m.publishWorkspaceUpdated(ctx, next)
		return workspaceevents.WorkspaceLifecycleCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(next), Transitioned: true}, nil
	}
	return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrConflict
}

func (m *Biz) applyTerminalDelivery(ctx context.Context, cmd workspaceevents.RuntimeTerminalDeliveryFact) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		transition, err := reduceWorkspaceTerminalDelivery(current, cmd)
		if err != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		if transition.Noop {
			return workspaceevents.WorkspaceLifecycleCommandResult{
				Workspace: workspaceeventmapping.WorkspaceDTO(current), Transitioned: false,
				DeliveryAck: workspaceTerminalDeliveryAck(current, cmd, runtimecontract.RuntimeDeliveryAcknowledgedNoop),
			}, nil
		}
		next := applyWorkspaceProjection(current, transition)
		if err := m.workspaces.CompareAndSwap(ctx, next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.WorkspaceLifecycleCommandResult{}, err
		}
		m.publishWorkspaceUpdated(ctx, next)
		return workspaceevents.WorkspaceLifecycleCommandResult{
			Workspace: workspaceeventmapping.WorkspaceDTO(next), Transitioned: true,
			DeliveryAck: workspaceTerminalDeliveryAck(next, cmd, runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased),
		}, nil
	}
	return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrConflict
}

func workspaceTerminalDeliveryAck(
	ws *workspacemodel.Workspace,
	cmd workspaceevents.RuntimeTerminalDeliveryFact,
	state runtimecontract.RuntimeDeliveryAcknowledgedState,
) *runtimecontract.RuntimeDeliveryAcknowledgement {
	if ws == nil {
		return nil
	}
	cmd = cmd.Normalized()
	ack := runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID:         ws.ID,
		RunID:               cmd.RunID,
		TerminalDeliveryID:  cmd.TerminalDeliveryID,
		Owner:               runtimecontract.RuntimeDeliveryOwnerWorkspace,
		AcknowledgedState:   state,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		CausationID:         cmd.CausationID,
		Outcome:             cmd.Outcome,
	}.Normalized()
	return &ack
}
