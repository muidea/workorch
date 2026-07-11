package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func addActivationDefinitions(snapshot *readmodelcore.WorkspaceSnapshot, goal *workspacemodel.Goal, dod *workspacemodel.DoD) {
	if snapshot == nil {
		return
	}
	if goal != nil {
		snapshot.Goals = []appmodelcore.Goal{definitioncore.MapGoal(goal)}
	}
	if dod != nil {
		snapshot.DoDs = []appmodelcore.DoD{definitioncore.MapDoD(dod)}
	}
}
