package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ProjectPatch(item *runtimeevidencemodel.Patch) projectioncontract.Patch {
	if item == nil {
		return projectioncontract.Patch{}
	}
	addedLines, deletedLines := patchLineStats(item)
	return projectioncontract.Patch{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.RunID),
		StepID:           string(item.StepID),
		ToolCallID:       string(item.ToolCallID),
		ApprovalID:       string(item.ApprovalID),
		RevertApprovalID: string(item.RevertApprovalID),
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            string(item.State),
		AddedLines:       addedLines,
		DeletedLines:     deletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		AffectedFiles:    cloneReadModelStringSlice(item.AffectedFiles),
		RiskLevel:        item.RiskLevel,
		RiskReasons:      cloneReadModelStringSlice(item.RiskReasons),
		Diagnostics:      cloneReadModelMapSlice(workorcheventpayload.MapSlice(item.Diagnostics)),
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}
