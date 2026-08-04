package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ProjectValidationResultView(item projectioncontract.ValidationResult) projectioncontract.ValidationResult {
	return projectioncontract.ValidationResult{
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
