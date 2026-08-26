package workspace

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeDoDCommand(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandDefineDoD:
		cmd, ok := ev.Data().(workspaceevents.DefineDoDCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.DefineDoD(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		cmd, ok := ev.Data().(workspaceevents.UpdateDoDCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.UpdateDoD(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		cmd, ok := ev.Data().(workspaceevents.DeleteDoDCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.DeleteDoD(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		cmd, ok := ev.Data().(workspaceevents.GetDoDCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		hidden, err := m.workspaceDeletionReadHidden(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if hidden {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrNotFound
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		hidden, err := m.workspaceDeletionReadHidden(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if hidden {
			return workspaceevents.WorkspaceCommandResult{DoDs: []*workspacemodel.DoD{}}, nil
		}
		items, err := m.dods.ListByWorkspaceID(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoDs: workspacemodel.CloneDoDs(items)}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}
