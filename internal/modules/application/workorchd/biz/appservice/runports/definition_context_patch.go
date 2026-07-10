package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func WorkspaceDefinitionContextPatch(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.WorkspaceID == "" || item.GoalID == "" || item.DoDID == "" {
		return nil, nil
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	var (
		workspace *workspacemodel.Workspace
		goal      *workspacemodel.Goal
		dod       *workspacemodel.DoD
	)
	if err := readmodelcore.RunTasks(
		func() error {
			var err error
			workspace, err = workspaceports.ReadWorkspaceRecord(ctx, hub, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			goal, err = activationcore.SelectWorkspaceExecutionGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
				return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
			}, eventRecords, item.WorkspaceID, item.GoalID)
			return err
		},
		func() error {
			var err error
			dod, err = activationcore.SelectWorkspaceExecutionDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
				return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
			}, eventRecords, item.WorkspaceID, item.DoDID)
			return err
		},
	); err != nil {
		return nil, err
	}
	if workspace == nil || goal == nil || dod == nil {
		return nil, common.ErrInvalidArgument
	}
	effectiveGoal, workspaceGoals, err := activationcore.EffectiveWorkspaceGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
		return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspace.ID, goal)
	if err != nil {
		return nil, err
	}
	effectiveDoD, workspaceDoDs, err := activationcore.EffectiveWorkspaceDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
		return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
	}, eventRecords, workspace.ID, dod)
	if err != nil {
		return nil, err
	}
	goalRevisions := activationcore.WorkspaceDefinitionRevisionContexts(ctx, eventRecords, workspace.ID, workspaceevents.TopicGoalDefined)
	doDRevisions := activationcore.WorkspaceDefinitionRevisionContexts(ctx, eventRecords, workspace.ID, workspaceevents.TopicDoDDefined)
	return workspaceDefinitionContextPatchFromDefinitions(effectiveGoal, workspaceGoals, goalRevisions, effectiveDoD, workspaceDoDs, doDRevisions), nil
}
