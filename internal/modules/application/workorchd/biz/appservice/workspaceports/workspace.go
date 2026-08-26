package workspaceports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceDeletionIngressSource = "application/workorchd"

func ReadWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, workspaceID)
}

func ListWorkspaceRecords(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.Workspace, error) {
	return ownerreadports.ListWorkspacesByOwnerCommand(ctx, hub)
}

func CreateWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, title string, description string, deliverableDirectory string, internalDirectory string) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	command := workspaceevents.CreateWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandCreate)
	if err != nil {
		return err
	}
	return response.ValidateFor(command)
}

func UpdateWorkspaceRecord(ctx context.Context, hub magicevent.Hub, item *workspacemodel.Workspace) error {
	if hub == nil || item == nil {
		return common.ErrInvalidArgument
	}
	command := workspaceevents.UpdateWorkspaceCommand{
		WorkspaceID:          item.ID,
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultLLMSelection:  runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection),
		DefaultSoulID:        item.DefaultSoulID,
		SoulBindings:         soulbinding.FromSoulIDMap(item.SoulBindings),
		ContextHints:         item.ContextHints.Clone(),
		EvidencePolicies:     workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdate, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdate)
	if err != nil {
		return err
	}
	return response.ValidateFor(command)
}

func UpdateWorkspaceLLMSelection(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	var cloned *runtimecontract.ProviderInferenceSelection
	if selection != nil {
		value := selection.Normalized()
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		cloned = &value
	}
	command := workspaceevents.UpdateWorkspaceLLMSelectionCommand{
		WorkspaceID:      workspaceID,
		Selection:        cloned,
		ExpectedRevision: expectedRevision,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceLLMSelectionResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdateLLMSelection)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspaceeventmapping.Workspace(response.Workspace), nil
}

func DeleteWorkspaceRecord(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, command appmodelcore.DeleteWorkspaceCommand) (workspaceevents.DeleteWorkspaceResult, error) {
	if hub == nil || workspaceID == "" {
		return workspaceevents.DeleteWorkspaceResult{}, common.ErrInvalidArgument
	}
	command = command.Normalized(string(workspaceID))
	ownerCommand := workspaceevents.DeleteWorkspaceCommand{WorkspaceID: workspaceID, RequestID: command.RequestID, Purge: command.Purge, ExpectedRevision: command.ExpectedRevision}.Normalized()
	if err := ownerCommand.Validate(); err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDelete, workspaceDeletionIngressSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, ownerCommand)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDelete, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.DeleteWorkspaceResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandDelete)
	if err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	if err := response.ValidateFor(ownerCommand); err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	return response, nil
}
