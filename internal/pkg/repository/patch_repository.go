package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type PatchRepository interface {
	Create(context.Context, *run.Patch) error
	Get(context.Context, common.PatchID) (*run.Patch, error)
	ListByRun(context.Context, common.RunID) ([]*run.Patch, error)
	ListByStep(context.Context, common.StepID) ([]*run.Patch, error)
	Update(context.Context, *run.Patch) error
	Delete(context.Context, common.PatchID) error
}
