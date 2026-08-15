package definitionquality

import (
	"strings"
	"unicode/utf8"
)

func normalizeText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ", ",", " ", "，", " ", ":", " ", "：", " ", ";", " ", "；", " ", "\n", " ", "\t", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksQuestionOnly(text string) bool {
	return strings.Contains(text, "?") || strings.Contains(text, "？")
}

func runeCount(value string) int {
	return utf8.RuneCountInString(strings.TrimSpace(value))
}

func clampScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
