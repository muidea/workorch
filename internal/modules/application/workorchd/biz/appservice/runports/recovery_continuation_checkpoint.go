package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func HasContinuationCheckpointForRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) bool {
	if item == nil {
		return false
	}
	ctx = contextWithRunWorkspace(ctx, item)
	return hasContinuationCheckpointForRunID(ctx, hub, item.ID)
}

func hasContinuationCheckpointForRunID(ctx context.Context, hub magicevent.Hub, runID common.RunID) bool {
	if hub == nil || runID == "" {
		return false
	}
	items, err := ListRuntimeCheckpointsByRun(ctx, hub, runID)
	if err != nil || len(items) == 0 {
		return false
	}
	latest := runtimecore.LatestRuntimeCheckpointModel(items)
	if latest == nil || latest.CheckpointType != common.CheckpointTypeRecovery {
		return false
	}
	return runtimecore.NonEmptyString(latest.Data["continuationAction"]) ||
		runtimecore.NonEmptyString(latest.Data["resumeAction"]) ||
		runtimecore.NonEmptyString(latest.Data["nextAction"])
}
