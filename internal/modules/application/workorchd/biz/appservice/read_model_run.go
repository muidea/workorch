package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	return readmodelports.RunSnapshot(ctx, runSnapshotReaders(deps), runID)
}

func readRunRecoverySnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	return readmodelports.RunRecoverySnapshot(ctx, runSnapshotReaders(deps), runID)
}

func readRunStatus(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunStatus, error) {
	candidates := ownerReaderCandidatesFromReadModelDependencies(deps)
	return readmodelports.RunStatus(ctx, candidates.RunStatusReaders(deps.ownerReaders), runID)
}

func runSnapshotReaders(deps readModelDependencies) readmodelports.RunSnapshotReaders {
	candidates := ownerReaderCandidatesFromReadModelDependencies(deps)
	return candidates.RunSnapshotReaders(deps.ownerReaders, deps.published)
}
