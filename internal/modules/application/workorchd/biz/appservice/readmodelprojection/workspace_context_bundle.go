package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapWorkspaceContextBundle(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.ContextBundle {
	language := WorkspaceSnapshotLanguage(snapshot)
	progress := snapshot.TaskProgress
	ret := readmodelcore.ContextBundle{
		WorkspaceID:  snapshot.Workspace.ID,
		Mode:         "workspace",
		Summary:      localizationcore.WorkspaceProgressText(language, "context_summary"),
		TaskProgress: progress,
		Recovery:     progress.Recovery,
		Sections:     workspaceContextSections(snapshot, progress, language),
	}
	ret.CompressionCandidates = ContextCompressionCandidates(ret)
	ret.CompressedContext = CompressedContextFromBundle(ret, localizationcore.RetentionPolicy(language))
	return ret
}
