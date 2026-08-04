package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func addTaskDecompositionNode(item *projectioncontract.TaskDecomposition, node map[string]any, executedByNode map[string]projectioncontract.StepView, rootMeta taskLifecycleMetadata, blockingTickets map[string]string) {
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
