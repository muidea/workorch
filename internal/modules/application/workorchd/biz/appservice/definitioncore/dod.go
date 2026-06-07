package definitioncore

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/doditems"
)

func LatestWorkspaceDoD(items []*workspacemodel.DoD) *workspacemodel.DoD {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && len(NormalizeDoDItems(items[i].Items)) > 0 {
			return items[i]
		}
	}
	return nil
}

func NormalizeDoDItems(items []string) []string {
	return doditems.Normalize(items)
}

func MergedDoDItems(existing []*workspacemodel.DoD, appended []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(appended))
	for _, item := range existing {
		if item == nil {
			continue
		}
		for _, entry := range NormalizeDoDItems(item.Items) {
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			ret = append(ret, entry)
		}
	}
	for _, entry := range NormalizeDoDItems(appended) {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		ret = append(ret, entry)
	}
	return ret
}
