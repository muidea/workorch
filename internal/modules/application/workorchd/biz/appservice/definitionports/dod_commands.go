package definitionports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadDoD(ctx context.Context, hub events.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	return ownerreadports.ReadDoDByOwnerCommand(ctx, hub, dodID)
}

func ListDoDsByWorkspace(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	return ownerreadports.ListDoDsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func DefineDoD(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	response, err := ownercommandports.PublishWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandDefineDoD, "dod", string(dodID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.DefineDoDCommand{
			DoDID:       dodID,
			WorkspaceID: workspaceID,
			Items:       append([]string(nil), items...),
		},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func UpdateDoD(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (*workspacemodel.DoD, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	response, err := ownercommandports.PublishWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateDoD, "dod", string(dodID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.UpdateDoDCommand{
			DoDID: dodID,
			Items: append([]string(nil), items...),
		},
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func DeleteDoD(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, dodID common.DoDID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	return ownercommandports.Publish(ctx, hub, workspaceevents.TopicWorkspaceCommandDeleteDoD, "dod", string(dodID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand:     workspacecontracts.DeleteDoDCommand{DoDID: dodID},
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
}
