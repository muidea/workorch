package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func WorkspaceSnapshotLanguage(snapshot readmodelcore.WorkspaceSnapshotState) string {
	return localizationcore.DefaultDialogLanguage()
}
