package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func addTaskDecompositionNode(item *readmodelcore.TaskDecomposition, node map[string]any, executedByNode map[string]presenter.StepView) {
	taskNode := taskDecompositionNode(node, executedByNode)
	item.Nodes = append(item.Nodes, taskNode)
	item.EvidenceRefs = append(item.EvidenceRefs, taskNode.EvidenceRefs...)
	switch taskNode.State {
	case "completed":
		item.NodeCompleted++
	case "failed":
		item.NodeFailed++
	case "running":
		item.NodeRunning++
	}
}
