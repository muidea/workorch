package appservice

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func getRunFromRepository(ctx context.Context, runs repository.RunRepository, runID common.RunID) (*runmodel.Run, error) {
	if runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return runs.Get(ctx, runID)
	}
	return runs.GetByWorkspaceID(ctx, workspaceID, runID)
}
