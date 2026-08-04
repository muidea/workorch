package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func GovernanceEvidenceRefs(snapshot readmodelcore.RunSnapshotState) []string {
	refs := make([]string, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Decisions {
		refs = append(refs, item.ID, item.StepID, item.ResolutionID)
	}
	for _, item := range snapshot.Approvals {
		refs = append(refs, item.ID, item.StepID)
	}
	return compactUniqueStrings(refs)
}
