package activationcore

import (
	"crypto/sha256"
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func MemoryRetrievalQueryText(goal *workspacemodel.Goal, dod *workspacemodel.DoD) string {
	parts := []string{}
	if goal != nil && strings.TrimSpace(goal.Text) != "" {
		parts = append(parts, "Goal: "+strings.TrimSpace(goal.Text))
	}
	if dod != nil && len(dod.Items) > 0 {
		parts = append(parts, "Definition of Done:")
		for _, item := range dod.Items {
			item = strings.TrimSpace(item)
			if item != "" {
				parts = append(parts, "- "+item)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func PreviewRetrievalQuery(text string) string {
	text = strings.TrimSpace(text)
	const limit = 240
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func SHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}
