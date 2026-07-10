package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDoD(ctx context.Context, hub magicevent.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	return ownerreadports.ReadDoDByOwnerCommand(ctx, hub, dodID)
}

func ListDoDsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	return ownerreadports.ListDoDsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func DefineDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	response, err := sendDefinitionMagicCommand[workspaceevents.DefineDoDCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandDefineDoD, workspaceevents.CommandDestination, "dod", string(dodID), workspaceID, workspaceevents.DefineDoDCommand{
		DoDID:       dodID,
		WorkspaceID: workspaceID,
		Items:       append([]string(nil), items...),
	})
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.DoD, nil
}

func UpdateDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	response, err := sendDefinitionMagicCommand[workspaceevents.UpdateDoDCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateDoD, workspaceevents.CommandDestination, "dod", string(dodID), workspaceID, workspaceevents.UpdateDoDCommand{
		WorkspaceID: workspaceID,
		DoDID:       dodID,
		Items:       append([]string(nil), items...),
	})
	if err != nil {
		return nil, err
	}
	if response.DoD == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.DoD, nil
}

func DeleteDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	_, err := sendDefinitionMagicCommand[workspaceevents.DeleteDoDCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandDeleteDoD, workspaceevents.CommandDestination, "dod", string(dodID), workspaceID, workspaceevents.DeleteDoDCommand{WorkspaceID: workspaceID, DoDID: dodID})
	return err
}
