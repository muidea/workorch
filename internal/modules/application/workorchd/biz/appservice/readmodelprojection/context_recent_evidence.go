package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextRecentEvidence(timeline []readmodelcore.RuntimeTimelineItem, limit int) []readmodelcore.ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]readmodelcore.ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, contextEvidenceFromTimelineItem(item))
	}
	return ret
}

func contextEvidenceFromTimelineItem(item readmodelcore.RuntimeTimelineItem) readmodelcore.ContextEvidence {
	return readmodelcore.ContextEvidence{
		ID:         firstNonEmptyTimelineString(item.ID, item.Topic, item.StepID),
		Kind:       firstNonEmptyTimelineString(item.Source+"/"+item.Type, item.Source),
		State:      firstNonEmptyTimelineString(item.StepState, item.RunState),
		Summary:    item.Summary,
		OccurredAt: item.OccurredAt,
		EvidenceRefs: compactUniqueStrings([]string{
			item.ID,
			item.StepID,
			item.Topic,
		}),
		Data: compactContextEvidenceData(item),
	}
}
