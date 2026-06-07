package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func applyWorkspaceRunOutcome(ret *readmodelcore.WorkspaceOutcome, outcome readmodelcore.RunOutcome) {
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
