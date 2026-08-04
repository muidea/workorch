package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectTaskPlan(snapshot readmodelcore.RunSnapshotState, decomposition projectioncontract.TaskDecomposition, step projectioncontract.StepView, hasStep bool, occurrences map[string]string) projectioncontract.TaskPlan {
	fields := taskPlanProjectionFields(decomposition, step, hasStep, occurrences)
	plan := projectioncontract.TaskPlan{
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
		Nodes:            cloneTaskDecompositionNodes(decomposition.Nodes),
		EvidenceRefs:     compactUniqueStrings(append([]string{decomposition.RootStepID}, decomposition.EvidenceRefs...)),
		OccurredAt:       fields.occurredAt,
		SourceProjection: "task_decomposition",
	}
	if hasStep {
		meta := taskLifecycleMetadataFromStep(step)
		if ticketID := taskBlockingTicketByStep(snapshot)[step.ID]; ticketID != "" {
			meta.blockingTicketID = ticketID
		}
		applyTaskLifecycleToPlan(&plan, meta)
	}
	return plan
}
