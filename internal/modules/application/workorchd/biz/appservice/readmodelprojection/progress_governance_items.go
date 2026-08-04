package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func decisionProgressItem(item projectioncontract.Decision) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "decision",
		Title:        firstNonEmptyTimelineString(item.Question, item.DecisionKind, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}

func approvalProgressItem(item projectioncontract.Approval) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "approval",
		Title:        firstNonEmptyTimelineString(item.Reason, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}
