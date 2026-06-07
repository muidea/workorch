package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
)

func MemoryRecallTraces(ctx context.Context, reader readmodelcore.MemoryRecallTraceReader, query readmodelcore.MemoryRecallTraceQuery) ([]readmodelcore.MemoryRecallTrace, error) {
	items, err := reader.List(ctx, readmodelcore.MemoryRecallTraceRepositoryQuery(query))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.MapMemoryRecallTraces(items), nil
}

func EventRecords(ctx context.Context, reader readmodelcore.EventRecordReader, query readmodelcore.EventRecordQuery) ([]readmodelcore.EventRecord, error) {
	items, err := reader.List(ctx, readmodelcore.EventRecordRepositoryQuery(query))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.MapEventRecords(items), nil
}
