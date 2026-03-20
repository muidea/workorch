package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RunRepository interface {
	Create(context.Context, *run.Run) error
	Get(context.Context, common.RunID) (*run.Run, error)
	Update(context.Context, *run.Run) error
}
