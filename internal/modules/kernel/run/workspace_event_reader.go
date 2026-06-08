package run

import (
	"context"
	"fmt"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type workspaceEventReader struct {
	hub events.Hub
}

func (r workspaceEventReader) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	err := r.hub.Publish(ctx, events.Envelope{
		Topic:        workspaceevents.TopicWorkspaceCommandGet,
		SourceModule: ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(id),
		WorkspaceID:  string(id),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: string(id),
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		item, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
		if !ok || item == nil {
			return nil, common.ErrNotFound
		}
		return item, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("workspace command %s returned no response", workspaceevents.TopicWorkspaceCommandGet)
}
