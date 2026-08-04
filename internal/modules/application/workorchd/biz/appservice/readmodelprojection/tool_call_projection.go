package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func ProjectToolCall(item *runtimeevidencemodel.ToolCall) projectioncontract.ToolCall {
	if item == nil {
		return projectioncontract.ToolCall{}
	}
	return projectToolCallBase(item)
}

func ProjectToolCallView(item projectioncontract.ToolCall) projectioncontract.ToolCall {
	return projectioncontract.ToolCall{
		ID:          item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		ToolName:    item.ToolName,
		State:       item.State,
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}
