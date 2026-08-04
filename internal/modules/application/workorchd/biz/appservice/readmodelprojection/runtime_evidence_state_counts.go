package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func accumulateRuntimeEvidenceStates(ret *projectioncontract.RuntimeEvidenceSummary, snapshot readmodelcore.RunSnapshotState) {
	accumulateToolCallEvidenceStates(ret, snapshot.ToolCalls)
	accumulatePatchEvidenceStates(ret, snapshot.Patches)
	accumulateValidationEvidenceStates(ret, snapshot.ValidationResults)
}
