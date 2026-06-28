package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

func RunObservability(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunObservability, error) {
	snapshot, err := runSnapshot(ctx, readers, runID, runSnapshotLoadObservability, false)
	if err != nil {
		return readmodelcore.RunObservability{}, err
	}
	return RunObservabilityFromSnapshot(snapshot), nil
}

func RunTaskProgress(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.TaskProgress, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return readmodelcore.TaskProgress{}, err
	}
	return readmodelprojection.RunSnapshotTaskProgress(snapshot), nil
}

func RunContextBundle(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.ContextBundle, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return readmodelcore.ContextBundle{}, err
	}
	return readmodelprojection.MapRunContextBundle(snapshot), nil
}

func RunTaskDecompositions(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) ([]readmodelcore.TaskDecomposition, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return nil, err
	}
	return readmodelprojection.RunSnapshotTaskDecompositions(snapshot), nil
}

func RunStatusFromSnapshot(snapshot readmodelcore.RunSnapshot) readmodelcore.RunStatus {
	return readmodelprojection.MapRunStatus(snapshot)
}

func RunObservabilityFromSnapshot(snapshot readmodelcore.RunSnapshot) readmodelcore.RunObservability {
	return readmodelprojection.MapRunObservability(snapshot)
}

func ProjectRunSnapshotDetail(snapshot readmodelcore.RunSnapshot) readmodelcore.RunSnapshot {
	return readmodelprojection.ProjectRunSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	return readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectDecisionView(item readmodelcore.Decision) readmodelcore.Decision {
	return readmodelprojection.ProjectDecisionView(item)
}
