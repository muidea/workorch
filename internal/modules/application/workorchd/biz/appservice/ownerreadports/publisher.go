package ownerreadports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

const sourceModule = "application/workorchd/appservice"

func workspaceScopeFromContext(ctx context.Context) common.WorkspaceID {
	return common.RuntimeWorkspaceIDFromContext(ctx)
}
