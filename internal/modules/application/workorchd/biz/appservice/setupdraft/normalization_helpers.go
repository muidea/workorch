package setupdraft

import "strings"

func NormalizedConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func CompactStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func NormalizeMessages(messages []Message) []Message {
	ret := make([]Message, 0, len(messages))
	for _, item := range messages {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		switch role {
		case "assistant", "system":
		default:
			role = "user"
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		ret = append(ret, Message{Role: role, Content: content})
	}
	return ret
}
