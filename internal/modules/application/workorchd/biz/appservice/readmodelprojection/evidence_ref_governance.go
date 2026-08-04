package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func addRunGovernanceEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Decisions {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.Approvals {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
}
