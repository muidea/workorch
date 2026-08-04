package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRunOutcomeTerminalResult(ret *projectioncontract.RunOutcome, result projectioncontract.Result) {
	if result.State == string(common.ResultStateAccepted) {
		ret.State = "accepted"
		ret.Accepted = true
	} else {
		ret.State = "failed"
		ret.Reason = result.Summary
	}
	ret.Terminal = true
	ret.ResultID = result.ID
	ret.Summary = result.Summary
	ret.EvidenceRefs = compactUniqueStrings(result.EvidenceRefs)
	ret.ArtifactRefs = compactUniqueStrings(result.ArtifactRefs)
}
