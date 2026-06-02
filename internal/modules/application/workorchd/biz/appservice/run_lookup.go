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
	items, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && item.ID == runID {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}
