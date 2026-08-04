package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceSnapshotTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress {
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress
	}
	return MapLocalizedWorkspaceTaskProgress(snapshot)
}

func WorkspaceContextBundleFromSnapshots(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) projectioncontract.ContextBundle {
	if snapshot.TaskProgress.RunID != "" && runSnapshot != nil {
		return MapRunContextBundle(*runSnapshot)
	}
	return MapWorkspaceContextBundle(snapshot)
}

func WorkspaceTaskDecompositionsFromSnapshots(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	if snapshot.TaskProgress.RunID == "" || runSnapshot == nil {
		return nil
	}
	return runSnapshot.TaskDecompositions
}
