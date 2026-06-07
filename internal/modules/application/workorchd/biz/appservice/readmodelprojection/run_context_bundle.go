package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapRunContextBundle(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextBundle {
	progress := RunSnapshotTaskProgress(snapshot)
	timeline := RuntimeTimeline(snapshot)
	recentEvidence := ContextRecentEvidence(timeline, 12)
	ret := readmodelcore.ContextBundle{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		Mode:             "run_recovery",
		Summary:          ContextBundleSummary(snapshot, progress),
		TaskProgress:     progress,
		Recovery:         progress.Recovery,
		LatestCheckpoint: LatestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		RecentEvidence:   recentEvidence,
	}
	ret.Sections = runContextBundleSections(snapshot, progress, recentEvidence)
	ret.Sections = CompactContextSections(ret.Sections)
	ret.CompressionCandidates = ContextCompressionCandidates(ret)
	ret.CompressedContext = CompressedContextFromBundle(ret, localizationcore.RetentionPolicy(SnapshotLanguage(snapshot)))
	return ret
}
