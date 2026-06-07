package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
)

func ListMemoryRecallTracesWithSession(ctx context.Context, session *RuntimeSession, query MemoryRecallTraceQuery) (ret []MemoryRecallTrace, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]MemoryRecallTrace, error) {
		return readmodelports.MemoryRecallTraces(ctx, deps.memoryRecalls, query)
	})
}

func ListMemoryRecallTracesWithBackend(ctx context.Context, backend string, dataRoot string, query MemoryRecallTraceQuery) ([]MemoryRecallTrace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MemoryRecallTrace, error) {
		return ListMemoryRecallTracesWithSession(ctx, session, query)
	})
}

func ListEventRecordsWithSession(ctx context.Context, session *RuntimeSession, query EventRecordQuery) (ret []EventRecord, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]EventRecord, error) {
		return readmodelports.EventRecords(ctx, deps.eventRecords, query)
	})
}

func ListEventRecordsWithBackend(ctx context.Context, backend string, dataRoot string, query EventRecordQuery) ([]EventRecord, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]EventRecord, error) {
		return ListEventRecordsWithSession(ctx, session, query)
	})
}

func ListEventRecordsFromEnv(ctx context.Context, query EventRecordQuery) ([]EventRecord, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]EventRecord, error) {
		return ListEventRecordsWithSession(ctx, session, query)
	})
}
