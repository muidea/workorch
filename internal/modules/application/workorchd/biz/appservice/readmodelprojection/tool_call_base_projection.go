package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func projectToolCallBase(item *runtimeevidencemodel.ToolCall) projectioncontract.ToolCall {
	return projectioncontract.ToolCall{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		ToolName:    item.ToolName,
		State:       string(item.State),
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}
