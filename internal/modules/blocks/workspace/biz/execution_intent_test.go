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
	biz, err := New(hub, nil, workspaceOwnerTestDependencies(
		hub, nil, fixture.workspaces, fixture.workspaces, fixture.goals, fixture.dods, memoryrepo.NewWorkspaceTemplateRepository(),
	))
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
	candidate := (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: workspaceID,
		CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                   workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision},
		DoDInputs:                   []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod", Text: dod.Items[0], SourceIdentity: "dod:" + string(dod.ID) + "@" + dod.DefinitionRevision}},
		WorkspaceTemplateSnapshot:   workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "template-1", Revision: "template-r1", SnapshotHash: "template-hash"},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil), AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent,
		ResolvedProjectWorkspaceDirectory: "/workspace/" + string(workspaceID),
		ProposedRelationInputs:            []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
	startup := (workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID: workspaceID, DefinitionRevision: definitionRevision,
		GoalDefinitionRef: string(goal.ID), GoalTextHash: workspacemodel.StableDefinitionTextHash(goal.Text), DoDDefinitionRefs: []string{string(dod.ID)}, DoDItemSetHash: workspacemodel.StableDefinitionStringSetHash(dod.Items),
		WorkspaceTemplateRef: "template-1", TemplateRevision: "template-r1", TemplateSnapshotHash: "template-hash",
		RuntimeProfileContractsHash:       runtimecontract.StableRuntimeProfileContractsHash(nil),
		ResolvedProjectWorkspaceDirectory: "/workspace/" + string(workspaceID),
		CandidateItemToItemRefMappings:    []workspacemodel.CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: string(goal.ID)}, {CandidateItemRef: "dod", ItemRef: string(dod.ID)}},
	}).WithHash()
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: workspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: definitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	workspace := &workspacemodel.Workspace{
		ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: activeRunID,
		DeliverableDirectory: startup.ResolvedProjectWorkspaceDirectory, StartupInput: &startup, ActivationAdmission: &admission,
		DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{candidate},
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
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
	}
	coverageItems := []runtimecontract.RuntimeCoverageItem{{
		ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: string(dod.ID), TargetText: dod.Items[0], Order: 1,
	}}
	slices := []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, coverageItems, slices)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: definition, Activation: activation, DefinitionSeed: seed,
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: string(dod.ID), AssessmentText: dod.Items[0],
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequireAssessment: true,
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", string(dod.ID)),
		}},
		Slices: slices, CoverageItems: coverageItems,
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(), RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
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
		stored, readErr := fixture.workspaces.Get(context.Background(), "workspace-1")
		if readErr != nil || stored.DefaultLLMSelection != nil {
			t.Fatalf("rejected admission changed Workspace default selection=%+v readErr=%v", stored.DefaultLLMSelection, readErr)
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

func TestWorkspaceExecutionIntentWaitingDeletionBlocksResumeAndRestartBeforeRunOwner(t *testing.T) {
	response := func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}}
	}
	fixture := newWorkspaceExecutionIntentFixture(t, response)
	workspaceID := common.WorkspaceID("workspace-deletion-waiting")
	bootstrap := fixture.seedWorkspace(t, workspaceID, "", true)
	current, err := fixture.workspaces.Get(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	waiting := workspaceWithWaitingDeletionForTest(t, current, "delete-waiting")
	if err := fixture.workspaces.CompareAndSwap(context.Background(), waiting, current.Revision); err != nil {
		t.Fatal(err)
	}
	for _, command := range []workspaceevents.WorkspaceExecutionIntentCommand{
		{WorkspaceID: workspaceID, RequestID: "resume-while-deleting", Kind: runtimecontract.RuntimeExecutionIntentResume},
		{WorkspaceID: workspaceID, RequestID: "restart-while-deleting", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap},
	} {
		if _, err := fixture.execute(context.Background(), command); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("kind=%s err=%v, want conflict", command.Kind, err)
		}
	}
	if fixture.received != 0 {
		t.Fatalf("Run owner received %d execution intents while deletion waits", fixture.received)
	}
}

func TestWorkspaceRestartMaterializesDefaultSelectionByPrivateEventBeforeRunOwner(t *testing.T) {
	var fixture *workspaceExecutionIntentFixture
	observedBeforeRun := false
	fixture = newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		stored, err := fixture.workspaces.Get(context.Background(), command.WorkspaceID)
		observedBeforeRun = err == nil && stored.DefaultLLMSelection != nil &&
			stored.DefaultLLMSelection.Target.ProviderID == "provider-1" && stored.DefaultLLMSelection.Target.ModelID == "model-1"
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: "run-new", RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
		}}
	})
	bootstrap := fixture.seedWorkspace(t, "workspace-default", "", true)
	bootstrap.Activation.ProviderSelection.ReasoningEffort = "high"
	result, err := fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "workspace-default", RequestID: "restart-default", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap,
	})
	if err != nil || result.RunID != "run-new" || !observedBeforeRun || fixture.received != 1 {
		t.Fatalf("restart result=%+v err=%v observedBeforeRun=%t received=%d", result, err, observedBeforeRun, fixture.received)
	}
	stored, err := fixture.workspaces.Get(context.Background(), "workspace-default")
	if err != nil || stored.DefaultLLMSelection == nil {
		t.Fatalf("stored Workspace=%+v err=%v", stored, err)
	}
	if stored.DefaultLLMSelection.ReasoningEffort != "" {
		t.Fatalf("Run-scoped reasoning effort leaked into Workspace default: %+v", stored.DefaultLLMSelection)
	}

	firstRevision := stored.Revision
	_, err = fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "workspace-default", RequestID: "restart-default-duplicate", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap,
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := fixture.workspaces.Get(context.Background(), "workspace-default")
	if err != nil || duplicate.Revision != firstRevision {
		t.Fatalf("duplicate materialization revision=%d want=%d err=%v", duplicate.Revision, firstRevision, err)
	}
}

func TestWorkspaceExecutionInternalEventRejectsStaleStartupWithoutMutation(t *testing.T) {
	fixture := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{}
	})
	bootstrap := fixture.seedWorkspace(t, "workspace-stale", "", true)
	before, err := fixture.workspaces.Get(context.Background(), "workspace-stale")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.biz.submitWorkspaceExecutionInternalEvent(context.Background(), workspaceExecutionInternalEvent{
		Kind:              workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID:       before.ID,
		StartupInputHash:  "stale-startup-input",
		ProviderSelection: bootstrap.Activation.ProviderSelection,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale startup event err=%v", err)
	}
	after, readErr := fixture.workspaces.Get(context.Background(), "workspace-stale")
	if readErr != nil || after.Revision != before.Revision || after.DefaultLLMSelection != nil {
		t.Fatalf("stale event mutated Workspace before=%+v after=%+v readErr=%v", before, after, readErr)
	}
}

func TestWorkspaceExecutionInternalEventRejectsStaleStartupWithExistingDefault(t *testing.T) {
	fixture := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{}
	})
	bootstrap := fixture.seedWorkspace(t, "workspace-stale-existing", "", true)
	current, err := fixture.workspaces.Get(context.Background(), "workspace-stale-existing")
	if err != nil {
		t.Fatal(err)
	}
	existing := runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "user-provider", ModelID: "user-model"}}
	current.DefaultLLMSelection = &existing
	if err := fixture.workspaces.Update(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	before, err := fixture.workspaces.Get(context.Background(), current.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.biz.submitWorkspaceExecutionInternalEvent(context.Background(), workspaceExecutionInternalEvent{
		Kind:              workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID:       before.ID,
		StartupInputHash:  "stale-startup-input",
		ProviderSelection: bootstrap.Activation.ProviderSelection,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale startup with existing default err=%v", err)
	}
	after, readErr := fixture.workspaces.Get(context.Background(), before.ID)
	if readErr != nil || after.Revision != before.Revision || after.DefaultLLMSelection == nil || !after.DefaultLLMSelection.Same(existing) || fixture.received != 0 {
		t.Fatalf("stale existing-default event mutated or reached Run: before=%+v after=%+v received=%d readErr=%v", before, after, fixture.received, readErr)
	}
}

func TestWorkspaceExecutionInternalEventPreservesExistingOwnerPreference(t *testing.T) {
	fixture := newWorkspaceExecutionIntentFixture(t, func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{}
	})
	bootstrap := fixture.seedWorkspace(t, "workspace-preference", "", true)
	current, err := fixture.workspaces.Get(context.Background(), "workspace-preference")
	if err != nil {
		t.Fatal(err)
	}
	existing := runtimecontract.ProviderInferenceSelection{
		Target: runtimecontract.ProviderModelTarget{ProviderID: "user-provider", ModelID: "user-model"}, ReasoningEffort: "low",
	}
	current.DefaultLLMSelection = &existing
	if err := fixture.workspaces.Update(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	before, err := fixture.workspaces.Get(context.Background(), "workspace-preference")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.biz.submitWorkspaceExecutionInternalEvent(context.Background(), workspaceExecutionInternalEvent{
		Kind:              workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID:       before.ID,
		StartupInputHash:  before.StartupInput.StartupInputHash,
		ProviderSelection: bootstrap.Activation.ProviderSelection,
	})
	if err != nil || stored.Revision != before.Revision || stored.DefaultLLMSelection == nil || !stored.DefaultLLMSelection.Same(existing) {
		t.Fatalf("existing preference was not preserved stored=%+v beforeRevision=%d err=%v", stored, before.Revision, err)
	}
}

func TestWorkspaceExecutionRestartRequiresCurrentAdmissionCandidateBeforeRunOwner(t *testing.T) {
	response := func(command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: command.WorkspaceID, RunID: "run-new", RequestID: command.RequestID, Kind: command.Kind,
			Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
		}}
	}

	t.Run("current exact candidate reaches Run owner", func(t *testing.T) {
		fixture := newWorkspaceExecutionIntentFixture(t, response)
		bootstrap := fixture.seedWorkspace(t, "workspace-current", "", true)
		result, err := fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{
			WorkspaceID: "workspace-current", RequestID: "restart-current", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap,
		})
		if err != nil || result.RunID != "run-new" || fixture.received != 1 {
			t.Fatalf("current restart result=%+v err=%v received=%d", result, err, fixture.received)
		}
	})

	t.Run("non-current persisted candidate is rejected without owner mutation", func(t *testing.T) {
		fixture := newWorkspaceExecutionIntentFixture(t, response)
		bootstrap := fixture.seedWorkspace(t, "workspace-invalid", "", true)
		workspace, err := fixture.workspaces.Get(context.Background(), "workspace-invalid")
		if err != nil {
			t.Fatal(err)
		}
		workspace.DefinitionAdmissionCandidates[0].SchemaVersion--
		if err := fixture.workspaces.Update(context.Background(), workspace); err != nil {
			t.Fatal(err)
		}
		before, err := fixture.workspaces.Get(context.Background(), "workspace-invalid")
		if err != nil {
			t.Fatal(err)
		}
		_, err = fixture.execute(context.Background(), workspaceevents.WorkspaceExecutionIntentCommand{
			WorkspaceID: "workspace-invalid", RequestID: "restart-invalid", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap,
		})
		after, readErr := fixture.workspaces.Get(context.Background(), "workspace-invalid")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !errors.Is(err, common.ErrConflict) || fixture.received != 0 || after.Revision != before.Revision {
			t.Fatalf("invalid restart err=%v received=%d revision=%d want=%d", err, fixture.received, after.Revision, before.Revision)
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
