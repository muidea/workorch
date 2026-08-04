package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func LastContextEvidence(items []projectioncontract.ContextEvidence, limit int) []projectioncontract.ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]projectioncontract.ContextEvidence(nil), items[start:]...)
}

func ContextEvidenceRefs(items []projectioncontract.ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}
