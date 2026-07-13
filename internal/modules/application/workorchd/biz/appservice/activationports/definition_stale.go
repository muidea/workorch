package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ActiveRunDefinitionStale(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, run *runmodel.Run, selectedGoal *workspacemodel.Goal, selectedDoD *workspacemodel.DoD) bool {
	if hub == nil || run == nil || selectedGoal == nil || selectedDoD == nil {
		return true
	}
	if run.GoalID != selectedGoal.ID || run.DoDID != selectedDoD.ID {
		return true
	}
	if run.RuntimeBootstrap == nil {
		return true
	}
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	effectiveGoal, _, err := activationcore.EffectiveWorkspaceGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
		return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspaceID, selectedGoal)
	if err != nil || effectiveGoal == nil {
		return true
	}
	effectiveDoD, _, err := activationcore.EffectiveWorkspaceDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
		return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspaceID, selectedDoD)
	if err != nil || effectiveDoD == nil {
		return true
	}
	return activationcore.ActiveRunDefinitionContextStale(run, effectiveGoal, effectiveDoD)
}
