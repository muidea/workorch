package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapExecutionSections(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]projectioncontract.ExecutionSection, 0)
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
