package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func applyWorkspaceRunOutcome(ret *projectioncontract.WorkspaceOutcome, outcome projectioncontract.RunOutcome) {
	if ret == nil {
		return
	}
	ret.State = outcome.State
	ret.RunID = outcome.RunID
	ret.ResultID = outcome.ResultID
	ret.Accepted = outcome.Accepted
	ret.Terminal = outcome.Terminal
	ret.Summary = outcome.Summary
	ret.Reason = outcome.Reason
	ret.EvidenceRefs = outcome.EvidenceRefs
	ret.ArtifactRefs = outcome.ArtifactRefs
}
