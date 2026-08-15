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
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.DefineDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		cmd, ok := ev.Data().(workspaceevents.UpdateDoDCommand)
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.UpdateDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		cmd, ok := ev.Data().(workspaceevents.DeleteDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, cmd.DoDID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		cmd, ok := ev.Data().(workspaceevents.GetDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoDs: workspacemodel.CloneDoDs(items)}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}
