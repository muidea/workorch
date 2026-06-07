package definitioncore

import (
	"errors"
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceGoalConsolidationPlanSelectsLatestValidGoal(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-definition-plan")
	plan, err := WorkspaceGoalConsolidationPlan([]*workspacemodel.Goal{
		{ID: "goal-001", WorkspaceID: workspaceID, Text: "first"},
		{ID: "goal-other", WorkspaceID: "ws-other", Text: "other"},
		{ID: "goal-empty", WorkspaceID: workspaceID, Text: " "},
		{ID: "goal-002", WorkspaceID: workspaceID, Text: "second"},
	}, workspaceID, "third")
	if err != nil {
		t.Fatalf("WorkspaceGoalConsolidationPlan: %v", err)
	}
	if plan.CanonicalID != "goal-002" {
		t.Fatalf("canonical = %q, want goal-002", plan.CanonicalID)
	}
	if plan.Text != "- first\n- other\n- second\n- third" {
		t.Fatalf("text = %q", plan.Text)
	}
	if !reflect.DeepEqual(plan.ObsoleteIDs, []common.GoalID{"goal-001", "goal-empty"}) {
		t.Fatalf("obsolete ids = %#v", plan.ObsoleteIDs)
	}
}

func TestWorkspaceDoDConsolidationPlanSelectsLatestValidDoD(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-definition-plan")
	plan, err := WorkspaceDoDConsolidationPlan([]*workspacemodel.DoD{
		{ID: "dod-001", WorkspaceID: workspaceID, Items: []string{"first"}},
		{ID: "dod-empty", WorkspaceID: workspaceID, Items: []string{" "}},
		{ID: "dod-002", WorkspaceID: workspaceID, Items: []string{"second"}},
	}, workspaceID, []string{"first", "third"})
	if err != nil {
		t.Fatalf("WorkspaceDoDConsolidationPlan: %v", err)
	}
	if plan.CanonicalID != "dod-002" {
		t.Fatalf("canonical = %q, want dod-002", plan.CanonicalID)
	}
	if !reflect.DeepEqual(plan.Items, []string{"first", "second", "third"}) {
		t.Fatalf("items = %#v", plan.Items)
	}
	if !reflect.DeepEqual(plan.ObsoleteIDs, []common.DoDID{"dod-001", "dod-empty"}) {
		t.Fatalf("obsolete ids = %#v", plan.ObsoleteIDs)
	}
}

func TestWorkspaceConsolidationPlanRejectsMissingCanonical(t *testing.T) {
	if _, err := WorkspaceGoalConsolidationPlan(nil, "ws-definition-plan", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("goal error = %v, want ErrInvalidArgument", err)
	}
	if _, err := WorkspaceDoDConsolidationPlan(nil, "ws-definition-plan", nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("dod error = %v, want ErrInvalidArgument", err)
	}
}
