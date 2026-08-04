package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func runEvidenceRefIndex(snapshot readmodelcore.RunSnapshotState) map[string]struct{} {
	ret := runEvidenceRefs{}
	ret.add(snapshot.Run.ID)
	addRunRuntimeEvidenceRefs(ret, snapshot)
	addRunResultArtifactEvidenceRefs(ret, snapshot)
	addRunGovernanceEvidenceRefs(ret, snapshot)
	return ret
}
