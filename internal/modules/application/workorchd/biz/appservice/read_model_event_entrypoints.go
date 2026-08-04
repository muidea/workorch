package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func ListMemoryRecallTracesWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.MemoryRecallTraceQuery) (ret []projectioncontract.MemoryRecallTrace, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]projectioncontract.MemoryRecallTrace, error) {
		return readmodelports.MemoryRecallTraces(ctx, deps.memoryRecalls, query)
	})
}

func ListMemoryRecallTracesWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]projectioncontract.MemoryRecallTrace, error) {
		return ListMemoryRecallTracesWithSession(ctx, session, query)
	})
}

func ListEventRecordsWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.EventRecordQuery) (ret []projectioncontract.EventRecord, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]projectioncontract.EventRecord, error) {
		return readmodelports.EventRecords(ctx, deps.eventRecords, query)
	})
}

func ListEventRecordsWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]projectioncontract.EventRecord, error) {
		return ListEventRecordsWithSession(ctx, session, query)
	})
}

func ListEventRecordsFromEnv(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]projectioncontract.EventRecord, error) {
		return ListEventRecordsWithSession(ctx, session, query)
	})
}
