package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func TimelineStepOccurrences(snapshot readmodelcore.RunSnapshotState) map[string]string {
	ret := map[string]string{}
	remember := func(stepID string, occurredAt string) {
		if stepID == "" || occurredAt == "" {
			return
		}
		if ret[stepID] == "" || occurredAt > ret[stepID] {
			ret[stepID] = occurredAt
		}
	}
	for _, item := range snapshot.EventRecords {
		stepID, ok := timelineStepIDFromEventRecord(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, item := range snapshot.Observed {
		stepID, ok := timelineStepIDFromObservedEvent(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		if !isStepRuntimeCheckpoint(checkpoint) {
			continue
		}
		remember(checkpoint.CurrentStepID, checkpoint.OccurredAt)
	}
	return ret
}
