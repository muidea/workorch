package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func runRequiredDoDLeafItems(snapshot readmodelcore.RunSnapshot) []string {
	dod := readModelMapValue(snapshot.Run.ExecutionContext["dod"])
	items := readModelStringSliceValue(dod["items"])
	if len(items) == 0 {
		for _, item := range readModelMapSliceValue(snapshot.Run.ExecutionContext["dods"]) {
			items = append(items, readModelStringSliceValue(item["items"])...)
		}
	}
	if len(items) == 0 {
		return nil
	}
	ret := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		raw = strings.TrimSpace(raw)
		if raw == "" || !readModelDoDItemIsLeaf(raw) {
			continue
		}
		normalized := normalizeReadModelDoDText(raw)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		ret = append(ret, raw)
	}
	return ret
}
