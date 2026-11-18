package workspace

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeDoDCommand(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandDefineDoD:
		cmd, ok := ev.Data().(workspaceevents.DefineDoDCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.manager.DefineDoD(scopedCtx, cmd); err != nil {
			return workspaceCommandResponse{}, err
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		cmd, ok := ev.Data().(workspaceevents.UpdateDoDCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.manager.UpdateDoD(scopedCtx, cmd); err != nil {
			return workspaceCommandResponse{}, err
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		cmd, ok := ev.Data().(workspaceevents.DeleteDoDCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if err := m.manager.DeleteDoD(scopedCtx, cmd); err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{}, nil
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		cmd, ok := ev.Data().(workspaceevents.GetDoDCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		_, dods, visible, err := m.readVisibleWorkspaceFormalDefinitions(scopedCtx, cmd.WorkspaceID, false, false)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if !visible {
			return workspaceCommandResponse{}, common.ErrNotFound
		}
		for _, dod := range dods {
			if dod != nil && dod.ID == cmd.DoDID {
				return workspaceCommandResponse{DoD: workspacemodel.CloneDoD(dod)}, nil
			}
		}
		item, err := m.dods.Get(scopedCtx, cmd.DoDID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		return workspaceCommandResponse{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		_, items, visible, err := m.readVisibleWorkspaceFormalDefinitions(scopedCtx, cmd.WorkspaceID, false, true)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if !visible {
			return workspaceCommandResponse{DoDs: []*workspacemodel.DoD{}}, nil
		}
		return workspaceCommandResponse{DoDs: workspacemodel.CloneDoDs(items)}, nil
	default:
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
}
