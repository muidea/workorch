package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func WorkspaceSnapshotTaskProgress(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.TaskProgress {
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress
	}
	return MapLocalizedWorkspaceTaskProgress(snapshot)
}

func WorkspaceContextBundleFromSnapshots(snapshot readmodelcore.WorkspaceSnapshot, runSnapshot *readmodelcore.RunSnapshot) readmodelcore.ContextBundle {
	if snapshot.TaskProgress.RunID != "" && runSnapshot != nil {
		return MapRunContextBundle(*runSnapshot)
	}
	return MapWorkspaceContextBundle(snapshot)
}

func WorkspaceTaskDecompositionsFromSnapshots(snapshot readmodelcore.WorkspaceSnapshot, runSnapshot *readmodelcore.RunSnapshot) []readmodelcore.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	if snapshot.TaskProgress.RunID == "" || runSnapshot == nil {
		return nil
	}
	return runSnapshot.TaskDecompositions
}
