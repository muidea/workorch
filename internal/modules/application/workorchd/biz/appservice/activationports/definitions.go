package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceExecutionDefinitionContext struct {
	EffectiveGoal          *workspacemodel.Goal
	EffectiveDoD           *workspacemodel.DoD
	GoalContexts           []map[string]any
	DoDContexts            []map[string]any
	DefinitionChangeImpact map[string]any
}

func selectWorkspaceExecutionDefinitions(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Goal, *workspacemodel.DoD, error) {
	if hub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	var goal *workspacemodel.Goal
	var dod *workspacemodel.DoD
	err := readmodelcore.RunTasks(
		func() error {
			var err error
			goal, err = activationcore.SelectWorkspaceExecutionGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
				return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
			}, workspaceID, "")
			return err
		},
		func() error {
			var err error
			dod, err = activationcore.SelectWorkspaceExecutionDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
				return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
			}, workspaceID, "")
			return err
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return goal, dod, nil
}

// 封存定义只能使用已准入的原值；前序身份用于变更说明，不合并旧承诺或重新解析文本。
func sealedWorkspaceExecutionDefinitionsContext(goal *workspacemodel.Goal, dod *workspacemodel.DoD) WorkspaceExecutionDefinitionContext {
	effectiveGoal := workspacemodel.CloneGoal(goal)
	effectiveDoD := workspacemodel.CloneDoD(dod)
	goalContexts := []map[string]any{activationcore.GoalContext(effectiveGoal)}
	dodContexts := []map[string]any{activationcore.DoDContext(effectiveDoD)}
	return WorkspaceExecutionDefinitionContext{
		EffectiveGoal:          effectiveGoal,
		EffectiveDoD:           effectiveDoD,
		GoalContexts:           goalContexts,
		DoDContexts:            dodContexts,
		DefinitionChangeImpact: activationcore.DefinitionChangeImpactFromDefinitions(effectiveGoal, goalContexts, nil, effectiveDoD, dodContexts, nil),
	}
}

func WorkspaceExecutionDefinitionsContext(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (WorkspaceExecutionDefinitionContext, error) {
	if hub == nil || workspaceID == "" || goal == nil || dod == nil {
		return WorkspaceExecutionDefinitionContext{}, common.ErrInvalidArgument
	}
	effectiveGoal, goalContexts, err := activationcore.EffectiveWorkspaceGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
		return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
	}, workspaceID, goal)
	if err != nil {
		return WorkspaceExecutionDefinitionContext{}, err
	}
	effectiveDoD, doDContexts, err := activationcore.EffectiveWorkspaceDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
		return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
	}, workspaceID, dod)
	if err != nil {
		return WorkspaceExecutionDefinitionContext{}, err
	}
	return WorkspaceExecutionDefinitionContext{
		EffectiveGoal:          effectiveGoal,
		EffectiveDoD:           effectiveDoD,
		GoalContexts:           goalContexts,
		DoDContexts:            doDContexts,
		DefinitionChangeImpact: activationcore.DefinitionChangeImpactFromDefinitions(effectiveGoal, goalContexts, nil, effectiveDoD, doDContexts, nil),
	}, nil
}
