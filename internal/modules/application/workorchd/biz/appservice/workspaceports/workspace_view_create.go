package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func createWorkspaceViewAttempt(ctx context.Context, hub events.Hub, req CreateWorkspaceViewRequest) (*workspacemodel.Workspace, error) {
	id := workspacecore.GenerateWorkspaceID()
	layout, err := workspacecore.ResolveDirectoryLayout(req.RepositoryBackend, req.RepositoryDataDir, string(id), req.Command.DeliverableDirectory)
	if err != nil {
		return nil, err
	}
	if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
		return nil, err
	}
	if err := CreateWorkspaceRecord(ctx, hub, id, req.Command.Title, req.Command.Description, layout.DeliverableDirectory, layout.InternalDirectory); err != nil {
		return nil, err
	}
	item, err := ReadWorkspaceRecord(ctx, hub, id)
	if err != nil {
		return nil, err
	}
	return ApplyWorkspaceTemplateOnCreate(ctx, hub, item, req.Command.TemplateID)
}
