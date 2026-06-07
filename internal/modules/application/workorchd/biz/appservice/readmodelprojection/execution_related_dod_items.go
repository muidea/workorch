package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func relatedDoDItemsForSection(node readmodelcore.TaskDecompositionNode, dodItems []string) []string {
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
