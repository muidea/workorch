package runports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func workspaceDefinitionContextPatchFromDefinitions(effectiveGoal *workspacemodel.Goal, workspaceGoals []map[string]any, goalRevisions []map[string]any, effectiveDoD *workspacemodel.DoD, workspaceDoDs []map[string]any, doDRevisions []map[string]any) map[string]any {
	if effectiveGoal == nil || effectiveDoD == nil {
		return nil
	}
	patch := map[string]any{
		"goalId":             string(effectiveGoal.ID),
		"goal":               activationcore.GoalContext(effectiveGoal),
		"doDId":              string(effectiveDoD.ID),
		"dod":                activationcore.DoDContext(effectiveDoD),
		"definitionRevision": activationcore.CurrentWorkspaceDefinitionRevisionContext(effectiveGoal, workspaceGoals, effectiveDoD, workspaceDoDs),
	}
	if len(workspaceGoals) > 0 {
		patch["goals"] = workspaceGoals
		patch["goalDefinitionPolicy"] = "workspace_goals_are_cumulative_latest_refines_previous"
	}
	if len(workspaceDoDs) > 0 {
		patch["dods"] = workspaceDoDs
		patch["doDDefinitionPolicy"] = "workspace_dods_are_cumulative_latest_refines_previous"
	}
	if impact := activationcore.DefinitionChangeImpactFromDefinitions(effectiveGoal, workspaceGoals, goalRevisions, effectiveDoD, workspaceDoDs, doDRevisions); len(impact) > 0 {
		patch["definitionChangeImpact"] = impact
	}
	return patch
}
