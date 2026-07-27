package activationcore

import (
	"fmt"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func ActiveRunDefinitionContextStale(run *runmodel.Run, effectiveGoal *workspacemodel.Goal, effectiveDoD *workspacemodel.DoD) bool {
	if run == nil || effectiveGoal == nil || effectiveDoD == nil || run.RuntimeBootstrap == nil {
		return true
	}
	if run.GoalID != effectiveGoal.ID || run.DoDID != effectiveDoD.ID {
		return true
	}
	definition := run.RuntimeBootstrap.Definition.Normalized()
	if definition.GoalID != effectiveGoal.ID || definition.DoDID != effectiveDoD.ID || strings.TrimSpace(definition.GoalDefinition) != strings.TrimSpace(effectiveGoal.Text) {
		return true
	}
	dodLines := make([]string, 0, len(effectiveDoD.Items))
	for idx, item := range effectiveDoD.Items {
		if item = strings.TrimSpace(item); item != "" {
			dodLines = append(dodLines, fmt.Sprintf("%d. %s", idx+1, item))
		}
	}
	return strings.TrimSpace(definition.DoDDefinition) != strings.Join(dodLines, "\n") || strings.TrimSpace(run.DefinitionRevision) != definition.Revision
}
