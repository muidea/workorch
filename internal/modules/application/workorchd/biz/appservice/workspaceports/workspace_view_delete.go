package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeDeletedWorkspaceView(ctx context.Context, hub events.Hub, req DeleteWorkspaceViewRequest, id common.WorkspaceID, workspace *workspacemodel.Workspace) error {
	if err := PurgeWorkspaceEventStream(ctx, hub, id); err != nil {
		return err
	}
	return workspacecore.PurgeDirectories(req.RepositoryBackend, req.RepositoryDataDir, string(id), workspace.DeliverableDirectory, workspace.InternalDirectory)
}
