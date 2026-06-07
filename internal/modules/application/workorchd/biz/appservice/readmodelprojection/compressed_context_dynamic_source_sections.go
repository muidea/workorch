package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func appendCompressedContextDynamicSourceSections(sections []readmodelcore.ContextSection, snapshot readmodelcore.RunSnapshot, recentEvidence []readmodelcore.ContextEvidence) []readmodelcore.ContextSection {
	if len(readModelMapValue(snapshot.Run.ExecutionContext["goal"])) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "goal", Priority: 95})
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["dod"])) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "dod", Priority: 95})
	}
	if HasNonDefinitionExecutionContext(snapshot.Run.ExecutionContext) {
		sections = append(sections, readmodelcore.ContextSection{ID: "execution_context", Priority: 85})
	}
	sections = append(sections, readmodelcore.ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "governance", Priority: 80})
	}
	if len(RunScopedResults(snapshot)) > 0 || len(RunScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, readmodelcore.ContextSection{ID: "recent_evidence", Priority: 70})
	}
	return sections
}
