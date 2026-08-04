package readmodelprojection

import (
	"sort"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func CompactContextSections(items []projectioncontract.ContextSection) []projectioncontract.ContextSection {
	ret := make([]projectioncontract.ContextSection, 0, len(items))
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
