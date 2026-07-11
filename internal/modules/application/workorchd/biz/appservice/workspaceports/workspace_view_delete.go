package workspaceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeDeletedWorkspaceView(ctx context.Context, hub magicevent.Hub, req DeleteWorkspaceViewRequest, id common.WorkspaceID, workspace *workspacemodel.Workspace) error {
	if err := PurgeWorkspaceEventStream(ctx, hub, id); err != nil {
		return err
	}
	return workspacecore.PurgeDirectories(req.RepositoryBackend, req.RepositoryDataDir, string(id), workspace.DeliverableDirectory, workspace.InternalDirectory)
}
