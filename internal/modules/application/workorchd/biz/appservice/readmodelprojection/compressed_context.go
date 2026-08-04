package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func CompressedContextFromBundle(bundle readmodelcore.ContextBundle, retentionPolicy string) readmodelcore.CompressedContext {
	return readmodelcore.CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        bundle.WorkspaceID,
		RunID:              bundle.RunID,
		Summary:            bundle.Summary,
		CurrentMilestone:   bundle.TaskProgress.CurrentMilestone,
		Milestones:         bundle.TaskProgress.Milestones,
		Blockers:           bundle.TaskProgress.Blockers,
		Recovery:           bundle.Recovery,
		RecentEvidence:     LastContextEvidence(bundle.RecentEvidence, 6),
		ResultRefs:         ResultRefsFromTaskProgress(bundle.TaskProgress),
		SourceSectionRefs:  compressedContextSectionRefsFromBundle(bundle.Sections),
		SourceEvidenceRefs: compressedContextSourceEvidenceRefs(bundle.TaskProgress, bundle.RecentEvidence),
		RetentionPolicy:    retentionPolicy,
	}
}

func CompressedContextFromRunSnapshot(snapshot readmodelcore.RunSnapshot, progress projectioncontract.TaskProgress, recentEvidence []readmodelcore.ContextEvidence, summary string, retentionPolicy string) readmodelcore.CompressedContext {
	return readmodelcore.CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        snapshot.Run.WorkspaceID,
		RunID:              snapshot.Run.ID,
		Summary:            summary,
		CurrentMilestone:   progress.CurrentMilestone,
		Milestones:         progress.Milestones,
		Blockers:           progress.Blockers,
		Recovery:           progress.Recovery,
		RecentEvidence:     LastContextEvidence(recentEvidence, 6),
		ResultRefs:         ResultRefsFromTaskProgress(progress),
		SourceSectionRefs:  CompressedContextSourceSectionRefs(snapshot, recentEvidence),
		SourceEvidenceRefs: compressedContextSourceEvidenceRefs(progress, recentEvidence),
		RetentionPolicy:    retentionPolicy,
	}
}
