package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeCheckpointRepository interface {
	Create(context.Context, *run.RuntimeCheckpoint) error
	Get(context.Context, common.CheckpointID) (*run.RuntimeCheckpoint, error)
	ListByRun(context.Context, common.RunID) ([]*run.RuntimeCheckpoint, error)
	Update(context.Context, *run.RuntimeCheckpoint) error
}
