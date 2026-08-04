package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func patchProgressItem(item projectioncontract.Patch) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "patch",
		Title:        firstNonEmptyTimelineString(item.TargetRef, item.PatchType, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID},
	}
}

func validationProgressItem(item projectioncontract.ValidationResult, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "validation_result",
		Title:        firstNonEmptyTimelineString(item.ValidationKind, item.TargetRef, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID},
	}
}
