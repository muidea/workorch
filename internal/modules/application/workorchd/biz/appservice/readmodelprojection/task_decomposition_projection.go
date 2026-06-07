package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func taskDecompositionFromStep(snapshot readmodelcore.RunSnapshot, step presenter.StepView, executedByNode map[string]presenter.StepView) (readmodelcore.TaskDecomposition, bool) {
	graph := taskGraphFromValue(step.Output["executionGraph"])
	nodes := readModelMapSliceValue(graph["nodes"])
	if len(nodes) == 0 {
		return readmodelcore.TaskDecomposition{}, false
	}
	rootStepID := step.ID
	item := readmodelcore.TaskDecomposition{
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
		addTaskDecompositionNode(&item, node, executedByNode)
	}
	item.State = taskDecompositionState(item)
	item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
	return item, true
}
