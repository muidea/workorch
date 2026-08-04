package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func ProjectValidationResult(item *runtimeevidencemodel.ValidationResult) projectioncontract.ValidationResult {
	if item == nil {
		return projectioncontract.ValidationResult{}
	}
	return projectioncontract.ValidationResult{
		ID:              string(item.ID),
		WorkspaceID:     string(item.WorkspaceID),
		RunID:           string(item.RunID),
		StepID:          string(item.StepID),
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           string(item.State),
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: string(item.EvidenceQuality),
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}
