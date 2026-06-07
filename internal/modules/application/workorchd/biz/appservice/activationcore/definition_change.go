package activationcore

import workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"

func DefinitionChangeImpactFromDefinitions(effectiveGoal *workspacemodel.Goal, goals []map[string]any, goalRevisions []map[string]any, effectiveDoD *workspacemodel.DoD, dods []map[string]any, doDRevisions []map[string]any) map[string]any {
	goalIDs := definitionIDsFromContexts(goals, "id")
	doDIDs := definitionIDsFromContexts(dods, "id")
	effectiveGoalID := ""
	if effectiveGoal != nil {
		effectiveGoalID = string(effectiveGoal.ID)
	}
	effectiveDoDID := ""
	if effectiveDoD != nil {
		effectiveDoDID = string(effectiveDoD.ID)
	}
	changed := len(goalIDs) > 1 || len(doDIDs) > 1 || len(goalRevisions) > 0 || len(doDRevisions) > 0
	if !changed {
		return nil
	}
	return map[string]any{
		"version":                        "workorch.definition_change_impact.v1",
		"changed":                        true,
		"policy":                         "workspace_goal_dod_cumulative_latest_refines_previous",
		"requiredPreExecutionAssessment": true,
		"goalChange": map[string]any{
			"effectiveGoalId":        effectiveGoalID,
			"latestGoalId":           LatestDefinitionID(goalIDs, effectiveGoalID),
			"previousGoalIds":        PreviousDefinitionIDs(goalIDs, effectiveGoalID),
			"allGoalIds":             goalIDs,
			"definitionRevisions":    CloneContextMapSlice(goalRevisions),
			"changedInPlace":         len(goalRevisions) > 0,
			"allGoalsAreConstraints": true,
		},
		"doDChange": map[string]any{
			"effectiveDoDId":                effectiveDoDID,
			"latestDoDId":                   LatestDefinitionID(doDIDs, effectiveDoDID),
			"previousDoDIds":                PreviousDefinitionIDs(doDIDs, effectiveDoDID),
			"allDoDIds":                     doDIDs,
			"definitionRevisions":           CloneContextMapSlice(doDRevisions),
			"changedInPlace":                len(doDRevisions) > 0,
			"allDoDItemsAreAcceptanceGates": true,
		},
		"strategyRequirements": []string{
			"preserve still-valid prior Goal and DoD constraints",
			"identify additive requirements and conflicts before state-changing work",
			"choose state-change, evidence, and verification targets before state-changing work",
			"do not weaken prior acceptance evidence or constraints",
		},
	}
}

func DefinitionChangeImpactChanged(impact map[string]any) bool {
	if len(impact) == 0 {
		return false
	}
	value, ok := impact["changed"].(bool)
	return ok && value
}
