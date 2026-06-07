package activationports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateAndStartWorkspaceExecutionRunUsesEventHubOwnerCommands(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-activation-runports")
	goalID := common.GoalID("goal-activation-runports")
	dodID := common.DoDID("dod-activation-runports")
	seedActivationDefinitions(t, ctx, exports, workspaceID, goalID, dodID)

	runID, err := CreateAndStartWorkspaceExecutionRun(ctx, exports.Infrastructure.EventHub, CreateAndStartWorkspaceExecutionRunRequest{
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
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-activation-runports-sequence")
	seedActivationDefinitions(t, ctx, exports, workspaceID, "goal-activation-runports-sequence", "dod-activation-runports-sequence")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          "run-001",
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runID, err := CreateAndStartWorkspaceExecutionRun(ctx, exports.Infrastructure.EventHub, CreateAndStartWorkspaceExecutionRunRequest{
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

func TestBuildWorkspaceExecutionContextForIDsUsesEventHubInputs(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-activation-context-runports")
	goalID := common.GoalID("goal-activation-context-runports")
	dodID := common.DoDID("dod-activation-context-runports")
	seedActivationDefinitions(t, ctx, exports, workspaceID, goalID, dodID)

	contextData, err := BuildWorkspaceExecutionContextForIDs(ctx, WorkspaceExecutionContextByIDRequest{
		Hub:                   exports.Infrastructure.EventHub,
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

func seedActivationDefinitions(t *testing.T, ctx context.Context, exports bootstrap.Exports, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) {
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
