package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RunSnapshotTaskProgress(snapshot readmodelcore.RunSnapshotState) projectioncontract.TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return MapLocalizedTaskProgress(snapshot)
}

func RunSnapshotTaskDecompositions(snapshot readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return MapTaskDecompositions(snapshot)
}

func MapTaskPlansFromSnapshot(snapshot readmodelcore.RunSnapshotState) []projectioncontract.TaskPlan {
	decompositions := RunSnapshotTaskDecompositions(snapshot)
	return MapTaskPlans(snapshot, decompositions)
}
