package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: language_projection.go

func snapshotLanguage(snapshot readmodelcore.RunSnapshotState) string {
	return localizationcore.DefaultDialogLanguage()
}

// source: workspace_language_projection.go

func workspaceSnapshotLanguage(snapshot readmodelcore.WorkspaceSnapshotState) string {
	return localizationcore.DefaultDialogLanguage()
}
