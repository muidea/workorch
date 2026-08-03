package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type workspaceEventReader struct {
	send func(magicevent.Event) magicevent.Result
}

func (r workspaceEventReader) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.send == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, modulecommon.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: id})
	ev.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGet, events.EventHubLaneScope{
		SubjectType: "workspace",
		SubjectID:   string(id),
		WorkspaceID: string(id),
	}))
	result := r.send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(workspaceevents.WorkspaceCommandResult)
	if !ok || response.Workspace == nil {
		return nil, common.ErrNotFound
	}
	workspace := response.Workspace
	ret := &workspacemodel.Workspace{
		Revision: workspace.Revision, ID: workspace.ID, Title: workspace.Title, Description: workspace.Description,
		DeliverableDirectory: workspace.DeliverableDirectory, InternalDirectory: workspace.InternalDirectory,
		State: workspace.State, ActiveRunID: workspace.ActiveRunID, TemplateID: workspace.TemplateID, ProfileID: workspace.ProfileID,
		DefaultSoulID: workspace.DefaultSoulID, SoulBindings: workspaceevents.SoulIDMapFromBindings(workspace.SoulBindings),
		ContextHints: workspace.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(workspace.EvidencePolicies),
	}
	if workspace.DefaultLLMTarget != nil {
		target := *workspace.DefaultLLMTarget
		ret.DefaultLLMTarget = &target
	}
	if workspace.TerminalDelivery != nil {
		identity := *workspace.TerminalDelivery
		ret.TerminalDelivery = &identity
	}
	return ret, nil
}
