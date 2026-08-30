package run

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type blockingNextWorkspaceRunRepository struct {
	repository.RunRepository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type fixedRunInventoryRepository struct {
	repository.RunRepository
	items []*runmodel.Run
}

func (r *fixedRunInventoryRepository) List(context.Context) ([]*runmodel.Run, error) {
	return runmodel.CloneRuns(r.items), nil
}

func (r *blockingNextWorkspaceRunRepository) CreateNextWorkspaceRun(ctx context.Context, item *runmodel.Run) error {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.RunRepository.CreateNextWorkspaceRun(ctx, item)
}

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
		if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
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
	if err != nil || duplicate.RunID != first.RunID || duplicate.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
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

func TestWorkspaceExecutionRestartContinuesLatestTerminalLineage(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-terminal-restart")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	runs := memory.NewRunRepository()
	predecessor := &runmodel.Run{
		WorkspaceID: workspaceID, GoalID: "goal-old", DoDID: "dod-old", State: common.RunStateCompleted,
		Lineage: runmodel.RunLineage{LineageState: runmodel.RunLineageStateActive},
		TerminalDelivery: &runtimecontract.RuntimeTerminalDeliveryIdentity{
			WorkspaceID: workspaceID, RunID: "pending-owner-id", TerminalDeliveryID: "terminal-completed",
		},
		TerminalResultID: "ws-terminal-restart/run-001/result/dod-assessment",
		RuntimeState:     &runtimecontract.RuntimeStateRecord{StateRevision: 9},
	}
	if err := runs.CreateNextWorkspaceRun(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	predecessor.TerminalDelivery.RunID = predecessor.ID
	if err := runs.Update(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	bootstrap := restartTestBootstrap(t, workspaceID)
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-terminal", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	biz := &Biz{runs: runs}
	admission, err := biz.restartWorkspaceExecutionIntent(ctx, command)
	if err != nil || admission.RunID != predecessor.ID || admission.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending || admission.Reason != runtimecontract.RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested {
		t.Fatalf("terminal restart admission=%+v err=%v", admission, err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessor.ID)
	if err != nil || stored.ReplacementIntent == nil || stored.ReplacementIntent.State != runmodel.ReplacementIntentTerminationAcknowledged || stored.ReplacementIntent.TerminalDeliveryID != predecessor.TerminalDelivery.TerminalDeliveryID || stored.ReplacementIntent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization {
		t.Fatalf("terminal predecessor replacement intent=%+v err=%v", stored, err)
	}
	generation := stored.ReplacementIntent.Recovery.Generation
	claimedAt := stored.ReplacementIntent.Recovery.DueAt
	if _, err := biz.submitReplacementInternalEvent(ctx, runs, replacementInternalEvent{
		Kind: replacementInternalEventRecoveryClaimed, WorkspaceID: workspaceID, RunID: predecessor.ID, OccurredAt: claimedAt,
		RecoveryClaimed: &replacementRecoveryClaimedFact{RequestID: command.RequestID, Generation: generation, ClaimedAt: claimedAt},
	}); err != nil {
		t.Fatalf("claim terminal successor materialization: %v", err)
	}
	successor, err := biz.materializeReplacementAfterTerminalDelivery(ctx, stored, predecessor.TerminalDelivery.TerminalDeliveryID, generation)
	if err != nil {
		t.Fatalf("materialize terminal successor: %v", err)
	}
	successor, err = runs.GetByWorkspaceID(ctx, workspaceID, successor.ID)
	if err != nil {
		t.Fatalf("reload terminal successor: %v", err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, workspaceID, predecessor.ID)
	if err != nil || stored.Lineage.LineageState != runmodel.RunLineageStateSuperseded || stored.Lineage.SuccessorRunID != successor.ID ||
		successor.Lineage.PredecessorRunID != predecessor.ID || successor.Lineage.PredecessorResultID != predecessor.TerminalResultID || successor.State != common.RunStateActive {
		t.Fatalf("terminal restart lineage predecessor=%+v successor=%+v err=%v", stored, successor, err)
	}
}

func TestConcurrentDifferentRestartsPersistAtMostOneCurrentRun(t *testing.T) {
	runs := memory.NewRunRepository()
	biz := &Biz{runs: runs}
	workspaceID := common.WorkspaceID("ws-concurrent-restart")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	commands := []runevents.WorkspaceExecutionIntentCommand{
		{WorkspaceID: workspaceID, RequestID: "restart-a", Kind: runtimecontract.RuntimeExecutionIntentRestart, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap},
		{WorkspaceID: workspaceID, RequestID: "restart-b", Kind: runtimecontract.RuntimeExecutionIntentRestart, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap},
	}
	errs := make(chan error, len(commands))
	var wg sync.WaitGroup
	for _, command := range commands {
		command := command
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := biz.restartWorkspaceExecutionIntent(ctx, command)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent restart: %v", err)
		}
	}
	items, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current := 0
	for _, item := range items {
		if !runStateIsTerminal(item.State) {
			current++
		}
	}
	if current != 1 || len(items) != 1 {
		t.Fatalf("concurrent restart persisted Runs=%+v, want one current Run", items)
	}
}

func TestWorkspaceExecutionRestartReplayActivatesDurablePendingCutWithoutNewRun(t *testing.T) {
	runs := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-pending-replay")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-pending-replay", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	biz := &Biz{runs: runs}
	created, err := biz.createWorkspaceExecutionReplacement(ctx, replacementIntentFromCommand(command), "")
	if err != nil || created.State != common.RunStatePending {
		t.Fatalf("durable create cut = %+v, err=%v", created, err)
	}
	generation := created.WorkspaceRunGeneration

	replayed, err := biz.restartWorkspaceExecutionIntent(ctx, command)
	if err != nil || replayed.RunID != created.ID || replayed.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || replayed.Reason != runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated {
		t.Fatalf("pending replay = %+v, err=%v", replayed, err)
	}
	items, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil || len(items) != 1 || items[0].State != common.RunStateActive || items[0].WorkspaceRunGeneration != generation {
		t.Fatalf("pending replay Runs = %+v, err=%v", items, err)
	}

}

func TestWorkspaceExecutionRestartRejectsDifferentRequestWhileActivationPending(t *testing.T) {
	runs := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-pending-conflict")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-original", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	biz := &Biz{runs: runs}
	created, err := biz.createWorkspaceExecutionReplacement(ctx, replacementIntentFromCommand(command), "")
	if err != nil || created.State != common.RunStatePending {
		t.Fatalf("durable create cut = %+v, err=%v", created, err)
	}
	different := command
	different.RequestID = "restart-different"
	if _, err := biz.restartWorkspaceExecutionIntent(ctx, different); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different request during pending activation err=%v, want conflict", err)
	}
	items, _ := runs.ListByWorkspaceID(ctx, workspaceID)
	if len(items) != 1 || items[0].State != common.RunStatePending || items[0].WorkspaceRunGeneration != created.WorkspaceRunGeneration {
		t.Fatalf("different request mutated pending activation: %+v", items)
	}
}

func TestRunConfigureRecoversPendingActivationBeforeRuntimeInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	workspaceID := common.WorkspaceID("ws-configure-recovery")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-configure-recovery", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	creatorRuns := filerepo.NewRunRepository(path)
	creator := &Biz{runs: creatorRuns}
	created, err := creator.createWorkspaceExecutionReplacement(ctx, replacementIntentFromCommand(command), "")
	if err != nil || created.State != common.RunStatePending {
		t.Fatalf("persist create-before-activation cut = %+v, err=%v", created, err)
	}

	hub := magicevent.NewHub(32)
	owner := New()
	if err := owner.Configure(runBizTestDependencies(filerepo.NewRunRepository(path), nil, nil, hub, nil)); err != nil {
		t.Fatalf("fresh owner Configure recovery: %v", err)
	}
	t.Cleanup(func() {
		_ = owner.Stop(context.Background())
		hub.Terminate(context.Background())
	})
	// Configure installs this read contract only after pending activation CAS.
	response := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeRecoveryInventory, "kernel/orchestrator", runevents.CommandDestination,
		magicevent.NewHeader(), context.Background(), runevents.QueryRuntimeRecoveryInventoryCommand{Limit: 10},
	))
	data, _ := response.Get()
	inventory, ok := data.(runevents.QueryRuntimeRecoveryInventoryResult)
	if !ok || len(inventory.Items) != 1 || inventory.Items[0].RunID != created.ID || inventory.Items[0].State != common.RunStateActive {
		t.Fatalf("post-recovery Setup inventory = %#v", data)
	}
	stored, err := filerepo.NewRunRepository(path).GetByWorkspaceID(ctx, workspaceID, created.ID)
	if err != nil || stored.State != common.RunStateActive || stored.WorkspaceRunGeneration != created.WorkspaceRunGeneration {
		t.Fatalf("recovered Run = %+v, err=%v", stored, err)
	}
	revision := stored.Revision

	// A second fresh owner observes the already-active record and performs no
	// additional transition or generation allocation.
	secondHub := magicevent.NewHub(16)
	second := New()
	if err := second.Configure(runBizTestDependencies(filerepo.NewRunRepository(path), nil, nil, secondHub, nil)); err != nil {
		t.Fatalf("second fresh owner Configure: %v", err)
	}
	if err := second.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	secondHub.Terminate(context.Background())
	items, err := filerepo.NewRunRepository(path).ListByWorkspaceID(ctx, workspaceID)
	if err != nil || len(items) != 1 || items[0].Revision != revision || items[0].WorkspaceRunGeneration != created.WorkspaceRunGeneration {
		t.Fatalf("duplicate startup recovery = %+v, err=%v", items, err)
	}
}

func TestRunConfigureValidatesAllPendingActivationsBeforeAnyCAS(t *testing.T) {
	runs := memory.NewRunRepository()
	createPending := func(workspaceID common.WorkspaceID, requestID string) *runmodel.Run {
		t.Helper()
		bootstrap := restartTestBootstrap(t, workspaceID)
		command := runevents.WorkspaceExecutionIntentCommand{
			WorkspaceID: workspaceID, RequestID: requestID, Kind: runtimecontract.RuntimeExecutionIntentRestart,
			GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
		}
		created, err := (&Biz{runs: runs}).createWorkspaceExecutionReplacement(common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID), replacementIntentFromCommand(command), "")
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	valid := createPending("ws-validation-a", "restart-valid")
	malformed := createPending("ws-validation-b", "restart-malformed")
	storedMalformed, err := runs.GetByWorkspaceID(context.Background(), malformed.WorkspaceID, malformed.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedMalformed.StartupInputHash = "corrupt-startup-input"
	if err := runs.CompareAndSwap(context.Background(), storedMalformed, storedMalformed.Revision); err != nil {
		t.Fatal(err)
	}

	owner := New()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Configure malformed inventory err=%v, want conflict", err)
	}
	unchanged, err := runs.GetByWorkspaceID(context.Background(), valid.WorkspaceID, valid.ID)
	if err != nil || unchanged.State != common.RunStatePending || unchanged.Revision != valid.Revision {
		t.Fatalf("valid earlier item was partially activated: %+v, err=%v", unchanged, err)
	}
}

func TestRunConfigureRejectsMultipleCurrentRunsBeforeAnyCAS(t *testing.T) {
	base := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-multiple-current")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-one", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	created, err := (&Biz{runs: base}).createWorkspaceExecutionReplacement(ctx, replacementIntentFromCommand(command), "")
	if err != nil {
		t.Fatal(err)
	}
	second := runmodel.CloneRun(created)
	second.ID = "run-corrupt-second-current"
	second.WorkspaceRunGeneration++
	second.ActivationRequestID = "restart-two"
	repo := &fixedRunInventoryRepository{RunRepository: base, items: []*runmodel.Run{created, second}}
	owner := New()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	if err := owner.Configure(runBizTestDependencies(repo, nil, nil, hub, nil)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Configure multiple current Runs err=%v, want conflict", err)
	}
	unchanged, err := base.GetByWorkspaceID(ctx, workspaceID, created.ID)
	if err != nil || unchanged.State != common.RunStatePending || unchanged.Revision != created.Revision {
		t.Fatalf("multiple-current validation partially activated Run: %+v, err=%v", unchanged, err)
	}
}

func TestReplacementMaterializationSerializesDifferentRestartAndKeepsOneCurrentRun(t *testing.T) {
	base := memory.NewRunRepository()
	blocking := &blockingNextWorkspaceRunRepository{RunRepository: base, entered: make(chan struct{}), release: make(chan struct{})}
	biz := &Biz{runs: blocking}
	workspaceID := common.WorkspaceID("ws-materialize-race")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	bootstrap := restartTestBootstrap(t, workspaceID)
	oldCommand := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: workspaceID, RequestID: "restart-old", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	intent := replacementIntentFromCommand(oldCommand)
	intent.State = runmodel.ReplacementIntentTerminationAcknowledged
	intent.TerminalDeliveryID = "terminal-old"
	intent.RuntimeStateRevision = 3
	intent.Recovery = newReplacementRecoveryLease(intent, runmodel.ReplacementRecoverySuccessorMaterialization, 1, "2026-08-25T00:00:00Z")
	intent.Recovery.State = runmodel.ReplacementRecoveryRunning
	intent.Recovery.DueAt = ""
	terminal := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: workspaceID, RunID: "historical-predecessor", TerminalDeliveryID: intent.TerminalDeliveryID}
	predecessor := &runmodel.Run{
		ID: "historical-predecessor", WorkspaceID: workspaceID, State: common.RunStateSuperseded,
		GoalID: "goal-old", DoDID: "dod-old", DefinitionRevision: "def-old", StartupInputHash: "startup-old",
		Lineage: runmodel.RunLineage{LineageState: "superseded"}, TerminalDelivery: &terminal,
		TerminalResultID: common.ResultID(string(workspaceID) + "/historical-predecessor/result/dod-assessment"), ReplacementIntent: &intent,
	}
	if err := base.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	materialized := make(chan error, 1)
	go func() {
		_, err := biz.materializeReplacementAfterTerminalDelivery(ctx, predecessor, terminal.TerminalDeliveryID)
		materialized <- err
	}()
	<-blocking.entered
	newCommand := oldCommand
	newCommand.RequestID = "restart-new"
	restarted := make(chan error, 1)
	go func() {
		_, err := biz.restartWorkspaceExecutionIntent(ctx, newCommand)
		restarted <- err
	}()
	close(blocking.release)
	if err := <-materialized; err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if err := <-restarted; err != nil {
		t.Fatalf("serialized restart: %v", err)
	}
	items, err := base.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current := 0
	for _, item := range items {
		if !runStateIsTerminal(item.State) {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("materialize/restart race persisted %d current Runs: %+v", current, items)
	}
}

func TestWorkspaceExecutionRestartRejectsInvalidPersistedDuplicateIntent(t *testing.T) {
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	biz := New()
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
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
	biz, runs, hub := newLifecycleTestBiz(t)
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
	continuations := make(chan runevents.RunContinuationRequestedData, 1)
	observer := magicevent.NewSimpleObserver(runevents.NotificationDestination, hub)
	observer.Subscribe(runevents.TopicRunContinuationRequested, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.RunContinuationRequestedData); ok {
			continuations <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunContinuationRequested) })
	duplicate, err := biz.restartWorkspaceExecutionIntent(ctx, cmd)
	if err != nil || duplicate.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted || duplicate.RunID != existing.ID {
		t.Fatalf("matching duplicate=%+v err=%v", duplicate, err)
	}
	select {
	case continuation := <-continuations:
		if continuation.WorkspaceID != existing.WorkspaceID || continuation.RunID != existing.ID || continuation.RequestID == "" {
			t.Fatalf("restart replay continuation = %+v", continuation)
		}
	case <-time.After(time.Second):
		t.Fatal("active restart replay did not republish a typed continuation fact")
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
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
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

func TestWorkspaceExecutionDefinitionClarificationRequiresRevisedSealedInput(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		definitionRevision string
		startupInputHash   string
	}{
		{name: "same definition and startup input", definitionRevision: "def-1", startupInputHash: "startup-input-test"},
		{name: "only definition revision changed", definitionRevision: "def-2", startupInputHash: "startup-input-test"},
		{name: "only startup input hash changed", definitionRevision: "def-1", startupInputHash: "startup-input-new"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runs := memory.NewRunRepository()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			biz := New()
			if err := biz.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
				t.Fatal(err)
			}
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-revision-required")
			predecessorBootstrap := restartTestBootstrapWithDefinition(t, "ws-revision-required", "def-1", "startup-input-test")
			predecessor := &runmodel.Run{
				ID: "run-001", WorkspaceID: "ws-revision-required", GoalID: "goal-1", DoDID: "dod-1",
				DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", RuntimeBootstrap: &predecessorBootstrap,
				State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-revision-required",
			}
			if err := runs.Create(ctx, predecessor); err != nil {
				t.Fatal(err)
			}
			before, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
			if err != nil {
				t.Fatal(err)
			}

			bootstrap := restartTestBootstrapWithDefinition(t, predecessor.WorkspaceID, testCase.definitionRevision, testCase.startupInputHash)
			command := runevents.WorkspaceExecutionIntentCommand{
				WorkspaceID: predecessor.WorkspaceID, RequestID: "restart-revision-required", Kind: runtimecontract.RuntimeExecutionIntentRestart,
				GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: testCase.definitionRevision,
				StartupInputHash: testCase.startupInputHash, Bootstrap: &bootstrap,
			}
			event := magicevent.NewEventWithContext(
				runevents.TopicRunCommandWorkspaceExecutionIntent,
				workspaceExecutionIntentRunSource,
				runevents.CommandDestination,
				magicevent.NewHeader(),
				ctx,
				command,
			)
			for attempt := 1; attempt <= 2; attempt++ {
				admission, err := biz.executeWorkspaceExecutionIntent(event)
				if err != nil {
					t.Fatalf("clarification restart attempt %d returned error: %v", attempt, err)
				}
				if admission.RunID != predecessor.ID ||
					admission.Outcome != runtimecontract.RuntimeExecutionIntentGovernanceRequired ||
					admission.Reason != runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending {
					t.Fatalf("clarification restart attempt %d admission = %+v", attempt, admission)
				}
			}

			after, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || after.ReplacementIntent != nil {
				t.Fatalf("rejected clarification restart mutated predecessor: before=%+v after=%+v", before, after)
			}
			items, err := runs.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].ID != predecessor.ID {
				t.Fatalf("rejected clarification restart created a successor: %+v", items)
			}
		})
	}
}

func TestRuntimeClarificationRejectsForgedConflictBindingWithoutStateAdvance(t *testing.T) {
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(16)
	biz := New()
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
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
