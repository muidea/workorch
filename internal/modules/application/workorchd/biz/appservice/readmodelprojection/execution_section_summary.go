package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func executionSectionSummary(node readmodelcore.TaskDecompositionNode) string {
	switch node.State {
	case "completed":
		return "section completed with linked runtime evidence"
	case "failed":
		return "section failed; inspect linked evidence and governance records"
	case "running":
		return "section is running"
	default:
		return "section is planned"
	}
}
