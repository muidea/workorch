package activationports

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSupersedeRunRequiresPersistedWorkspaceRelease(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		receipt orchestratorevents.RequestRuntimeTerminationResult
		wantErr error
	}{
		{
			name:    "release acknowledgement missing",
			receipt: orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: "delivery-1", WorkspaceReleased: false},
			wantErr: common.ErrConflict,
		},
		{
			name:    "release acknowledgement persisted",
			receipt: orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: "delivery-1", WorkspaceReleased: true, StateRevision: 9},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(event magicevent.Event, result magicevent.Result) {
				cmd, ok := event.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
				if !ok || cmd.WorkspaceID != "ws-replacement" || cmd.RunID != "run-old" || cmd.Kind != runtimecontract.RuntimeTerminationSuperseded {
					t.Fatalf("termination command = %+v", event.Data())
				}
				result.Set(tc.receipt, nil)
			})
			t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })

			err := SupersedeRun(ctx, hub, "ws-replacement", "run-old")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("SupersedeRun err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

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

func TestCreateAndStartWorkspaceExecutionRunSealsProviderTargetPrecedence(t *testing.T) {
	systemTarget := runtimecontract.ProviderModelTarget{ProviderID: "system-provider", ModelID: "system-model"}
	workspaceTarget := runtimecontract.ProviderModelTarget{ProviderID: "workspace-provider", ModelID: "workspace-model"}
	explicitTarget := runtimecontract.ProviderModelTarget{ProviderID: "explicit-provider", ModelID: "explicit-model"}
	tests := []struct {
		name      string
		explicit  *runtimecontract.ProviderModelTarget
		workspace *runtimecontract.ProviderModelTarget
		want      runtimecontract.ProviderModelTarget
	}{
		{name: "explicit overrides workspace and system", explicit: &explicitTarget, workspace: &workspaceTarget, want: explicitTarget},
		{name: "workspace overrides system", workspace: &workspaceTarget, want: workspaceTarget},
		{name: "system used when no override exists", want: systemTarget},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var explicit *runtimecontract.ProviderModelTarget
			if test.explicit != nil {
				value := *test.explicit
				explicit = &value
			}
			var workspace *runtimecontract.ProviderModelTarget
			if test.workspace != nil {
				value := *test.workspace
				workspace = &value
			}
			ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) {
				return workorchconfig.Config{LLM: workorchconfig.LLMConfig{DefaultChat: workorchconfig.ProviderModelTargetConfig{ProviderID: systemTarget.ProviderID, ModelID: systemTarget.ModelID}}}, nil
			})
			target, err := configuredActivationProviderTarget(ctx, explicit, workspace)
			if err != nil {
				t.Fatalf("configuredActivationProviderTarget: %v", err)
			}
			if !target.Same(test.want) {
				t.Fatalf("selected target = %+v, want %+v", target, test.want)
			}
			runtime := newOwnerRuntime()
			t.Cleanup(func() {
				if err := runtime.Shutdown(context.Background()); err != nil {
					t.Errorf("shutdown runtime: %v", err)
				}
			})
			exports := testRuntimeExports(runtime)
			workspaceID := common.WorkspaceID("ws-provider-precedence-" + test.name)
			goalID := common.GoalID("goal-provider-precedence")
			dodID := common.DoDID("dod-provider-precedence")
			seedActivationDefinitions(t, ctx, exports, workspaceID, goalID, dodID)
			bootstrap := activationRunTestBootstrap(goalID, dodID)
			bootstrap.Activation.ProviderTarget = target
			runID, err := CreateAndStartWorkspaceExecutionRun(ctx, exports.Infrastructure.CommandHub, CreateAndStartWorkspaceExecutionRunRequest{
				WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: bootstrap,
			})
			if err != nil {
				t.Fatalf("CreateAndStartWorkspaceExecutionRun: %v", err)
			}
			if explicit != nil {
				explicit.ModelID = "mutated-explicit"
			}
			if workspace != nil {
				workspace.ModelID = "mutated-workspace"
			}
			stored, err := exports.Repositories.Runs.Get(ctx, runID)
			if err != nil || stored.RuntimeBootstrap == nil || !stored.RuntimeBootstrap.Activation.ProviderTarget.Same(test.want) {
				t.Fatalf("persisted sealed target = %+v err=%v, want %+v", stored, err, test.want)
			}
		})
	}
}

func activationRunTestBootstrap(goalID common.GoalID, dodID common.DoDID) *runtimecontract.RuntimeBootstrap {
	bootstrap := &runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: "goal:" + string(goalID) + "|dod:" + string(dodID), GoalID: goalID, GoalDefinition: "goal",
			DoDID: dodID, DoDDefinition: "1. dod", CapabilityContract: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "activation-runports-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
			}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired, AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1}},
		},
		Focuses: []runtimecontract.CoverageFocus{{
			ID: "focus-001", SliceID: "slice-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"target.txt"},
			ValidationRecipeID: "activation-runports-validation",
		}},
		Slices:            []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-001"}},
		ConvergencePolicy: runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 1, MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1},
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/activation-runports/cwi-001", CoverageWorkItemID: "cwi-001", FocusID: "focus-001", DefinitionRevision: bootstrap.Definition.Revision,
		Proofs:     []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "activation-runports-validation"}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/activation-runports",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	bootstrap.Focuses[0].CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
	return bootstrap
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
