package definitioncore

import "strings"

func formatStructuredGoalMarkdown(lines []string) string {
	ret := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(trimmedRight)
		if trimmed == "" {
			if len(ret) > 0 {
				blank = true
			}
			continue
		}
		if blank {
			ret = append(ret, "")
			blank = false
		}
		ret = append(ret, trimmedRight)
	}
	return strings.TrimSpace(strings.Join(ret, "\n"))
}
