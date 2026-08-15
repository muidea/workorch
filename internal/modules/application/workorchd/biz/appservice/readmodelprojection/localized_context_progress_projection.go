package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: localized_compressed_context.go

func compressedContextFromLocalizedRunSnapshot(snapshot readmodelcore.RunSnapshotState) projectioncontract.CompressedContext {
	language := snapshotLanguage(snapshot)
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	return compressedContextFromRunSnapshot(snapshot, progress, recentEvidence, contextBundleSummary(snapshot, progress), localizationcore.RetentionPolicy(language))
}

// source: localized_progress.go

func mapLocalizedWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress {
	language := workspaceSnapshotLanguage(snapshot)
	return mapWorkspaceTaskProgress(snapshot, workspaceTaskProgressLabels{
		NoActiveRun:      localizationcore.WorkspaceProgressText(language, "no_active_run"),
		ActiveRun:        localizationcore.WorkspaceProgressText(language, "active_run"),
		WorkspaceDefined: localizationcore.WorkspaceProgressText(language, "workspace_defined"),
		FormalResult:     localizationcore.MilestoneTitle(language, "formal_result"),
		Runs:             localizationcore.WorkspaceProgressText(language, "runs"),
		Results:          localizationcore.WorkspaceProgressText(language, "results"),
		Artifacts:        localizationcore.WorkspaceProgressText(language, "artifacts"),
	})
}

func mapLocalizedTaskProgress(snapshot readmodelcore.RunSnapshotState) projectioncontract.TaskProgress {
	language := snapshotLanguage(snapshot)
	return mapTaskProgress(snapshot, runTaskProgressLabels{
		RecoveryNextAction: localizationcore.RecoveryNextAction(language),
		RunCreated:         localizationcore.MilestoneTitle(language, "run_created"),
		ExecutionSteps:     localizationcore.MilestoneTitle(language, "execution_steps"),
		Governance:         localizationcore.MilestoneTitle(language, "governance"),
		Validation:         localizationcore.MilestoneTitle(language, "validation"),
		FormalResult:       localizationcore.MilestoneTitle(language, "formal_result"),
	})
}

// source: run_context_bundle.go

func mapRunContextBundle(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextBundle {
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	ret := projectioncontract.ContextBundle{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		Mode:             "run_recovery",
		Summary:          contextBundleSummary(snapshot, progress),
		TaskProgress:     progress,
		Recovery:         progress.Recovery,
		LatestCheckpoint: latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		RecentEvidence:   recentEvidence,
	}
	ret.Sections = runContextBundleSections(snapshot, progress, recentEvidence)
	ret.Sections = compactContextSections(ret.Sections)
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, localizationcore.RetentionPolicy(snapshotLanguage(snapshot)))
	return ret
}

// source: run_context_bundle_sections.go

func runContextBundleSections(snapshot readmodelcore.RunSnapshotState, progress projectioncontract.TaskProgress, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
		contextWorkspaceSection(snapshot),
		contextRunSection(snapshot),
		contextGoalSection(snapshot),
		contextDoDSection(snapshot),
		contextTaskProgressSection(progress),
		contextTaskDecompositionSection(snapshot.TaskDecompositions),
		contextExecutionSectionsSection(snapshot.ExecutionSections),
		contextMemoryRecallSection(snapshot.MemoryRecallTraces),
		contextGovernanceSection(snapshot, localizationcore.MilestoneTitle(snapshotLanguage(snapshot), "governance")),
		contextFormalOutputSection(snapshot),
		contextRecentEvidenceSection(recentEvidence),
		contextRecoverySection(progress.Recovery),
	}
}

// source: workspace_context_bundle.go

func mapWorkspaceContextBundle(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.ContextBundle {
	language := workspaceSnapshotLanguage(snapshot)
	progress := snapshot.TaskProgress
	ret := projectioncontract.ContextBundle{
		WorkspaceID:  snapshot.Workspace.ID,
		Mode:         "workspace",
		Summary:      localizationcore.WorkspaceProgressText(language, "context_summary"),
		TaskProgress: progress,
		Recovery:     progress.Recovery,
		Sections:     workspaceContextSections(snapshot, progress, language),
	}
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, localizationcore.RetentionPolicy(language))
	return ret
}

// source: workspace_context_sections.go

func workspaceContextSections(snapshot readmodelcore.WorkspaceSnapshotState, progress projectioncontract.TaskProgress, language string) []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
		{
			ID:       "workspace",
			Kind:     "workspace",
			Title:    localizationcore.WorkspaceProgressText(language, "workspace"),
			Priority: 100,
			State:    snapshot.Workspace.State,
			Summary:  snapshot.Workspace.Title,
			Data: map[string]any{
				"id":                   snapshot.Workspace.ID,
				"title":                snapshot.Workspace.Title,
				"description":          snapshot.Workspace.Description,
				"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
			},
		},
		{
			ID:           "task_progress",
			Kind:         "task_progress",
			Title:        localizationcore.WorkspaceProgressText(language, "task_progress"),
			Priority:     90,
			State:        progress.State,
			Summary:      progress.Summary,
			EvidenceRefs: progress.EvidenceRefs,
		},
	}
}
