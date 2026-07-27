package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func projectToolCallBase(item *runmodel.ToolCall) readmodelcore.ToolCall {
	return readmodelcore.ToolCall{
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
