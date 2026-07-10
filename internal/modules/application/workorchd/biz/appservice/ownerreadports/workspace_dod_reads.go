package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDoDByOwnerCommand(ctx context.Context, hub magicevent.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	if hub == nil || dodID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[workspaceevents.GetDoDCommand, workspaceevents.WorkspaceCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, workspaceevents.TopicWorkspaceCommandGetDoD, workspaceevents.CommandDestination, "dod", string(dodID), workspaceevents.GetDoDCommand{
		WorkspaceID: workspaceID,
		DoDID:       dodID,
	})
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.DoD, nil
}

func ListDoDsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.ListDoDsByWorkspaceCommand, workspaceevents.WorkspaceCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, workspaceevents.TopicWorkspaceCommandListDoDsByWS, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceevents.ListDoDsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.DoDs, nil
}
