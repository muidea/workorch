package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func accumulateRuntimeEvidenceStates(ret *readmodelcore.RuntimeEvidenceSummary, snapshot readmodelcore.RunSnapshot) {
	accumulateToolCallEvidenceStates(ret, snapshot.ToolCalls)
	accumulatePatchEvidenceStates(ret, snapshot.Patches)
	accumulateValidationEvidenceStates(ret, snapshot.ValidationResults)
}
