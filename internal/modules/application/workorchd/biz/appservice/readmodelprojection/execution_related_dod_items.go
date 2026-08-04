package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func relatedDoDItemsForSection(node projectioncontract.TaskDecompositionNode, dodItems []string) []string {
	if len(dodItems) == 0 {
		return nil
	}
	haystack := strings.ToLower(strings.Join([]string{node.ID, node.Title, node.Kind}, " "))
	ret := make([]string, 0)
	for _, item := range dodItems {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(item))) {
			ret = append(ret, item)
		}
	}
	return ret
}
