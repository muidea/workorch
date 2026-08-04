package repository

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type MemoryRecallTraceRepository interface {
	Create(context.Context, *memorymodel.MemoryRecallTrace) error
	Upsert(context.Context, *memorymodel.MemoryRecallTrace) error
	Get(context.Context, common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error)
	List(context.Context, workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
	Delete(context.Context, common.MemoryRecallTraceID) error
}
