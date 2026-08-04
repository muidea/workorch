package repository

import (
	"context"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type PatchRepository interface {
	Create(context.Context, *runtimeevidencemodel.Patch) error
	Get(context.Context, common.PatchID) (*runtimeevidencemodel.Patch, error)
	ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.Patch, error)
	ListByStep(context.Context, common.StepID) ([]*runtimeevidencemodel.Patch, error)
	Update(context.Context, *runtimeevidencemodel.Patch) error
	Delete(context.Context, common.PatchID) error
}
