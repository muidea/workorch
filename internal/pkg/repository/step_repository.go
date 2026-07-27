package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type StepRepository interface {
	Create(context.Context, *run.Step) error
	Get(context.Context, common.StepID) (*run.Step, error)
	ListByRun(context.Context, common.RunID) ([]*run.Step, error)
	ListByRunSummary(context.Context, common.RunID) ([]*run.Step, error)
	Update(context.Context, *run.Step) error
	Delete(context.Context, common.StepID) error
}
