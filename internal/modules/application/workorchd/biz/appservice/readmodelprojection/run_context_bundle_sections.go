package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func runContextBundleSections(snapshot readmodelcore.RunSnapshotState, progress projectioncontract.TaskProgress, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
		ContextWorkspaceSection(snapshot),
		ContextRunSection(snapshot),
		ContextGoalSection(snapshot),
		ContextDoDSection(snapshot),
		ContextTaskProgressSection(progress),
		ContextTaskDecompositionSection(snapshot.TaskDecompositions),
		ContextExecutionSectionsSection(snapshot.ExecutionSections),
		ContextMemoryRecallSection(snapshot.MemoryRecallTraces),
		ContextGovernanceSection(snapshot, localizationcore.MilestoneTitle(SnapshotLanguage(snapshot), "governance")),
		ContextFormalOutputSection(snapshot),
		ContextRecentEvidenceSection(recentEvidence),
		ContextRecoverySection(progress.Recovery),
	}
}
