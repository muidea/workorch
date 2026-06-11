package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

func contextWithGovernanceScope(ctx context.Context, scope common.WorkspaceID) context.Context {
	if scope == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, scope)
}
