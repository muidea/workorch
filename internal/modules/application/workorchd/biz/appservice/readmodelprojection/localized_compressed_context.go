package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func CompressedContextFromLocalizedRunSnapshot(snapshot readmodelcore.RunSnapshot) readmodelcore.CompressedContext {
	language := SnapshotLanguage(snapshot)
	progress := RunSnapshotTaskProgress(snapshot)
	timeline := RuntimeTimeline(snapshot)
	recentEvidence := ContextRecentEvidence(timeline, 12)
	return CompressedContextFromRunSnapshot(snapshot, progress, recentEvidence, ContextBundleSummary(snapshot, progress), localizationcore.RetentionPolicy(language))
}
