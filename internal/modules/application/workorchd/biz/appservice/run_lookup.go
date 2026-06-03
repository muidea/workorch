package appservice

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func getRunFromReadModelReader(ctx context.Context, runs readModelRunReader, runID common.RunID) (*runmodel.Run, error) {
	if runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
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
	return runs.Get(ctx, runID)
}
