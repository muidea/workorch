package repository

import (
	"context"

	result "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ResultRepository interface {
	Create(context.Context, *result.Result) error
	Get(context.Context, common.ResultID) (*result.Result, error)
	ListByWorkspace(context.Context, common.WorkspaceID) ([]*result.Result, error)
	ListByRun(context.Context, common.RunID) ([]*result.Result, error)
	Update(context.Context, *result.Result) error
	Delete(context.Context, common.ResultID) error
}
