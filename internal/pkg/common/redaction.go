package common

import (
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var secretValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsk-[a-z0-9][a-z0-9_\-]{6,}\b`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|passwd|secret|private[_-]?key)\s*[:=]\s*([^\s,;]+)`),
	regexp.MustCompile(`(?i)\b(bearer)\s+[a-z0-9._\-]{8,}\b`),
}

// RedactSensitiveMap returns a deep-copy projection safe for persistence,
// events, read models, and LLM context. It is intentionally domain-neutral:
// detection is based on credential-shaped keys and common secret value forms.
func RedactSensitiveMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		if isSensitiveKey(key) {
			ret[key] = redactedValue
			continue
		}
		ret[key] = RedactSensitiveValue(value)
	}
	return ret
}

func RedactSensitiveValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return RedactSensitiveString(typed)
	case map[string]any:
		return RedactSensitiveMap(typed)
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, RedactSensitiveMap(item))
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, RedactSensitiveValue(item))
		}
		return ret
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, RedactSensitiveString(item))
		}
		return ret
	default:
		return typed
	}
}

func RedactSensitiveString(value string) string {
	ret := value
	for _, pattern := range secretValuePatterns {
		ret = pattern.ReplaceAllStringFunc(ret, func(match string) string {
			lower := strings.ToLower(match)
			if strings.HasPrefix(lower, "bearer ") {
				return "Bearer " + redactedValue
			}
			if idx := strings.IndexAny(match, ":="); idx >= 0 {
				return strings.TrimSpace(match[:idx+1]) + " " + redactedValue
			}
			return redactedValue
		})
	}
	return ret
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(normalized)
	switch normalized {
	case "apikey", "accesstoken", "refreshtoken", "token", "password", "passwd",
		"secret", "clientsecret", "privatekey", "credential", "credentials", "authorization":
		return true
	default:
		return strings.Contains(normalized, "apikey") ||
			strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "token") ||
			strings.Contains(normalized, "privatekey")
	}
}
