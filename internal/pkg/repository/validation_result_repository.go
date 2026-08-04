package repository

import (
	"context"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ValidationResultRepository interface {
	Create(context.Context, *runtimeevidencemodel.ValidationResult) error
	Get(context.Context, common.ValidationResultID) (*runtimeevidencemodel.ValidationResult, error)
	ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.ValidationResult, error)
	ListByStep(context.Context, common.StepID) ([]*runtimeevidencemodel.ValidationResult, error)
	Update(context.Context, *runtimeevidencemodel.ValidationResult) error
	Delete(context.Context, common.ValidationResultID) error
}
