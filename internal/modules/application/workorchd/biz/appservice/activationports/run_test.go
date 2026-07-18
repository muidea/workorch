package activationports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		DefinitionRevision: "goal:" + string(goalID) + "|dod:" + string(dodID),
		Priority:           7,
		RuntimeBootstrap:   activationRunTestBootstrap(goalID, dodID),
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
	if item.RuntimeBootstrap == nil || item.RuntimeBootstrap.Definition.Revision != "goal:"+string(goalID)+"|dod:"+string(dodID) {
		t.Fatalf("run runtime bootstrap = %#v, want typed bootstrap", item.RuntimeBootstrap)
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
		WorkspaceID:        workspaceID,
		GoalID:             "goal-activation-runports-sequence",
		DoDID:              "dod-activation-runports-sequence",
		DefinitionRevision: "goal:goal-activation-runports-sequence|dod:dod-activation-runports-sequence",
		RuntimeBootstrap:   activationRunTestBootstrap("goal-activation-runports-sequence", "dod-activation-runports-sequence"),
	})
	if err != nil {
		t.Fatalf("CreateAndStartWorkspaceExecutionRun: %v", err)
	}
	if runID != "run-002" {
		t.Fatalf("runID = %s, want run-002", runID)
	}
}

func activationRunTestBootstrap(goalID common.GoalID, dodID common.DoDID) *runtimecontract.RuntimeBootstrap {
	return &runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: "goal:" + string(goalID) + "|dod:" + string(dodID), GoalID: goalID, GoalDefinition: "goal",
			DoDID: dodID, DoDDefinition: "1. dod", CapabilityContract: []string{runtimecontract.StepKindShellCommand},
		},
		Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired, AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1}}},
		Focuses: []runtimecontract.CoverageFocus{{
			ID: "focus-001", SliceID: "slice-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		}},
		Slices:            []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-001"}},
		ConvergencePolicy: runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 1, MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1},
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

func TestBuildWorkspaceRuntimeActivationForIDsUsesEventHubInputs(t *testing.T) {
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

	build, err := BuildWorkspaceRuntimeActivationForIDs(ctx, WorkspaceRuntimeActivationByIDRequest{
		Hub:                   exports.Infrastructure.CommandHub,
		WorkspaceID:           workspaceID,
		GoalID:                goalID,
		DoDID:                 dodID,
		DefaultDialogLanguage: "en-US",
	})
	if err != nil {
		t.Fatalf("BuildWorkspaceRuntimeActivationForIDs: %v", err)
	}
	if build.EffectiveGoal == nil || build.EffectiveDoD == nil || build.EffectiveGoal.ID != goalID || build.EffectiveDoD.ID != dodID {
		t.Fatalf("runtime activation definitions = %#v/%#v, want goal/dod IDs", build.EffectiveGoal, build.EffectiveDoD)
	}
	if build.RuntimeActivation.DialogLanguage != "en-US" {
		t.Fatalf("dialogLanguage = %#v, want en-US", build.RuntimeActivation.DialogLanguage)
	}
	if build.DefinitionRevision != "goal:"+string(goalID)+"|dod:"+string(dodID) {
		t.Fatalf("definitionRevision = %q, want typed definition revision", build.DefinitionRevision)
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
