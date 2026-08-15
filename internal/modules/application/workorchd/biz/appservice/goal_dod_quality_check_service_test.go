package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
)

func TestCheckGoalDoDQualityWithBackendFromStoredDefinitionsReady(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-quality-ready", "quality ready workspace", "", "workspaces/quality-ready")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "实现 workspace 级别的 Goal/DoD 质量预检接口")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{
		"执行 go test ./... 并记录结果",
		"输出 unified_diff 和交付物索引 evidenceRefs",
		"说明已知风险与限制",
	})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	check, err := CheckGoalDoDQualityWithBackend(context.Background(), "file", dataRoot, definitionquality.Command{
		WorkspaceID: workspace.ID,
		GoalID:      goal.ID,
		DoDID:       dod.ID,
	})
	if err != nil {
		t.Fatalf("CheckGoalDoDQualityWithBackend: %v", err)
	}
	if check.WorkspaceID != workspace.ID || check.GoalID != goal.ID || check.DoDID != dod.ID {
		t.Fatalf("check identity = %+v, want workspace/goal/dod ids", check)
	}
	if !check.Executable || !check.Verifiable || check.Status != "ready" {
		t.Fatalf("check state = executable:%v verifiable:%v status:%s, want ready", check.Executable, check.Verifiable, check.Status)
	}
	if check.ExecutionScore < 60 || check.VerificationScore < 60 {
		t.Fatalf("scores = %d/%d, want >= 60", check.ExecutionScore, check.VerificationScore)
	}
}

func TestCheckGoalDoDQualityWithBackendDraftNeedsRefinement(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-quality-draft", "quality draft workspace", "", "workspaces/quality-draft")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	check, err := CheckGoalDoDQualityWithBackend(context.Background(), "file", dataRoot, definitionquality.Command{
		WorkspaceID: workspace.ID,
		GoalText:    "修一下",
		DoDItems:    []string{"完成"},
	})
	if err != nil {
		t.Fatalf("CheckGoalDoDQualityWithBackend: %v", err)
	}
	if check.Status != "needs_refinement" {
		t.Fatalf("status = %q, want needs_refinement", check.Status)
	}
	if check.Verifiable {
		t.Fatalf("verifiable = true, want false")
	}
	if len(check.Issues) == 0 || len(check.Suggestions) == 0 {
		t.Fatalf("issues/suggestions = %+v/%+v, want non-empty", check.Issues, check.Suggestions)
	}
}

func TestCheckGoalDoDQualityWithBackendRequiresWorkspaceID(t *testing.T) {
	if _, err := CheckGoalDoDQualityWithBackend(context.Background(), "file", t.TempDir(), definitionquality.Command{}); err == nil {
		t.Fatal("expected invalid argument")
	}
}
