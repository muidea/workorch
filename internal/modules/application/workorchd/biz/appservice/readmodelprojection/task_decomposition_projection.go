package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func taskDecompositionFromStep(snapshot readmodelcore.RunSnapshotState, step projectioncontract.StepView, executedByNode map[string]projectioncontract.StepView) (projectioncontract.TaskDecomposition, bool) {
	graph := taskGraphFromValue(step.Output["executionGraph"])
	nodes := readModelMapSliceValue(graph["nodes"])
	if len(nodes) == 0 {
		return projectioncontract.TaskDecomposition{}, false
	}
	rootStepID := step.ID
	blockingTickets := taskBlockingTicketByStep(snapshot)
	rootMeta := taskLifecycleMetadataFromStep(step)
	if ticketID := blockingTickets[rootStepID]; ticketID != "" {
		rootMeta.blockingTicketID = ticketID
	}
	item := projectioncontract.TaskDecomposition{
		ID:          rootStepID + "/task-decomposition",
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		RootStepID:  rootStepID,
		State:       "pending",
		Summary:     TimelineStepSummary(step.Output),
		GraphBranch: readModelStringValue(step.Output["graphBranch"]),
		NodeTotal:   len(nodes),
		EvidenceRefs: compactUniqueStrings([]string{
			rootStepID,
		}),
	}
	for _, node := range nodes {
		addTaskDecompositionNode(&item, node, executedByNode, rootMeta, blockingTickets)
	}
	item.State = taskDecompositionState(item)
	item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
	return item, true
}
