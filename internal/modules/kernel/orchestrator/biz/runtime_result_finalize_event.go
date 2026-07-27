package orchestrator

import (
	"context"

	runModel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) runtimeEventRun(ctx context.Context, runID common.RunID) (*runModel.Run, error) {
	if o == nil || o.runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		return o.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	}
	return o.runs.Get(ctx, runID)
}
