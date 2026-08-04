package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MemoryRecallTraces(ctx context.Context, reader readmodelcore.MemoryRecallTraceReader, query readmodelcore.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
	items, err := reader.List(ctx, readmodelcore.MemoryRecallTraceOwnerQuery(query))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.MapMemoryRecallTraces(items), nil
}

func EventRecords(ctx context.Context, reader readmodelcore.EventRecordReader, query readmodelcore.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
	items, err := reader.List(ctx, readmodelcore.EventRecordOwnerQuery(query))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.MapEventRecords(items), nil
}
