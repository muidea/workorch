package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ContextRecentEvidence(timeline []projectioncontract.RuntimeTimelineItem, limit int) []projectioncontract.ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]projectioncontract.ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, contextEvidenceFromTimelineItem(item))
	}
	return ret
}

func contextEvidenceFromTimelineItem(item projectioncontract.RuntimeTimelineItem) projectioncontract.ContextEvidence {
	return projectioncontract.ContextEvidence{
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
