package readmodelprojection

import (
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: step_view_projection.go

func projectStepView(item projectioncontract.StepView) projectioncontract.StepView {
	kind := llmcommon.CanonicalRuntimeBlockKind(item.Kind)
	return projectioncontract.StepView{
		ID:            item.ID,
		Kind:          kind,
		State:         item.State,
		Output:        projectStepOutput(kind, item.Output),
		ContractAudit: cloneStepViewMap(projectStepContractAudit(item.Input, item.Output, item.ContractAudit)),
	}
}

// source: task_decomposition_node.go

func taskDecompositionNode(raw map[string]any, executedByNode map[string]projectioncontract.StepView) projectioncontract.TaskDecompositionNode {
	nodeID := readModelStringValue(raw["id"])
	input, _ := raw["input"].(map[string]any)
	ret := projectioncontract.TaskDecompositionNode{
		ID:        nodeID,
		Kind:      llmcommon.CanonicalRuntimeBlockKind(readModelStringValue(raw["kind"])),
		Title:     taskNodeTitle(raw),
		State:     "pending",
		DependsOn: readModelStringSliceValue(raw["dependsOn"]),
		Branch:    readModelStringValue(raw["branch"]),
		Input:     cloneReadModelMap(input),
	}
	ret.CoverageTargets = taskCoverageTargetsFromInput(ret.Input)
	if executedByNode != nil {
		if step, ok := executedByNode[nodeID]; ok {
			ret.StepID = step.ID
			ret.StepState = step.State
			ret.State = taskNodeStateFromStep(step.State)
			ret.EvidenceRefs = compactUniqueStrings([]string{nodeID, step.ID})
			return ret
		}
	}
	ret.EvidenceRefs = compactUniqueStrings([]string{nodeID})
	return ret
}
