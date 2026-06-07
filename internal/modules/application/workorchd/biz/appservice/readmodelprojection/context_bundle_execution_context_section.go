package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextExecutionContextSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	data := cloneReadModelMap(snapshot.Run.ExecutionContext)
	delete(data, "goal")
	delete(data, "dod")
	delete(data, "compressedContext")
	delete(data, "coverageWorkItems")
	return readmodelcore.ContextSection{
		ID:       "execution_context",
		Kind:     "execution_context",
		Title:    "Execution context",
		Priority: 85,
		Summary:  "selected Soul, Skill, MCP, Memory, and provider context",
		Data:     data,
	}
}
