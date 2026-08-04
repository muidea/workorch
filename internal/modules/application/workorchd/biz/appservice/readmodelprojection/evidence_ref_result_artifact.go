package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func addRunResultArtifactEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Results {
		refs.add(item.ID)
		for _, ref := range item.EvidenceRefs {
			refs.add(ref)
		}
	}
	for _, item := range snapshot.Artifacts {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.ResultID)
		refs.add(item.ToolCallID)
		refs.add(item.PatchID)
		refs.add(item.ValidationResultID)
		for _, ref := range item.EvidenceRefs {
			refs.add(ref)
		}
	}
}
