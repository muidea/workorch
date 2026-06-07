package readmodelprojection

const (
	maxRawStreamTimelineItems  = 32
	streamTimelineHeadItemKeep = 12
	streamTimelineTailItemKeep = 12
)

func streamTimelineItemCapacity(total int) int {
	if total <= maxRawStreamTimelineItems {
		return total
	}
	return streamTimelineHeadItemKeep + streamTimelineTailItemKeep + 1
}

func streamTimelineEventOmitted(idx int, total int) bool {
	if total <= maxRawStreamTimelineItems {
		return false
	}
	return idx >= streamTimelineHeadItemKeep && idx < total-streamTimelineTailItemKeep
}

func streamEventOccurredAt(event map[string]any) string {
	for _, key := range []string{"occurredAt", "timestamp", "time"} {
		if value := readModelStringValue(event[key]); value != "" {
			return value
		}
	}
	return ""
}
