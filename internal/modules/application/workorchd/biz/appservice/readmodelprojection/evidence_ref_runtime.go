package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func addRunRuntimeEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Steps {
		refs.add(item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.Patches {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.ValidationResults {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.EvidenceRef)
	}
	for _, item := range snapshot.RuntimeCheckpoints {
		refs.add(item.ID)
		refs.add(item.CurrentStepID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.EvidenceRef)
	}
}
