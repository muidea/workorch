package readmodelprojection

import (
	"strings"
	"unicode"
)

func readModelDoDItemIsLeaf(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	content := trimReadModelDoDItemText(trimmed)
	return !(content == "" || strings.HasSuffix(content, ":") || strings.HasSuffix(content, "："))
}

func normalizeReadModelDoDText(value string) string {
	value = trimReadModelDoDItemText(value)
	value = strings.NewReplacer("**", "", "__", "", "`", "", "（", "(", "）", ")").Replace(value)
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
