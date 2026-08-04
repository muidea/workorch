package readmodelprojection

import (
	"strings"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func patchLineStats(item *runtimeevidencemodel.Patch) (int, int) {
	if item == nil {
		return 0, 0
	}
	if item.AddedLines != 0 || item.DeletedLines != 0 {
		return item.AddedLines, item.DeletedLines
	}
	text := item.Diff
	if strings.TrimSpace(text) == "" {
		text = item.Content
	}
	added := 0
	deleted := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}
