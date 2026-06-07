package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextRecentEvidenceSection(items []readmodelcore.ContextEvidence) readmodelcore.ContextSection {
	if len(items) == 0 {
		return readmodelcore.ContextSection{}
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return readmodelcore.ContextSection{
		ID:           "recent_evidence",
		Kind:         "recent_evidence",
		Title:        "Recent evidence",
		Priority:     70,
		Summary:      "latest runtime timeline items retained for recovery",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"items": items,
		},
	}
}
