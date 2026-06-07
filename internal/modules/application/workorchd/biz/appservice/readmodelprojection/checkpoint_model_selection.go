package readmodelprojection

import runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"

func latestRuntimeCheckpointModel(items []*runmodel.RuntimeCheckpoint) *runmodel.RuntimeCheckpoint {
	var latest *runmodel.RuntimeCheckpoint
	for _, item := range items {
		if item == nil {
			continue
		}
		if latest == nil ||
			item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}
