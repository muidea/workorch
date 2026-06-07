package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func projectTaskPlan(snapshot readmodelcore.RunSnapshot, decomposition readmodelcore.TaskDecomposition, step presenter.StepView, hasStep bool, occurrences map[string]string) readmodelcore.TaskPlan {
	fields := taskPlanProjectionFields(decomposition, step, hasStep, occurrences)
	return readmodelcore.TaskPlan{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		AnchorStepID:     fields.anchorStepID,
		RevisionStepID:   decomposition.RootStepID,
		State:            fields.state,
		Stage:            fields.stage,
		Summary:          decomposition.Summary,
		GraphBranch:      decomposition.GraphBranch,
		NodeTotal:        decomposition.NodeTotal,
		NodeCompleted:    decomposition.NodeCompleted,
		NodeFailed:       decomposition.NodeFailed,
		NodeRunning:      decomposition.NodeRunning,
		EvidenceRefs:     compactUniqueStrings(append([]string{decomposition.RootStepID}, decomposition.EvidenceRefs...)),
		OccurredAt:       fields.occurredAt,
		SourceProjection: "task_decomposition",
	}
}
