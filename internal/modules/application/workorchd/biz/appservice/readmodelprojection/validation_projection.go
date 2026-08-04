package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func ProjectValidationResult(item *runtimeevidencemodel.ValidationResult) readmodelcore.ValidationResult {
	if item == nil {
		return readmodelcore.ValidationResult{}
	}
	return readmodelcore.ValidationResult{
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
