package activationports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateAndStartWorkspaceExecutionRunUsesEventHubOwnerCommands(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-activation-runports")
	goalID := common.GoalID("goal-activation-runports")
	dodID := common.DoDID("dod-activation-runports")
	seedActivationDefinitions(t, ctx, exports, workspaceID, goalID, dodID)

	runID, err := CreateAndStartWorkspaceExecutionRun(ctx, exports.Infrastructure.CommandHub, CreateAndStartWorkspaceExecutionRunRequest{
		WorkspaceID:           workspaceID,
		GoalID:                goalID,
		DoDID:                 dodID,
		ExecutionContext:      map[string]any{"dialogLanguage": "en-US"},
		Priority:              7,
		LLMProviderID:         "verify-provider",
		DefaultDialogLanguage: "en-US",
	})
	if err != nil {
		t.Fatalf("CreateAndStartWorkspaceExecutionRun: %v", err)
	}
	if runID != "run-001" {
		t.Fatalf("runID = %s, want run-001", runID)
	}
	item, err := exports.Repositories.Runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if item.WorkspaceID != workspaceID || item.GoalID != goalID || item.DoDID != dodID {
		t.Fatalf("run identity = %+v, want workspace/goal/dod", item)
	}
	if !activationcore.IsObservableRunState(item.State) {
		t.Fatalf("run state = %s, want observable started state", item.State)
	}
	if item.Priority != 7 {
		t.Fatalf("priority = %d, want 7", item.Priority)
	}
	compressed, ok := item.ExecutionContext["compressedContext"].(map[string]any)
	if !ok || compressed["runId"] != string(runID) || compressed["workspaceId"] != string(workspaceID) {
		t.Fatalf("compressedContext = %#v, want run identity", item.ExecutionContext["compressedContext"])
	}
}

func TestCreateAndStartWorkspaceExecutionRunContinuesWorkspaceRunSequence(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-activation-runports-sequence")
	seedActivationDefinitions(t, ctx, exports, workspaceID, "goal-activation-runports-sequence", "dod-activation-runports-sequence")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          "run-001",
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runID, err := CreateAndStartWorkspaceExecutionRun(ctx, exports.Infrastructure.CommandHub, CreateAndStartWorkspaceExecutionRunRequest{
		WorkspaceID:           workspaceID,
		GoalID:                "goal-activation-runports-sequence",
		DoDID:                 "dod-activation-runports-sequence",
		DefaultDialogLanguage: "en-US",
	})
	if err != nil {
		t.Fatalf("CreateAndStartWorkspaceExecutionRun: %v", err)
	}
	if runID != "run-002" {
		t.Fatalf("runID = %s, want run-002", runID)
	}
}

func TestRunObservableAfterStartConflictReadsRunWithContextWorkspaceScope(t *testing.T) {
	runtime := newOwnerRuntime()
	defer runtime.Shutdown(context.Background())
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-start-conflict-scope")
	runID := common.RunID("run-001")
	if err := exports.Repositories.Runs.Create(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}

	observable, err := RunObservableAfterStartConflict(common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID), exports.Infrastructure.CommandHub, runID)
	if err != nil {
		t.Fatalf("RunObservableAfterStartConflict err = %v", err)
	}
	if !observable {
		t.Fatalf("observable = false, want true")
	}
}

func TestBuildWorkspaceExecutionContextForIDsUsesEventHubInputs(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-activation-context-runports")
	goalID := common.GoalID("goal-activation-context-runports")
	dodID := common.DoDID("dod-activation-context-runports")
	seedActivationDefinitions(t, ctx, exports, workspaceID, goalID, dodID)

	contextData, err := BuildWorkspaceExecutionContextForIDs(ctx, WorkspaceExecutionContextByIDRequest{
		Hub:                   exports.Infrastructure.CommandHub,
		WorkspaceID:           workspaceID,
		GoalID:                goalID,
		DoDID:                 dodID,
		DefaultDialogLanguage: "en-US",
	})
	if err != nil {
		t.Fatalf("BuildWorkspaceExecutionContextForIDs: %v", err)
	}
	if contextData["workspaceId"] != string(workspaceID) || contextData["goalId"] != string(goalID) || contextData["doDId"] != string(dodID) {
		t.Fatalf("context identity = %#v, want workspace/goal/dod IDs", contextData)
	}
	if contextData["dialogLanguage"] != "en-US" {
		t.Fatalf("dialogLanguage = %#v, want en-US", contextData["dialogLanguage"])
	}
	if contextData["skillSelection"] == nil {
		t.Fatalf("skillSelection is nil, want evidence")
	}
}

func seedActivationDefinitions(t *testing.T, ctx context.Context, exports testkit.Exports, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) {
	t.Helper()
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "activation runports",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "activation goal",
	}); err != nil {
		t.Fatalf("seed goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"activation dod"},
		State:       common.DoDStateReady,
	}); err != nil {
		t.Fatalf("seed dod: %v", err)
	}
}
