package run

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceExecutionRestartReplaysFromFilePersistenceAfterOwnerRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-file-replay")
	bootstrap := restartTestBootstrap(t, "ws-file-replay")
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-file-replay", RequestID: "restart-file-replay", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test",
		Bootstrap: &bootstrap,
	}

	startOwner := func(t *testing.T) (*Biz, *filerepo.RunRepository, magicevent.Hub) {
		t.Helper()
		hub := magicevent.NewHub(32)
		runs := filerepo.NewRunRepository(path)
		owner := New()
		if err := owner.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
			t.Fatalf("configure Run owner: %v", err)
		}
		if err := owner.Start(); err != nil {
			t.Fatalf("start Run owner: %v", err)
		}
		return owner, runs, hub
	}
	stopOwner := func(t *testing.T, owner *Biz, hub magicevent.Hub) {
		t.Helper()
		if err := owner.Stop(context.Background()); err != nil {
			t.Fatalf("stop Run owner: %v", err)
		}
		hub.Terminate(context.Background())
	}

	firstOwner, _, firstHub := startOwner(t)
	first, err := firstOwner.restartWorkspaceExecutionIntent(ctx, command)
	if err != nil || first.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || first.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
		t.Fatalf("first restart admission = %+v, err=%v", first, err)
	}
	stopOwner(t, firstOwner, firstHub)

	secondOwner, secondRuns, secondHub := startOwner(t)
	t.Cleanup(func() { stopOwner(t, secondOwner, secondHub) })
	duplicate, err := secondOwner.restartWorkspaceExecutionIntent(ctx, command)
	if err != nil || duplicate.RunID != first.RunID || duplicate.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart {
		t.Fatalf("persisted duplicate admission = %+v, first=%+v, err=%v", duplicate, first, err)
	}
	items, err := secondRuns.ListByWorkspaceID(ctx, command.WorkspaceID)
	if err != nil {
		t.Fatalf("list persisted Runs: %v", err)
	}
	if len(items) != 1 || items[0].ID != first.RunID || items[0].ActivationRequestID != command.RequestID {
		t.Fatalf("persisted Runs after replay = %+v, want exactly one activation", items)
	}

	conflicting := command
	changedBootstrap := command.Bootstrap.Clone()
	changedBootstrap.Activation.ProviderSelection.Target.ModelID = "different-model"
	conflicting.Bootstrap = &changedBootstrap
	if _, err := secondOwner.restartWorkspaceExecutionIntent(ctx, conflicting); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed payload after owner restart = %v, want ErrConflict", err)
	}
}

func TestWorkspaceExecutionRestartRejectsInvalidPersistedDuplicateIntent(t *testing.T) {
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-invalid-intent")
	bootstrap := restartTestBootstrap(t, "ws-invalid-intent")
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-invalid-intent", RequestID: "restart-invalid", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test",
		Bootstrap: &bootstrap,
	}
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: command.WorkspaceID, DefinitionRevision: "def-old", StartupInputHash: "startup-old",
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-invalid",
	}
	invalid := replacementIntentForPredecessor(replacementIntentFromCommand(command), predecessor)
	invalid.State = runmodel.ReplacementIntentClarificationResolutionBound
	invalid.ClarificationResolutionID = "resolution-without-binding"
	predecessor.ReplacementIntent = &invalid
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.restartWorkspaceExecutionIntent(ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("invalid persisted duplicate intent err=%v, want conflict", err)
	}
}

func TestWorkspaceExecutionRestartRequestIDBindsSealedPayload(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	bootstrap := restartTestBootstrap(t, "ws-1")
	existing := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test",
		RuntimeBootstrap: &bootstrap, ActivationRequestID: "restart-1", ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		State: common.RunStateActive,
	}
	if err := runs.Create(ctx, existing); err != nil {
		t.Fatal(err)
	}
	cmd := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RequestID: "restart-1", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	duplicate, err := biz.restartWorkspaceExecutionIntent(ctx, cmd)
	if err != nil || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart || duplicate.RunID != existing.ID {
		t.Fatalf("matching duplicate=%+v err=%v", duplicate, err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*runevents.WorkspaceExecutionIntentCommand)
	}{
		{name: "goal", mutate: func(value *runevents.WorkspaceExecutionIntentCommand) { value.GoalID = "goal-other" }},
		{name: "dod", mutate: func(value *runevents.WorkspaceExecutionIntentCommand) { value.DoDID = "dod-other" }},
		{name: "definition revision", mutate: func(value *runevents.WorkspaceExecutionIntentCommand) { value.DefinitionRevision = "def-other" }},
		{name: "bootstrap", mutate: func(value *runevents.WorkspaceExecutionIntentCommand) {
			changed := value.Bootstrap.Clone()
			changed.Activation.ProviderSelection.Target.ModelID = "different-model"
			value.Bootstrap = &changed
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cmd
			bootstrapCopy := cmd.Bootstrap.Clone()
			changed.Bootstrap = &bootstrapCopy
			test.mutate(&changed)
			if _, err := biz.restartWorkspaceExecutionIntent(ctx, changed); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("reused request with changed %s err=%v, want conflict", test.name, err)
			}
		})
	}
}

func TestWorkspaceExecutionDefinitionClarificationBindsPersistedIntentAndReplays(t *testing.T) {
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(16)
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-1", GoalID: "goal-old", DoDID: "dod-old", DefinitionRevision: "def-old",
		StartupInputHash: "startup-old", State: common.RunStateBlockedOnDecision,
		BlockingTicketID: "runtime-definition-clarification:ticket-1",
	}
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}

	resolutionCalls := 0
	observer := magicevent.NewSimpleObserver(decisionevents.CommandDestination, hub)
	observer.Subscribe(decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement, func(event magicevent.Event, result magicevent.Result) {
		resolutionCalls++
		cmd, ok := event.Data().(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		resolutionID := decisionevents.StableDefinitionClarificationResolutionID(cmd.TicketID, cmd.ReplacementIntentHash)
		result.Set(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{
			TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, PredecessorRunID: cmd.PredecessorRunID,
			ReplacementIntentID: cmd.ReplacementIntentID, ResolutionID: resolutionID,
			ResolutionType: decisionevents.DefinitionClarificationResolutionReplacementBound,
			Disposition:    decisionevents.DefinitionClarificationResolved, ConflictKind: decisionevents.RuntimeDefinitionClarificationNoConflict,
			ResolutionBindingHash: cmd.BindingHash(resolutionID, decisionevents.DefinitionClarificationResolved, decisionevents.RuntimeDefinitionClarificationNoConflict, "", ""),
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement) })

	bootstrap := restartTestBootstrap(t, "ws-1")
	cmd := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RequestID: "restart-1", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	admission, err := biz.restartWorkspaceExecutionIntent(ctx, cmd)
	if err != nil || admission.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending || admission.RunID != predecessor.ID {
		t.Fatalf("clarification restart admission=%+v err=%v", admission, err)
	}
	pending, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := biz.driveReplacementRecoveryGeneration(predecessor.WorkspaceID, predecessor.ID, pending.ReplacementIntent.Recovery.Generation); err != nil {
		t.Fatalf("drive clarification recovery: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementIntent == nil || stored.ReplacementIntent.State != runmodel.ReplacementIntentClarificationResolutionBound || stored.ReplacementIntent.ClarificationResolutionID == "" {
		t.Fatalf("clarification intent was not durably bound: %+v", stored.ReplacementIntent)
	}
	duplicate, err := biz.restartWorkspaceExecutionIntent(ctx, cmd)
	if err != nil || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart || duplicate.RunID != predecessor.ID {
		t.Fatalf("clarification duplicate=%+v err=%v", duplicate, err)
	}
	if resolutionCalls != 1 {
		t.Fatalf("bound duplicate submitted %d Decision resolutions, want 1", resolutionCalls)
	}
}

func TestRuntimeClarificationRejectsForgedConflictBindingWithoutStateAdvance(t *testing.T) {
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(16)
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	bootstrap := restartTestBootstrap(t, "ws-1")
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-1", DefinitionRevision: "def-old", StartupInputHash: "startup-old",
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-1",
	}
	requested := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RequestID: "restart-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	requested = replacementIntentForPredecessor(requested, predecessor)
	requested.State = runmodel.ReplacementIntentPendingClarificationResolution
	requested.Recovery = newReplacementRecoveryLease(requested, runmodel.ReplacementRecoveryClarificationResolution, 1, time.Now().UTC().Format(time.RFC3339Nano))
	predecessor.ReplacementIntent = &requested
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}

	exact := false
	observer := magicevent.NewSimpleObserver(decisionevents.CommandDestination, hub)
	observer.Subscribe(decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement, func(event magicevent.Event, result magicevent.Result) {
		cmd := event.Data().(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand)
		bindingHash := "forged"
		if exact {
			bindingHash = cmd.BindingHash("", decisionevents.DefinitionClarificationStale, decisionevents.RuntimeDefinitionClarificationScopeMismatch, "", "")
		}
		result.Set(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{
			TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, PredecessorRunID: cmd.PredecessorRunID,
			ReplacementIntentID: cmd.ReplacementIntentID, ResolutionType: decisionevents.DefinitionClarificationResolutionReplacementBound,
			Disposition: decisionevents.DefinitionClarificationStale, ConflictKind: decisionevents.RuntimeDefinitionClarificationScopeMismatch,
			ResolutionBindingHash: bindingHash,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement) })

	if err := biz.bindRuntimeClarificationReplacement(ctx, predecessor); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged conflict binding err=%v, want conflict", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil || stored.ReplacementIntent.State != runmodel.ReplacementIntentPendingClarificationResolution {
		t.Fatalf("forged binding advanced intent: %+v err=%v", stored.ReplacementIntent, err)
	}
	exact = true
	time.Sleep(replacementRecoveryInitialBackoff + 10*time.Millisecond)
	stored, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := biz.bindRuntimeClarificationReplacement(ctx, stored); err != nil {
		t.Fatalf("exact owner-adjudicated scope mismatch was not persisted: %v", err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil || stored.ReplacementIntent.State != runmodel.ReplacementIntentClarificationResolutionRejected {
		t.Fatalf("exact scope mismatch did not reject intent: %+v err=%v", stored.ReplacementIntent, err)
	}
}

func TestWorkspaceExecutionResumeRetainsCurrentRunOpaqueCWILedger(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	bootstrap := restartTestBootstrap(t, "ws-1")
	state := runtimecontract.RuntimeStateRecord{
		StateRevision: 17,
		// Run treats aggregate contents as opaque. This marker represents a
		// current-run canonical CWI ledger that only Orchestrator may interpret.
		ProcessedEventIDs: []string{"cwi-ledger/settled/cwi-1"},
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test",
		State: common.RunStateActive, RuntimeBootstrap: &bootstrap, RuntimeState: &state,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	continuations := make(chan runevents.RunContinuationRequestedData, 1)
	observer := magicevent.NewSimpleObserver(runevents.NotificationDestination, hub)
	observer.Subscribe(runevents.TopicRunContinuationRequested, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(runevents.RunContinuationRequestedData)
		if ok {
			continuations <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunContinuationRequested) })

	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RunID: "run-1", RequestID: "resume-current-run",
		Kind:   runtimecontract.RuntimeExecutionIntentResume,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
	}
	result := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent,
		workspaceExecutionIntentRunSource,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	admission, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok || admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested ||
		admission.RunID != run.ID || admission.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("resume admission = %+v", data)
	}

	select {
	case continuation := <-continuations:
		if continuation.WorkspaceID != run.WorkspaceID || continuation.RunID != run.ID {
			t.Fatalf("resume continuation switched Run scope: %+v", continuation)
		}
	case <-time.After(time.Second):
		t.Fatal("resume did not publish a current-run continuation fact")
	}

	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStateActive || stored.Revision != 1 || stored.RuntimeState == nil ||
		stored.RuntimeState.StateRevision != state.StateRevision ||
		len(stored.RuntimeState.ProcessedEventIDs) != 1 || stored.RuntimeState.ProcessedEventIDs[0] != state.ProcessedEventIDs[0] ||
		stored.ActivationRequestID != "" || stored.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		t.Fatalf("resume mutated current Run or opaque CWI ledger: %+v", stored)
	}
	items, err := runs.ListByWorkspaceID(ctx, run.WorkspaceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("resume created a replacement Run: items=%+v err=%v", items, err)
	}
	_ = biz
}
