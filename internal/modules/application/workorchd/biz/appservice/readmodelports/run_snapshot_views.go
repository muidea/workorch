package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RunObservability(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunObservabilityState, error) {
	snapshot, err := runSnapshot(ctx, readers, runID, runSnapshotLoadObservability, false)
	if err != nil {
		return readmodelcore.RunObservabilityState{}, err
	}
	return RunObservabilityFromSnapshot(snapshot), nil
}

func RunTaskProgress(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (projectioncontract.TaskProgress, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return projectioncontract.TaskProgress{}, err
	}
	return readmodelprojection.RunSnapshotTaskProgress(snapshot), nil
}

func RunContextBundle(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (projectioncontract.ContextBundle, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return projectioncontract.ContextBundle{}, err
	}
	return readmodelprojection.MapRunContextBundle(snapshot), nil
}

func RunTaskDecompositions(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) ([]projectioncontract.TaskDecomposition, error) {
	snapshot, err := RunSnapshot(ctx, readers, runID)
	if err != nil {
		return nil, err
	}
	return readmodelprojection.RunSnapshotTaskDecompositions(snapshot), nil
}

func RunStatusFromSnapshot(snapshot readmodelcore.RunSnapshotState) projectioncontract.RunStatus {
	return readmodelprojection.MapRunStatus(snapshot)
}

func RunObservabilityFromSnapshot(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunObservabilityState {
	return readmodelprojection.MapRunObservability(snapshot)
}

func ProjectRunSnapshotDetail(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	return readmodelprojection.ProjectRunSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	return readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectDecisionView(item projectioncontract.Decision) projectioncontract.Decision {
	return readmodelprojection.ProjectDecisionView(item)
}
