package dialoglanguage

import (
	"os"
	"strings"
)

func FromEnvironment() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang := NormalizeTag(os.Getenv(env)); lang != "" {
			return lang
		}
	}
	return ""
}

func NormalizeTag(value string) string {
	normalized := normalizeLanguageValue(value)
	if normalized == "" {
		return ""
	}
	switch {
	case normalized == "c" || normalized == "posix":
		return "en"
	case strings.HasPrefix(normalized, "zh") ||
		strings.Contains(normalized, "chinese") ||
		strings.Contains(normalized, "中文") ||
		strings.Contains(normalized, "汉语"):
		return "zh-CN"
	case strings.HasPrefix(normalized, "en") || strings.Contains(normalized, "english"):
		return "en"
	default:
		return "en"
	}
}

func DisplayName(value string) string {
	tag := NormalizeTag(value)
	switch tag {
	case "zh-CN":
		return "Chinese (Simplified)"
	case "en":
		return "English"
	default:
		return "English"
	}
}

func PrefersChinese(value string) bool {
	return NormalizeTag(value) == "zh-CN"
}

func IsSupported(value string) bool {
	normalized := normalizeLanguageValue(value)
	return normalized == "" ||
		normalized == "c" ||
		normalized == "posix" ||
		strings.HasPrefix(normalized, "zh") ||
		strings.Contains(normalized, "chinese") ||
		strings.Contains(normalized, "中文") ||
		strings.Contains(normalized, "汉语") ||
		strings.HasPrefix(normalized, "en") ||
		strings.Contains(normalized, "english")
}

func normalizeLanguageValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Split(value, ":")[0]
	value = strings.Split(value, ".")[0]
	return strings.ToLower(strings.ReplaceAll(value, "_", "-"))
}
