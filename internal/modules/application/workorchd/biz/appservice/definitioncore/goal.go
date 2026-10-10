package definitioncore

import (
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func LatestWorkspaceGoal(items []*workspacemodel.Goal) *workspacemodel.Goal {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && strings.TrimSpace(items[i].Text) != "" {
			return items[i]
		}
	}
	return nil
}

func MergedGoalText(existing []*workspacemodel.Goal, appended string) string {
	seen := map[string]struct{}{}
	parts := make([]string, 0, len(existing)+1)
	for _, item := range existing {
		if item == nil {
			continue
		}
		text := FormatGoalDefinitionText(item.Text)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	for _, entry := range NormalizeGoalTextAdditionEntries(appended) {
		if _, ok := seen[entry]; !ok {
			seen[entry] = struct{}{}
			parts = append(parts, entry)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
