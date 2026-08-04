package repository

import (
	"context"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ToolCallRepository interface {
	Create(context.Context, *runtimeevidencemodel.ToolCall) error
	Get(context.Context, common.ToolCallID) (*runtimeevidencemodel.ToolCall, error)
	ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.ToolCall, error)
	ListByStep(context.Context, common.StepID) ([]*runtimeevidencemodel.ToolCall, error)
	Update(context.Context, *runtimeevidencemodel.ToolCall) error
	Delete(context.Context, common.ToolCallID) error
}
