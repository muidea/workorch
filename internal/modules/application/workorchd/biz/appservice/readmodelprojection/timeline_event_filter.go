package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func TimelineEvents(items []projectioncontract.RuntimeTimelineItem) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" || item.Source == "persisted_event" {
			ret = append(ret, item)
		}
	}
	return ret
}
