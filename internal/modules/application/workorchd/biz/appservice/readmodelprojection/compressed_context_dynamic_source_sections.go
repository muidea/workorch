package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func appendCompressedContextDynamicSourceSections(sections []projectioncontract.ContextSection, snapshot readmodelcore.RunSnapshotState, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	sections = append(sections, projectioncontract.ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "governance", Priority: 80})
	}
	if len(RunScopedResults(snapshot)) > 0 || len(RunScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "recent_evidence", Priority: 70})
	}
	return sections
}
