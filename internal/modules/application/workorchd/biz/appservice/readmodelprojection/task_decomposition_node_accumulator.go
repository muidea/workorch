package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func addTaskDecompositionNode(item *readmodelcore.TaskDecomposition, node map[string]any, executedByNode map[string]StepView, rootMeta taskLifecycleMetadata, blockingTickets map[string]string) {
	taskNode := taskDecompositionNode(node, executedByNode)
	meta := rootMeta
	if taskNode.StepID != "" && executedByNode != nil {
		if step, ok := executedByNode[taskNode.ID]; ok {
			meta = taskLifecycleMetadataFromStep(step)
			if ticketID := blockingTickets[step.ID]; ticketID != "" {
				meta.blockingTicketID = ticketID
			}
		}
	}
	applyTaskLifecycleToNode(&taskNode, taskLifecycleMetadataForNode(taskNode, meta))
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
