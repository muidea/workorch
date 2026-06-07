package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func WorkspaceSnapshotLanguage(snapshot readmodelcore.WorkspaceSnapshot) string {
	if snapshot.Workspace.ActiveRunID != "" {
		for _, item := range snapshot.Runs {
			if item.ID == snapshot.Workspace.ActiveRunID {
				return LanguageFromExecutionContext(item.ExecutionContext)
			}
		}
	}
	for idx := len(snapshot.Runs) - 1; idx >= 0; idx-- {
		if lang := LanguageFromExecutionContext(snapshot.Runs[idx].ExecutionContext); lang != "" {
			return lang
		}
	}
	return localizationcore.DefaultDialogLanguage()
}
