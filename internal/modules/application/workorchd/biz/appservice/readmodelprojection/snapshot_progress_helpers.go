package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func RunSnapshotTaskProgress(snapshot readmodelcore.RunSnapshot) readmodelcore.TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return MapLocalizedTaskProgress(snapshot)
}

func RunSnapshotTaskDecompositions(snapshot readmodelcore.RunSnapshot) []readmodelcore.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return MapTaskDecompositions(snapshot)
}

func MapTaskPlansFromSnapshot(snapshot readmodelcore.RunSnapshot) []readmodelcore.TaskPlan {
	decompositions := RunSnapshotTaskDecompositions(snapshot)
	return MapTaskPlans(snapshot, decompositions)
}
