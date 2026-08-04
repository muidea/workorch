package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func persistedEventTimelineItems(snapshot readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(snapshot.EventRecords))
	for _, item := range snapshot.EventRecords {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		if streamItem, ok := streamProgressTimelineItemFromPersistedEvent(item, snapshot.Run.State); ok {
			ret = append(ret, streamItem)
			continue
		}
		ret = append(ret, persistedEventTimelineItem(item))
	}
	return ret
}

func observedEventTimelineItems(snapshot readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(snapshot.Observed))
	for _, item := range snapshot.Observed {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		if streamItem, ok := streamProgressTimelineItemFromObserved(item, snapshot.Run.State); ok {
			ret = append(ret, streamItem)
			continue
		}
		ret = append(ret, observedEventTimelineItem(item))
	}
	return ret
}
