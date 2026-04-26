package appservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestActivateWorkspaceExecutionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ActivateWorkspaceExecutionWithSession(context.Background(), nil, "ws-activation", ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendCreatesAndStartsRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-start", "activation start workspace", "", "workspaces/activation-start")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if !strings.HasPrefix(item.Run.ID, "run-") ||
		item.Run.WorkspaceID != workspace.ID ||
		item.Run.GoalID != goal.ID ||
		item.Run.DoDID != dod.ID ||
		item.Run.State != "failed" {
		t.Fatalf("activation run = %+v, want generated failed llm run with goal/dod", item.Run)
	}
	if item.Snapshot.Workspace.ID != workspace.ID || len(item.Snapshot.Runs) != 1 {
		t.Fatalf("activation snapshot = %+v, want workspace with one run", item.Snapshot)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(runSnapshot.Steps) != 1 || runSnapshot.Steps[0].Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("steps = %+v, want first llm workspace execution step", runSnapshot.Steps)
	}
	if stage, _ := runSnapshot.Steps[0].Output["stage"].(string); stage != "llm_provider_unavailable" {
		t.Fatalf("step output = %+v, want llm provider unavailable stage", runSnapshot.Steps[0].Output)
	}
	if len(runSnapshot.ToolCalls) != 1 || runSnapshot.ToolCalls[0].State != "failed" {
		t.Fatalf("toolCalls = %+v, want failed runtime evidence", runSnapshot.ToolCalls)
	}
	if len(runSnapshot.ValidationResults) != 1 || runSnapshot.ValidationResults[0].State != "failed" {
		t.Fatalf("validationResults = %+v, want failed runtime evidence", runSnapshot.ValidationResults)
	}
	if len(runSnapshot.RuntimeCheckpoints) == 0 {
		t.Fatalf("runtimeCheckpoints empty, want runtime checkpoints")
	}
	if len(runSnapshot.Patches) != 0 {
		t.Fatalf("patches = %+v, want no patch evidence until a real patch is applied", runSnapshot.Patches)
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresGoalAndDoD(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-missing", "activation missing workspace", "", "workspaces/activation-missing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument when goal and DoD are missing", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresExplicitDefinitionWhenAmbiguous(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-ambiguous", "activation ambiguous workspace", "", "workspaces/activation-ambiguous")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	firstGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "first activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend first: %v", err)
	}
	if _, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "second activation goal"); err != nil {
		t.Fatalf("DefineGoalWithBackend second: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	if _, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict when goal selection is ambiguous", err)
	}
	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: firstGoal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend explicit: %v", err)
	}
	if item.Run.GoalID != firstGoal.ID || item.Run.DoDID != dod.ID || item.Run.State != "failed" {
		t.Fatalf("activation run = %+v, want explicit goal/dod failed through llm provider validation", item.Run)
	}
}

func TestActivateWorkspaceExecutionWithBackendReturnsActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-active", "activation active workspace", "", "workspaces/activation-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "active activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"active activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	activeRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, activeRunID, goal.ID, dod.ID); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, activeRunID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_decision" {
		t.Fatalf("started run = %+v, want blocked_on_decision", started)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != activeRunID || item.Run.State != "blocked_on_decision" {
		t.Fatalf("activation run = %+v, want existing active blocked run", item.Run)
	}
	if len(item.Snapshot.Runs) != 1 || item.Snapshot.Workspace.ActiveRunID != activeRunID {
		t.Fatalf("activation snapshot = %+v, want active run projection", item.Snapshot)
	}
}

func useUnavailableLLMProvider(t *testing.T) {
	t.Helper()
	t.Setenv(llmcommon.EnvProvider, "")
	t.Setenv(llmcommon.EnvBaseURL, "")
	t.Setenv(llmcommon.EnvAPIKey, "")
	t.Setenv(llmcommon.EnvModel, "")
	t.Setenv(llmcommon.EnvDisabled, "")
	t.Setenv(llmcommon.EnvTimeout, "")
}
