package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func HasContinuationCheckpoint(ctx context.Context, hub events.Hub, runID common.RunID) bool {
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
