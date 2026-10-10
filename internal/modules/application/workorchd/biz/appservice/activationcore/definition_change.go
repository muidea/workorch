package activationcore

import workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"

func DefinitionChangeImpactFromDefinitions(effectiveGoal *workspacemodel.Goal, goals []map[string]any, goalRevisions []map[string]any, effectiveDoD *workspacemodel.DoD, dods []map[string]any, doDRevisions []map[string]any) map[string]any {
	goalRevisions = definitionRevisionContextsForEffectiveGoal(effectiveGoal, goalRevisions)
	doDRevisions = definitionRevisionContextsForEffectiveDoD(effectiveDoD, doDRevisions)
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
		"version":                        "workorch.definition_change_impact",
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

func definitionRevisionContextsForEffectiveGoal(effectiveGoal *workspacemodel.Goal, revisions []map[string]any) []map[string]any {
	if effectiveGoal == nil || !definitionModelHasPriorRevision(effectiveGoal.PreviousDefinitionHash, string(effectiveGoal.PreviousGoalID)) {
		return revisions
	}
	return appendDefinitionModelRevisionContext(revisions, string(effectiveGoal.ID), effectiveGoal.DefinitionRevision, effectiveGoal.PreviousDefinitionHash, string(effectiveGoal.PreviousGoalID))
}

func definitionRevisionContextsForEffectiveDoD(effectiveDoD *workspacemodel.DoD, revisions []map[string]any) []map[string]any {
	if effectiveDoD == nil || !definitionModelHasPriorRevision(effectiveDoD.PreviousDefinitionHash, string(effectiveDoD.PreviousDoDID)) {
		return revisions
	}
	return appendDefinitionModelRevisionContext(revisions, string(effectiveDoD.ID), effectiveDoD.DefinitionRevision, effectiveDoD.PreviousDefinitionHash, string(effectiveDoD.PreviousDoDID))
}

func definitionModelHasPriorRevision(previousHash string, previousID string) bool {
	return previousHash != "" || previousID != ""
}

func appendDefinitionModelRevisionContext(revisions []map[string]any, id string, revision string, previousHash string, previousID string) []map[string]any {
	for _, item := range revisions {
		if item == nil || item["id"] != id {
			continue
		}
		return revisions
	}
	next := map[string]any{
		"id":                 id,
		"revisionCount":      2,
		"source":             "current_definition_model",
		"definitionRevision": revision,
	}
	if previousHash != "" {
		next["previousDefinitionHash"] = previousHash
	}
	if previousID != "" {
		next["previousDefinitionId"] = previousID
	}
	return append(CloneContextMapSlice(revisions), next)
}
