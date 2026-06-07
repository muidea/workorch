package readmodelcore

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type EventRecordReader interface {
	List(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type MemoryRecallTraceReader interface {
	List(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
}
