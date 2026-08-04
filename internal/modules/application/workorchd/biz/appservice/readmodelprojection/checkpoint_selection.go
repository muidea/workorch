package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func latestRuntimeCheckpoint(items []projectioncontract.RuntimeCheckpoint) *projectioncontract.RuntimeCheckpoint {
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

func LatestRuntimeCheckpoint(items []projectioncontract.RuntimeCheckpoint) *projectioncontract.RuntimeCheckpoint {
	return latestRuntimeCheckpoint(items)
}
