package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CoverageWorkItemRepository interface {
	Create(context.Context, *orchestratormodel.CoverageWorkItem) error
	Get(context.Context, common.CoverageWorkItemID) (*orchestratormodel.CoverageWorkItem, error)
	ListByRun(context.Context, common.RunID) ([]*orchestratormodel.CoverageWorkItem, error)
	ListActiveByRun(context.Context, common.RunID) ([]*orchestratormodel.CoverageWorkItem, error)
	Update(context.Context, *orchestratormodel.CoverageWorkItem) error
	Delete(context.Context, common.CoverageWorkItemID) error
}
