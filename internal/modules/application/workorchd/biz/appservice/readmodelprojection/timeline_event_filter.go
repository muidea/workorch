package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TimelineEvents(items []readmodelcore.RuntimeTimelineItem) []readmodelcore.RuntimeTimelineItem {
	ret := make([]readmodelcore.RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" || item.Source == "persisted_event" {
			ret = append(ret, item)
		}
	}
	return ret
}
