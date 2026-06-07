package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (RunSnapshot, error) {
		return readRunSnapshot(ctx, deps, common.RunID(runID))
	})
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (RunSnapshot, error) {
		return ReadRunSnapshotWithSession(ctx, session, runID)
	})
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (RunSnapshot, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (RunSnapshot, error) {
		return ReadRunSnapshotWithSession(ctx, session, runID)
	})
}

func ReadRunStatusWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunStatus, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (RunStatus, error) {
		return readRunStatus(ctx, deps, common.RunID(runID))
	})
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunStatus, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (RunStatus, error) {
		return ReadRunStatusWithSession(ctx, session, runID)
	})
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (RunStatus, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (RunStatus, error) {
		return ReadRunStatusWithSession(ctx, session, runID)
	})
}

func RunStatusFromSnapshot(snapshot RunSnapshot) RunStatus {
	return readmodelports.RunStatusFromSnapshot(snapshot)
}

func RunObservabilityFromSnapshot(snapshot RunSnapshot) RunObservability {
	return readmodelports.RunObservabilityFromSnapshot(snapshot)
}
