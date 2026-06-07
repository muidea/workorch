package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ResultRefsFromTaskProgress(progress readmodelcore.TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}
