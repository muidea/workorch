package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func TaskProgressBlockers(snapshot readmodelcore.RunSnapshot) []readmodelcore.ProgressBlocker {
	var ret []readmodelcore.ProgressBlocker
	for _, item := range snapshot.Decisions {
		if IsResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, readmodelcore.ProgressBlocker{
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
		ret = append(ret, readmodelcore.ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}
