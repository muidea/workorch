package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func ProjectToolCall(item *runtimeevidencemodel.ToolCall) readmodelcore.ToolCall {
	if item == nil {
		return readmodelcore.ToolCall{}
	}
	return projectToolCallBase(item)
}

func ProjectToolCallView(item readmodelcore.ToolCall) readmodelcore.ToolCall {
	return readmodelcore.ToolCall{
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
