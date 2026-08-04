package readmodelprojection

import (
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
