package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapRuntimeEvidenceSummary(snapshot readmodelcore.RunSnapshotState, eventTotal int) projectioncontract.RuntimeEvidenceSummary {
	ret := projectioncontract.RuntimeEvidenceSummary{
		ToolCallTotal:          len(snapshot.ToolCalls),
		PatchTotal:             len(snapshot.Patches),
		ValidationTotal:        len(snapshot.ValidationResults),
		RuntimeCheckpointTotal: len(snapshot.RuntimeCheckpoints),
		ResultTotal:            len(snapshot.Results),
		ArtifactTotal:          len(snapshot.Artifacts),
		EventTotal:             eventTotal,
	}
	accumulateRuntimeEvidenceStates(&ret, snapshot)
	return ret
}
