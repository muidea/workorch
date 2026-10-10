package activationports

import (
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSealedWorkspaceExecutionDefinitionsPreserveExactSourcesAndLineage(t *testing.T) {
	goal := &workspacemodel.Goal{ID: "goal-current", WorkspaceID: "ws-sealed", Text: "  **原目标**；保留\r\n原文  ", DefinitionRevision: "revision-current"}
	dod := &workspacemodel.DoD{ID: "dod-current", WorkspaceID: goal.WorkspaceID, State: common.DoDStateReady, DefinitionRevision: goal.DefinitionRevision,
		Items: []string{"# 原判据\r\nA；B; C", "**重复原判据**", "**重复原判据**", "  原空白  "}}
	for _, successor := range []bool{false, true} {
		if successor {
			goal.PreviousGoalID, goal.PreviousDefinitionHash = "goal-previous", "definition-previous"
			dod.PreviousDoDID, dod.PreviousDefinitionHash = "dod-previous", "definition-previous"
		}
		view := sealedWorkspaceExecutionDefinitionsContext(goal, dod)
		if !reflect.DeepEqual(view.EffectiveGoal, goal) || !reflect.DeepEqual(view.EffectiveDoD, dod) {
			t.Fatal("封存输入被重新解析、去重或改写")
		}
		if len(view.GoalContexts) != 1 || len(view.DoDContexts) != 1 || view.GoalContexts[0]["id"] != string(goal.ID) || view.DoDContexts[0]["id"] != string(dod.ID) {
			t.Fatal("封存执行混入未准入定义")
		}
		impact := RuntimeDefinitionChangeImpact(view.DefinitionChangeImpact)
		if successor {
			if impact == nil || !impact.Changed || len(impact.GoalChange.DefinitionRevisions) != 1 || len(impact.DoDChange.DefinitionRevisions) != 1 ||
				impact.GoalChange.DefinitionRevisions[0].PreviousDefinitionID != "goal-previous" || impact.DoDChange.DefinitionRevisions[0].PreviousDefinitionID != "dod-previous" ||
				impact.GoalChange.EffectiveDefinitionID != string(goal.ID) || impact.DoDChange.EffectiveDefinitionID != string(dod.ID) {
				t.Fatal("后继变更说明丢失已封存前序身份")
			}
		} else if impact != nil {
			t.Fatal("首次定义虚构变更来源")
		}
		view.EffectiveGoal.Text = "changed"
		view.EffectiveDoD.Items[0] = "changed"
		view.DoDContexts[0]["items"].([]string)[1] = "changed"
		if goal.Text != "  **原目标**；保留\r\n原文  " || dod.Items[0] != "# 原判据\r\nA；B; C" || dod.Items[1] != "**重复原判据**" {
			t.Fatal("执行投影与 owner 来源共享可变数据")
		}
	}
}
