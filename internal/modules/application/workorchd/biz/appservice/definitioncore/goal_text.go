package definitioncore

import "strings"

func FormatGoalDefinitionText(text string) string {
	entries := NormalizeGoalTextEntries(text)
	return strings.TrimSpace(strings.Join(entries, "\n"))
}

func NormalizeGoalTextAdditionEntries(text string) []string {
	return NormalizeGoalTextEntries(text)
}

func NormalizeGoalTextEntries(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = NormalizeGoalNewlines(text)
	lines := strings.Split(text, "\n")
	hasStructure := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "• ") {
			hasStructure = true
			break
		}
	}
	if hasStructure {
		if formatted := formatStructuredGoalMarkdown(lines); formatted != "" {
			return []string{formatted}
		}
		return nil
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ';' || r == '；'
		}) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		ret = append(ret, "- "+part)
	}
	return ret
}

func NormalizeGoalNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
