package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func RuntimeTimeline(snapshot readmodelcore.RunSnapshot) []readmodelcore.RuntimeTimelineItem {
	items := make([]readmodelcore.RuntimeTimelineItem, 0, len(snapshot.Steps)+len(snapshot.RuntimeCheckpoints)+len(snapshot.Observed))
	stepOccurrences := TimelineStepOccurrences(snapshot)
	for _, trace := range snapshot.MemoryRecallTraces {
		items = append(items, memoryRecallTimelineItem(trace, snapshot.Run.State))
	}
	for _, step := range snapshot.Steps {
		occurredAt := stepOccurrences[step.ID]
		stepDisplayState := firstNonEmptyTimelineString(step.State, snapshot.Run.State)
		items = append(items, stepRuntimeTimelineItem(step, stepDisplayState, occurredAt))
		items = append(items, streamTimelineItems(step, stepDisplayState, occurredAt)...)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		items = append(items, checkpointRuntimeTimelineItem(checkpoint))
	}
	items = append(items, persistedEventTimelineItems(snapshot)...)
	if len(snapshot.EventRecords) == 0 {
		items = append(items, observedEventTimelineItems(snapshot)...)
	}
	sortRuntimeTimelineItems(items)
	return items
}
