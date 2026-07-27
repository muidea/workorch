package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ToolCallRepository interface {
	Create(context.Context, *run.ToolCall) error
	Get(context.Context, common.ToolCallID) (*run.ToolCall, error)
	ListByRun(context.Context, common.RunID) ([]*run.ToolCall, error)
	ListByStep(context.Context, common.StepID) ([]*run.ToolCall, error)
	Update(context.Context, *run.ToolCall) error
	Delete(context.Context, common.ToolCallID) error
}
