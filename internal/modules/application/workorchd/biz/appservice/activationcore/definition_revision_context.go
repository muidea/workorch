package activationcore

import workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

func CurrentWorkspaceDefinitionRevisionContext(effectiveGoal *workspacemodel.Goal, goalContexts []map[string]any, effectiveDoD *workspacemodel.DoD, doDContexts []map[string]any) map[string]any {
	if effectiveGoal == nil || effectiveDoD == nil {
		return nil
	}
	return map[string]any{
		"policy":          "workspace_goal_dod_cumulative",
		"effectiveGoalId": string(effectiveGoal.ID),
		"effectiveDoDId":  string(effectiveDoD.ID),
		"goalIds":         definitionContextIDs(goalContexts, "id"),
		"doDIds":          definitionContextIDs(doDContexts, "id"),
	}
}

func definitionContextIDs(items []map[string]any, key string) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if id, _ := item[key].(string); id != "" {
			ret = append(ret, id)
		}
	}
	return ret
}
