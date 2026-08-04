package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ResultRefsFromTaskProgress(progress projectioncontract.TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}
