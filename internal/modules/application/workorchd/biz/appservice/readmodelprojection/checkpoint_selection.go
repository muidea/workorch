package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func latestRuntimeCheckpoint(items []readmodelcore.RuntimeCheckpoint) *readmodelcore.RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return &latest
}

func LatestRuntimeCheckpoint(items []readmodelcore.RuntimeCheckpoint) *readmodelcore.RuntimeCheckpoint {
	return latestRuntimeCheckpoint(items)
}
