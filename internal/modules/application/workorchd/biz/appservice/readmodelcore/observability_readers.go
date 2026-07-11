package readmodelcore

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type EventRecordReader interface {
	List(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type MemoryRecallTraceReader interface {
	List(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
}
