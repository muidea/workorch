package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func ContextMemoryRecallSection(items []readmodelcore.MemoryRecallTrace) readmodelcore.ContextSection {
	if len(items) == 0 {
		return readmodelcore.ContextSection{}
	}
	refs := make([]string, 0)
	memoryIDs := make([]string, 0)
	status := "selected"
	for _, item := range items {
		refs = append(refs, item.ID, item.EvidenceRef)
		memoryIDs = append(memoryIDs, item.RetrievedMemoryIDs...)
		if item.Status == "degraded" {
			status = "degraded"
		}
	}
	return readmodelcore.ContextSection{
		ID:           "memory_recall",
		Kind:         "memory_recall",
		Title:        "Memory recall",
		Priority:     83,
		State:        status,
		Summary:      strconv.Itoa(len(items)) + " memory recall traces",
		EvidenceRefs: compactUniqueStrings(append(refs, memoryIDs...)),
		Data: map[string]any{
			"memoryRecallTraces": items,
			"memoryIds":          compactUniqueStrings(memoryIDs),
		},
	}
}
