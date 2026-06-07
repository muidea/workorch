package governancecore

import "strings"

func PreferredContinuationCandidate(value any) string {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if IsContinuationCandidate(item) {
				return strings.TrimSpace(item)
			}
		}
	case []any:
		for _, item := range typed {
			if candidate := PreferredContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case map[string]any:
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if candidate := PreferredContinuationCandidate(typed[key]); candidate != "" {
				return candidate
			}
		}
	case string:
		if IsContinuationCandidate(typed) {
			return strings.TrimSpace(typed)
		}
	}
	return ""
}

func IsContinuationCandidate(value string) bool {
	normalized := NormalizeRepairResolutionText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newscratch", "newproject",
		"currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, NormalizeRepairResolutionText(marker)) {
			return true
		}
	}
	return false
}
