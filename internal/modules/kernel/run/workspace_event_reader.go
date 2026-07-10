package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type workspaceEventReader struct {
	hub magicevent.Hub
}

func (r workspaceEventReader) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: id})
	ev.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.TopicWorkspaceCommandGet, events.EventHubLaneScope{
		SubjectType: "workspace",
		SubjectID:   string(id),
		WorkspaceID: string(id),
	}))
	result := r.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(workspaceevents.WorkspaceCommandResult)
	if !ok || response.Workspace == nil {
		return nil, common.ErrNotFound
	}
	return response.Workspace, nil
}
