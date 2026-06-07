package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func decisionProgressItem(item readmodelcore.Decision) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "decision",
		Title:        firstNonEmptyTimelineString(item.Question, item.DecisionKind, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}

func approvalProgressItem(item readmodelcore.Approval) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "approval",
		Title:        firstNonEmptyTimelineString(item.Reason, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}
