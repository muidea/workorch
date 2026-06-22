package readmodelprojection

const (
	maxRawStreamTimelineItems  = 32
	streamTimelineHeadItemKeep = 12
	streamTimelineTailItemKeep = 12
	streamTimelineDeltaRunes   = 512
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

func boundedStreamTimelineEvent(event map[string]any) map[string]any {
	ret := cloneReadModelMap(event)
	if len(ret) == 0 {
		return nil
	}
	if delta := readModelStringValue(ret["delta"]); delta != "" {
		preview, truncated, runeCount := streamTimelineDeltaPreview(delta)
		ret["delta"] = preview
		if truncated {
			ret["deltaTruncated"] = true
			ret["deltaRuneCount"] = runeCount
		}
	}
	if payload := readModelMapValue(ret["payload"]); len(payload) > 0 {
		if delta := readModelStringValue(payload["delta"]); delta != "" {
			preview, truncated, runeCount := streamTimelineDeltaPreview(delta)
			payload["delta"] = preview
			if truncated {
				payload["deltaTruncated"] = true
				payload["deltaRuneCount"] = runeCount
			}
		}
		ret["payload"] = payload
	}
	return ret
}

func boundedStreamStepOutput(output map[string]any) map[string]any {
	ret := cloneReadModelMap(output)
	if len(ret) == 0 {
		return nil
	}
	if events := readModelMapSliceValue(ret["streamEvents"]); len(events) > 0 {
		delete(ret, "streamEvents")
		ret["streamEventsOmitted"] = true
		ret["streamEventCount"] = len(events)
	}
	return ret
}

func streamTimelineDeltaPreview(value string) (string, bool, int) {
	runes := []rune(value)
	if len(runes) <= streamTimelineDeltaRunes {
		return value, false, len(runes)
	}
	return string(runes[:streamTimelineDeltaRunes]) + "...[truncated]", true, len(runes)
}
