package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func contextExecutionSectionStateAndEvidenceRefs(items []readmodelcore.ExecutionSection) (string, []string) {
	refs := make([]string, 0)
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.ActualEvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return state, refs
}
