package readmodelcore

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func GetRunFromReader(ctx context.Context, runs RunReader, runID common.RunID) (*runmodel.Run, error) {
	if runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		return runs.GetByWorkspaceID(ctx, workspaceID, runID)
	}
	return runs.Get(ctx, runID)
}
