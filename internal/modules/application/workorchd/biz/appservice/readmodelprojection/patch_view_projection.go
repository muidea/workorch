package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectPatchView(item readmodelcore.Patch) readmodelcore.Patch {
	return readmodelcore.Patch{
		ID:               item.ID,
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ToolCallID:       item.ToolCallID,
		ApprovalID:       item.ApprovalID,
		RevertApprovalID: item.RevertApprovalID,
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            item.State,
		AddedLines:       item.AddedLines,
		DeletedLines:     item.DeletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		AffectedFiles:    cloneReadModelStringSlice(item.AffectedFiles),
		RiskLevel:        item.RiskLevel,
		RiskReasons:      cloneReadModelStringSlice(item.RiskReasons),
		Diagnostics:      cloneReadModelMapSlice(item.Diagnostics),
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}
