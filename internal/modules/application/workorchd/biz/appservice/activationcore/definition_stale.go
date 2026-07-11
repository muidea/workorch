package activationcore

import (
	"encoding/json"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func StableContextJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func ActiveRunDefinitionContextStale(run *runmodel.Run, effectiveGoal *workspacemodel.Goal, effectiveDoD *workspacemodel.DoD) bool {
	if run == nil || effectiveGoal == nil || effectiveDoD == nil {
		return true
	}
	if run.GoalID != effectiveGoal.ID || run.DoDID != effectiveDoD.ID {
		return true
	}
	if len(run.ExecutionContext) == 0 {
		return true
	}
	current := map[string]any{
		"goalId": string(effectiveGoal.ID),
		"goal":   GoalContext(effectiveGoal),
		"doDId":  string(effectiveDoD.ID),
		"dod":    DoDContext(effectiveDoD),
	}
	existing := map[string]any{
		"goalId": run.ExecutionContext["goalId"],
		"goal":   run.ExecutionContext["goal"],
		"doDId":  run.ExecutionContext["doDId"],
		"dod":    run.ExecutionContext["dod"],
	}
	return StableContextJSON(existing) != StableContextJSON(current)
}
