package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func cloneTaskDecompositionNodes(items []readmodelcore.TaskDecompositionNode) []readmodelcore.TaskDecompositionNode {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.TaskDecompositionNode, 0, len(items))
	for _, item := range items {
		clone := item
		clone.DependsOn = cloneReadModelStringSlice(item.DependsOn)
		clone.EvidenceRefs = cloneReadModelStringSlice(item.EvidenceRefs)
		clone.CoverageTargets = cloneReadModelStringSlice(item.CoverageTargets)
		clone.Input = cloneReadModelMap(item.Input)
		ret = append(ret, clone)
	}
	return ret
}
