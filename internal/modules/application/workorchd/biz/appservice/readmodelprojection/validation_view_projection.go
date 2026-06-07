package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectValidationResultView(item readmodelcore.ValidationResult) readmodelcore.ValidationResult {
	return readmodelcore.ValidationResult{
		ID:              item.ID,
		WorkspaceID:     item.WorkspaceID,
		RunID:           item.RunID,
		StepID:          item.StepID,
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           item.State,
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: item.EvidenceQuality,
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}
