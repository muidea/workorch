package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func patchProgressItem(item readmodelcore.Patch) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "patch",
		Title:        firstNonEmptyTimelineString(item.TargetRef, item.PatchType, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID},
	}
}

func validationProgressItem(item readmodelcore.ValidationResult, recoveredHistoricalFailures bool) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "validation_result",
		Title:        firstNonEmptyTimelineString(item.ValidationKind, item.TargetRef, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID},
	}
}
