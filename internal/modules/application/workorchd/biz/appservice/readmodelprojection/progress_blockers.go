package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TaskProgressBlockers(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ProgressBlocker {
	var ret []projectioncontract.ProgressBlocker
	for _, item := range snapshot.Decisions {
		if IsResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyTimelineString(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
	}
	for _, item := range snapshot.Approvals {
		if IsResolvedApprovalState(item.State) {
			continue
		}
		ret = append(ret, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}
