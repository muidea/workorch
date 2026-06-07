package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func LastContextEvidence(items []readmodelcore.ContextEvidence, limit int) []readmodelcore.ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]readmodelcore.ContextEvidence(nil), items[start:]...)
}

func ContextEvidenceRefs(items []readmodelcore.ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}
