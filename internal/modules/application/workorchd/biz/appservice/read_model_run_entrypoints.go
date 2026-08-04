package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ReadRunSnapshotWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret readmodelcore.RunSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.RunSnapshot, error) {
		return readRunSnapshot(ctx, deps, common.RunID(runID))
	})
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (readmodelcore.RunSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.RunSnapshot, error) {
		return ReadRunSnapshotWithSession(ctx, session, runID)
	})
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (readmodelcore.RunSnapshot, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.RunSnapshot, error) {
		return ReadRunSnapshotWithSession(ctx, session, runID)
	})
}

func ReadRunStatusWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret projectioncontract.RunStatus, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (projectioncontract.RunStatus, error) {
		return readRunStatus(ctx, deps, common.RunID(runID))
	})
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (projectioncontract.RunStatus, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.RunStatus, error) {
		return ReadRunStatusWithSession(ctx, session, runID)
	})
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (projectioncontract.RunStatus, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.RunStatus, error) {
		return ReadRunStatusWithSession(ctx, session, runID)
	})
}

func RunStatusFromSnapshot(snapshot readmodelcore.RunSnapshot) projectioncontract.RunStatus {
	return readmodelports.RunStatusFromSnapshot(snapshot)
}

func RunObservabilityFromSnapshot(snapshot readmodelcore.RunSnapshot) readmodelcore.RunObservability {
	return readmodelports.RunObservabilityFromSnapshot(snapshot)
}
