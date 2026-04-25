package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ValidationResultRepository interface {
	Create(context.Context, *run.ValidationResult) error
	Get(context.Context, common.ValidationResultID) (*run.ValidationResult, error)
	ListByRun(context.Context, common.RunID) ([]*run.ValidationResult, error)
	ListByStep(context.Context, common.StepID) ([]*run.ValidationResult, error)
	Update(context.Context, *run.ValidationResult) error
}
