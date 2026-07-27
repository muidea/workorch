package readmodelcore

import (
	"context"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
)

type EventRecordReader interface {
	List(context.Context, eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type MemoryRecallTraceReader interface {
	List(context.Context, memoryevents.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
}
