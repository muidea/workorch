package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type WorkspaceExecutionDefinitionContext struct {
	EffectiveGoal          *workspacemodel.Goal
	EffectiveDoD           *workspacemodel.DoD
	GoalContexts           []map[string]any
	DoDContexts            []map[string]any
	DefinitionChangeImpact map[string]any
}

func SelectWorkspaceExecutionDefinitions(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, requestedGoalID common.GoalID, requestedDoDID common.DoDID) (*workspacemodel.Goal, *workspacemodel.DoD, error) {
	if hub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	eventRecords := readmodelports.EventRecordReader{Hub: hub}

	var goal *workspacemodel.Goal
	var dod *workspacemodel.DoD
	err := readmodelcore.RunTasks(
		func() error {
			var err error
			goal, err = activationcore.SelectWorkspaceExecutionGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
				return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
			}, eventRecords, workspaceID, requestedGoalID)
			return err
		},
		func() error {
			var err error
			dod, err = activationcore.SelectWorkspaceExecutionDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
				return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
			}, eventRecords, workspaceID, requestedDoDID)
			return err
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return goal, dod, nil
}

func WorkspaceExecutionDefinitionsContext(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (WorkspaceExecutionDefinitionContext, error) {
	if hub == nil || workspaceID == "" || goal == nil || dod == nil {
		return WorkspaceExecutionDefinitionContext{}, common.ErrInvalidArgument
	}
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	effectiveGoal, goalContexts, err := activationcore.EffectiveWorkspaceGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
		return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspaceID, goal)
	if err != nil {
		return WorkspaceExecutionDefinitionContext{}, err
	}
	effectiveDoD, doDContexts, err := activationcore.EffectiveWorkspaceDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
		return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspaceID, dod)
	if err != nil {
		return WorkspaceExecutionDefinitionContext{}, err
	}
	goalRevisions := activationcore.WorkspaceDefinitionRevisionContexts(ctx, eventRecords, workspaceID, events.TopicGoalDefined)
	doDRevisions := activationcore.WorkspaceDefinitionRevisionContexts(ctx, eventRecords, workspaceID, events.TopicDoDDefined)
	return WorkspaceExecutionDefinitionContext{
		EffectiveGoal:          effectiveGoal,
		EffectiveDoD:           effectiveDoD,
		GoalContexts:           goalContexts,
		DoDContexts:            doDContexts,
		DefinitionChangeImpact: activationcore.DefinitionChangeImpactFromDefinitions(effectiveGoal, goalContexts, goalRevisions, effectiveDoD, doDContexts, doDRevisions),
	}, nil
}
