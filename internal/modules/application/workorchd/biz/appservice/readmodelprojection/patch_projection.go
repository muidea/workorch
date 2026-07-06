package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func ProjectPatch(item *runmodel.Patch) readmodelcore.Patch {
	if item == nil {
		return readmodelcore.Patch{}
	}
	addedLines, deletedLines := patchLineStats(item)
	return readmodelcore.Patch{
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
		Diagnostics:      cloneReadModelMapSlice(item.Diagnostics),
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}
