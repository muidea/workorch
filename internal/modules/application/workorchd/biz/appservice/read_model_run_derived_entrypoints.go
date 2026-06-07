package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunObservabilityWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunObservability, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (RunObservability, error) {
		return readmodelports.RunObservability(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunObservabilityWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunObservability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (RunObservability, error) {
		return ReadRunObservabilityWithSession(ctx, session, runID)
	})
}

func ReadRunObservabilityFromEnv(ctx context.Context, runID string) (RunObservability, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (RunObservability, error) {
		return ReadRunObservabilityWithSession(ctx, session, runID)
	})
}

func ReadRunTaskProgressWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret TaskProgress, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (TaskProgress, error) {
		return readmodelports.RunTaskProgress(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (TaskProgress, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (TaskProgress, error) {
		return ReadRunTaskProgressWithSession(ctx, session, runID)
	})
}

func ReadRunTaskProgressFromEnv(ctx context.Context, runID string) (TaskProgress, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (TaskProgress, error) {
		return ReadRunTaskProgressWithSession(ctx, session, runID)
	})
}

func ReadRunContextBundleWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret ContextBundle, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (ContextBundle, error) {
		return readmodelports.RunContextBundle(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (ContextBundle, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ContextBundle, error) {
		return ReadRunContextBundleWithSession(ctx, session, runID)
	})
}

func ReadRunContextBundleFromEnv(ctx context.Context, runID string) (ContextBundle, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (ContextBundle, error) {
		return ReadRunContextBundleWithSession(ctx, session, runID)
	})
}

func ReadRunTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret []TaskDecomposition, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]TaskDecomposition, error) {
		return readmodelports.RunTaskDecompositions(ctx, runSnapshotReaders(deps), common.RunID(runID))
	})
}

func ReadRunTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, runID string) ([]TaskDecomposition, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]TaskDecomposition, error) {
		return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
	})
}

func ReadRunTaskDecompositionsFromEnv(ctx context.Context, runID string) ([]TaskDecomposition, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]TaskDecomposition, error) {
		return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
	})
}
