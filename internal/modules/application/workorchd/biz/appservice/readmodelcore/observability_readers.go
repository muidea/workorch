package readmodelcore

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type EventRecordReader interface {
	List(context.Context, workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type MemoryRecallTraceReader interface {
	List(context.Context, workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
}
