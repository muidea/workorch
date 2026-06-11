package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func EnsureWorkspaceExists(ctx context.Context, hub events.Hub, workspaceID string) error {
	id := common.WorkspaceID(workspaceID)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, id)
	_, err := ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, id)
	return err
}
