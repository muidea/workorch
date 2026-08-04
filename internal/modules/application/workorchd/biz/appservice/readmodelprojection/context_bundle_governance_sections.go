package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextGovernanceSection(snapshot readmodelcore.RunSnapshotState, title string) projectioncontract.ContextSection {
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return projectioncontract.ContextSection{}
	}
	return projectioncontract.ContextSection{
		ID:           "governance",
		Kind:         "governance",
		Title:        title,
		Priority:     80,
		State:        GovernanceProgressState(snapshot),
		Summary:      "decision tickets and approvals that affected run progress",
		EvidenceRefs: GovernanceEvidenceRefs(snapshot),
		Data: map[string]any{
			"decisions": snapshot.Decisions,
			"approvals": snapshot.Approvals,
		},
	}
}
