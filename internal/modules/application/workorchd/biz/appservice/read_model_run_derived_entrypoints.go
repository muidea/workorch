package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ReadRunObservabilityWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret readmodelcore.RunObservabilityState, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.RunObservabilityState, error) {
		return readmodelports.RunObservability(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunObservabilityWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (readmodelcore.RunObservabilityState, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.RunObservabilityState, error) {
		return ReadRunObservabilityWithSession(ctx, session, runID)
	})
}

func ReadRunObservabilityFromEnv(ctx context.Context, runID string) (readmodelcore.RunObservabilityState, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.RunObservabilityState, error) {
		return ReadRunObservabilityWithSession(ctx, session, runID)
	})
}

func ReadRunTaskProgressWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret projectioncontract.TaskProgress, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (projectioncontract.TaskProgress, error) {
		return readmodelports.RunTaskProgress(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (projectioncontract.TaskProgress, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.TaskProgress, error) {
		return ReadRunTaskProgressWithSession(ctx, session, runID)
	})
}

func ReadRunTaskProgressFromEnv(ctx context.Context, runID string) (projectioncontract.TaskProgress, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.TaskProgress, error) {
		return ReadRunTaskProgressWithSession(ctx, session, runID)
	})
}

func ReadRunContextBundleWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret projectioncontract.ContextBundle, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (projectioncontract.ContextBundle, error) {
		return readmodelports.RunContextBundle(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (projectioncontract.ContextBundle, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.ContextBundle, error) {
		return ReadRunContextBundleWithSession(ctx, session, runID)
	})
}

func ReadRunContextBundleFromEnv(ctx context.Context, runID string) (projectioncontract.ContextBundle, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (projectioncontract.ContextBundle, error) {
		return ReadRunContextBundleWithSession(ctx, session, runID)
	})
}

func ReadRunTaskDecompositionsWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret []projectioncontract.TaskDecomposition, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]projectioncontract.TaskDecomposition, error) {
		return readmodelports.RunTaskDecompositions(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, runID string) ([]projectioncontract.TaskDecomposition, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]projectioncontract.TaskDecomposition, error) {
		return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
	})
}

func ReadRunTaskDecompositionsFromEnv(ctx context.Context, runID string) ([]projectioncontract.TaskDecomposition, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]projectioncontract.TaskDecomposition, error) {
		return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
	})
}
