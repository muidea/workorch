package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func EnsureWorkspaceExists(ctx context.Context, hub magicevent.Hub, workspaceID string) error {
	id := common.WorkspaceID(workspaceID)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, id)
	_, err := ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, id)
	return err
}
