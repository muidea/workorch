package repository

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MemoryRecallTraceQuery struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Stage       string
	Source      string
	Status      string
	Limit       int
}

type MemoryRecallTraceRepository interface {
	Create(context.Context, *memorymodel.MemoryRecallTrace) error
	Upsert(context.Context, *memorymodel.MemoryRecallTrace) error
	Get(context.Context, common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error)
	List(context.Context, MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
	Delete(context.Context, common.MemoryRecallTraceID) error
}
