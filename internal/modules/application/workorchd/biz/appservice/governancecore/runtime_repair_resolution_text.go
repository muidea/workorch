package governancecore

import "strings"

func NormalizeRepairResolutionText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func ContainsAnyRepairResolutionCue(value string, cues []string) bool {
	for _, cue := range cues {
		if normalized := NormalizeRepairResolutionText(cue); normalized != "" && strings.Contains(value, normalized) {
			return true
		}
	}
	return false
}

func SemanticTextForRepairResolution(payload map[string]any) string {
	semantic := DecisionPayloadMap(payload["governanceSemantic"])
	if raw := strings.TrimSpace(StringFromDecisionPayload(semantic, "rawInput")); raw != "" {
		return strings.ToLower(strings.ReplaceAll(raw, " ", ""))
	}
	for _, key := range []string{"answer", "value", "input", "resumeInstruction"} {
		if raw := strings.TrimSpace(StringFromDecisionPayload(payload, key)); raw != "" {
			return strings.ToLower(strings.ReplaceAll(raw, " ", ""))
		}
	}
	return ""
}
