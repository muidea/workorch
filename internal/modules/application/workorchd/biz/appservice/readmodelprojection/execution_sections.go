package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapExecutionSections(snapshot readmodelcore.RunSnapshot) []readmodelcore.ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]readmodelcore.ExecutionSection, 0)
	for _, decomposition := range snapshot.TaskDecompositions {
		for _, node := range decomposition.Nodes {
			ret = append(ret, executionSectionFromNode(snapshot, decomposition, node, dodItems, refs))
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
