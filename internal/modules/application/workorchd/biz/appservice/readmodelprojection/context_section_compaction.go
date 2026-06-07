package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func CompactContextSections(items []readmodelcore.ContextSection) []readmodelcore.ContextSection {
	ret := make([]readmodelcore.ContextSection, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Priority == ret[j].Priority {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Priority > ret[j].Priority
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}
