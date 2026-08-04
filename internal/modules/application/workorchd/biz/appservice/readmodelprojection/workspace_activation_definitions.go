package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func addActivationDefinitions(snapshot *readmodelcore.WorkspaceSnapshot, goal *workspacemodel.Goal, dod *workspacemodel.DoD) {
	if snapshot == nil {
		return
	}
	if goal != nil {
		snapshot.Goals = []workorchmanagementcontract.Goal{definitioncore.MapGoal(goal)}
	}
	if dod != nil {
		snapshot.DoDs = []workorchmanagementcontract.DoD{definitioncore.MapDoD(dod)}
	}
}
