package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ActiveRunDefinitionStale(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, run *runmodel.Run, selectedGoal *workspacemodel.Goal, selectedDoD *workspacemodel.DoD) bool {
	if hub == nil || run == nil || selectedGoal == nil || selectedDoD == nil {
		return true
	}
	if run.GoalID != selectedGoal.ID || run.DoDID != selectedDoD.ID {
		return true
	}
	if len(run.ExecutionContext) == 0 {
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
