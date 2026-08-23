package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceExecutionIntentFixture struct {
	biz        *Biz
	hub        magicevent.Hub
	workspaces *memoryrepo.WorkspaceRepository
	goals      *memoryrepo.GoalRepository
	dods       *memoryrepo.DoDRepository
	received   int
}

func newWorkspaceExecutionIntentFixture(t *testing.T, respond func(runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult) *workspaceExecutionIntentFixture {
	t.Helper()
	hub := magicevent.NewHub(16)
	fixture := &workspaceExecutionIntentFixture{
		hub: hub, workspaces: memoryrepo.NewWorkspaceRepository(), goals: memoryrepo.NewGoalRepository(), dods: memoryrepo.NewDoDRepository(),
	}
	biz, err := New(hub, nil, Dependencies{
		Workspaces: fixture.workspaces, Goals: fixture.goals, DoDs: fixture.dods,
		Templates: memoryrepo.NewWorkspaceTemplateRepository(), EventHub: hub,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.biz = biz
	observerID := "/workspace/consumers/execution-intent-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandWorkspaceExecutionIntent, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.WorkspaceExecutionIntentCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		fixture.received++
		result.Set(respond(command), nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandWorkspaceExecutionIntent)
		hub.Terminate(context.Background())
	})
	return fixture
}

func (f *workspaceExecutionIntentFixture) execute(ctx context.Context, command workspaceevents.WorkspaceExecutionIntentCommand) (workspaceevents.WorkspaceExecutionIntentResult, error) {
	return f.biz.executeExecutionIntent(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandExecutionIntent,
		workspaceExecutionIngressSource,
		workspaceevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
}

func (f *workspaceExecutionIntentFixture) seedWorkspace(t *testing.T, workspaceID common.WorkspaceID, activeRunID common.RunID, withDefinitions bool) runtimecontract.RuntimeBootstrap {
	t.Helper()
	goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: workspaceID, Text: "goal", DefinitionRevision: "goal:00000000000000000001:hash", LineageState: "active"}
	dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: workspaceID, Items: []string{"done"}, State: common.DoDStateReady, DefinitionRevision: "dod:00000000000000000001:hash", LineageState: "active"}
	definitionRevision := workspacecontracts.ExecutionDefinitionRevision(string(goal.ID), goal.DefinitionRevision, string(dod.ID), dod.DefinitionRevision)
	startup := (workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID: workspaceID, DefinitionRevision: definitionRevision,
		GoalDefinitionRef: string(goal.ID), GoalTextHash: "goal-hash", DoDDefinitionRefs: []string{string(dod.ID)}, DoDItemSetHash: "dod-hash",
		WorkspaceTemplateRef: "template-1", TemplateRevision: "template-r1", TemplateSnapshotHash: "template-hash",
		RuntimeProfileContractsHash:       runtimecontract.StableRuntimeProfileContractsHash(nil),
		ResolvedProjectWorkspaceDirectory: "/workspace/" + string(workspaceID),
		CandidateItemToItemRefMappings:    []workspacemodel.CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: string(goal.ID)}, {CandidateItemRef: "dod", ItemRef: string(dod.ID)}},
	}).WithHash()
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: workspaceID, CandidateID: "candidate-1", CandidateRevision: 1, CandidateHash: "candidate-hash",
		DefinitionRevision: definitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	workspace := &workspacemodel.Workspace{
		ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: activeRunID,
		DeliverableDirectory: startup.ResolvedProjectWorkspaceDirectory, StartupInput: &startup, ActivationAdmission: &admission,
	}
	if err := f.workspaces.Create(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if withDefinitions {
		scoped := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		if err := f.goals.Create(scoped, goal); err != nil {
			t.Fatal(err)
		}
		if err := f.dods.Create(scoped, dod); err != nil {
			t.Fatal(err)
		}
	}
	return workspaceExecutionIntentBootstrap(t, workspaceID, goal, dod, definitionRevision, startup.StartupInputHash)
}

func workspaceExecutionIntentBootstrap(t *testing.T, workspaceID common.WorkspaceID, goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision, startupInputHash string) runtimecontract.RuntimeBootstrap {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: startupInputHash, Revision: definitionRevision,
		GoalID: goal.ID, GoalDefinition: goal.Text, DoDID: dod.ID, DoDDefinition: dod.Items[0], CapabilityContract: []string{"file/read"},
	}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}}}
	coverageItems := []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: string(dod.ID), Order: 1}}
	slices := []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, coverageItems, slices)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, Definition: definition, Activation: activation, DefinitionSeed: seed,
		Focuses: []runtimecontract.CoverageFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequireAssessment: true}},
		Slices:  slices, CoverageItems: coverageItems,
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(), RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("bootstrap for %s: %v", workspaceID, err)
	}
	return bootstrap.Clone()
}

func TestWorkspaceExecutionIntentRejectsMissingDefinitionAndCrossScope(t *testing.T) {
	response := func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: "run-1", RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
		}}
	}

	t.Run("missing goal and DoD", func(t *testing.T) {
		fixture := newWorkspaceExecutionIntentFixture(t, response)
		bootstrap := fixture.seedWorkspace(t, "workspace-1", "", false)
		_, err := fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "workspace-1", RequestID: "restart-1", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap})
		if !errors.Is(err, common.ErrConflict) || fixture.received != 0 {
			t.Fatalf("missing definition err=%v received=%d", err, fixture.received)
		}
	})

	t.Run("context scope mismatch", func(t *testing.T) {
		fixture := newWorkspaceExecutionIntentFixture(t, response)
		bootstrap := fixture.seedWorkspace(t, "workspace-1", "", true)
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "workspace-other")
		_, err := fixture.execute(ctx, workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "workspace-1", RequestID: "restart-1", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap})
		if !errors.Is(err, common.ErrConflict) || fixture.received != 0 {
			t.Fatalf("cross-scope err=%v received=%d", err, fixture.received)
		}
	})
}

func TestWorkspaceExecutionIntentDuplicateReplayUsesActiveRunAndRejectsResultDrift(t *testing.T) {
	fixture := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}}
	})
	fixture.seedWorkspace(t, "workspace-1", "run-active", false)
	command := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "workspace-1", RequestID: "resume-1", Kind: runtimecontract.RuntimeExecutionIntentResume}
	first, err := fixture.execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.execute(context.Background(), command)
	if err != nil || first != second || first.RunID != "run-active" || fixture.received != 2 {
		t.Fatalf("duplicate first=%+v second=%+v received=%d err=%v", first, second, fixture.received, err)
	}

	drifted := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: "run-other", RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}}
	})
	drifted.seedWorkspace(t, "workspace-2", "run-active", false)
	_, err = drifted.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "workspace-2", RequestID: "resume-2", Kind: runtimecontract.RuntimeExecutionIntentResume})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted active Run result err=%v", err)
	}
}

func TestWorkspaceExecutionIntentDataResetDoesNotMutateSlotOrDefaults(t *testing.T) {
	fixture := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: "run-active", RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentDataResetRequired, Reason: runtimecontract.RuntimeExecutionIntentReasonDevelopmentDataIncompatible,
		}}
	})
	bootstrap := fixture.seedWorkspace(t, "workspace-1", "run-active", true)
	before, err := fixture.workspaces.Get(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "workspace-1", RequestID: "restart-1", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap})
	if err != nil || result.Outcome != runtimecontract.RuntimeExecutionIntentDataResetRequired {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	after, err := fixture.workspaces.Get(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if after.ActiveRunID != before.ActiveRunID || after.DefaultLLMSelection != nil || after.Revision != before.Revision {
		t.Fatalf("data-reset admission mutated workspace before=%+v after=%+v", before, after)
	}
}
