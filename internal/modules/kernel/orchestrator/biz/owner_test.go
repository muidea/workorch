package orchestrator

// Owner Biz lifecycle tests.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.runtimeController != nil {
		t.Fatal("new module runtime controller is non-nil before Configure")
	}

	deps := moduleContractDependencies()
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	recoveryObserver.Subscribe(runevents.TopicRunCommandWorkspaceRetirementInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementInventory)
		deps.CommandHub.Terminate(context.Background())
	})
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.runtimeController == nil {
		t.Fatal("runtime controller is nil after Configure")
	}
	if module.RuntimeRepositoryConfigured() == false {
		t.Fatal("Runtime repository is not configured")
	}
	if module.EventHub() == nil {
		t.Fatal("command hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.subscriptions)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe orchestrator commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.subscriptions) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.subscriptions), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.runtimeController != nil || module.started || len(module.subscriptions) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestGMMR1SM4OrchestratorStartFailsWithoutRecoveryInventory(t *testing.T) {
	deps := moduleContractDependencies()
	t.Cleanup(func() { deps.CommandHub.Terminate(context.Background()) })
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err == nil {
		t.Fatal("Start succeeded without the required Run recovery inventory subscription")
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop after failed Start: %v", err)
	}
}

func TestRuntimeDriveGateSettlementPumpRearmsAfterFrameworkTickRejection(t *testing.T) {
	background := &rejectRuntimeSettlementTicksBackground{rejection: errors.New("settlement tick rejected")}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(newRuntimeTestOwner(), hub, background)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	if err := module.openRuntimeLifecycle(lifecycleCtx); err != nil {
		t.Fatal(err)
	}
	if err := module.startRuntimeDriveGateSettlementPump(module.runtimeLifecycleContext()); err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws-settlement-wake-rejection")
	runID := common.RunID("run-settlement-wake-rejection")
	key := runtimeDriveKey(workspaceID, runID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("failed to acquire settlement wake gate")
	}
	gate, _ := module.currentRuntimeDriveGate(key)
	if !module.retainRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
		WorkspaceID: workspaceID, RunID: runID, DriveKey: key,
		GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "test_wake_rejection", ReleaseWhenNoLease: false,
	}) {
		t.Fatal("failed to register settlement wake")
	}
	deadline := time.Now().Add(3500 * time.Millisecond)
	for background.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	module.rollbackRuntimeStart()
	if calls := background.callCount(); calls < 2 {
		t.Fatalf("one rejected framework tick permanently stopped settlement pump: calls=%d", calls)
	}
}

func TestFailedStartRemovesAllIngressAndCancelsLifecycle(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	t.Cleanup(func() { deps.CommandHub.Terminate(context.Background()) })
	// Let Start pass the pre-subscription retirement recovery stage, then fail
	// on the required Runtime inventory after every ingress has been installed.
	// This exercises rollback rather than the earlier fail-fast path.
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandWorkspaceRetirementInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementInventory)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
	})
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.StartContext(context.Background()); err == nil {
		t.Fatal("Start succeeded without recovery inventory")
	}
	if module.started || len(module.subscriptions) == 0 || module.lifecycleCtx == nil || module.lifecycleCancel == nil {
		t.Fatalf("failed Start lost partial resources: started=%v topics=%d lifecycle=%v", module.started, len(module.subscriptions), module.lifecycleCtx)
	}
	module.driveMu.Lock()
	closing := module.runtimeLeaseTimerClosing
	module.driveMu.Unlock()
	if !closing || !errors.Is(module.runtimeLifecycleSchedulingError(), context.Canceled) {
		t.Fatalf("failed Start reopened queued callback scheduling: closing=%v err=%v", closing, module.runtimeLifecycleSchedulingError())
	}

	data := runevents.RunStateChangedData{
		WorkspaceID: "ws-failed-start", RunID: "run-failed-start", WorkspaceRunGeneration: 1, RunRevision: 1,
		State: common.RunStateActive,
	}
	data.CausationID = runevents.RunStateChangedCausationID(data.RunID, data.WorkspaceRunGeneration, data.RunRevision, 0)
	deps.CommandHub.Post(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), data,
	))
	time.Sleep(20 * time.Millisecond)
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("failed Start retained Run ingress: background calls=%d", background.asyncFunctionCalls)
	}
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	if err := module.StartContext(context.Background()); err == nil {
		t.Fatal("partial Start reopened resources")
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	module = New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.StartContext(context.Background()); err != nil {
		t.Fatalf("new owner Start after checked cleanup: %v", err)
	}
	if !module.started || len(module.subscriptions) == 0 {
		t.Fatal("Start retry did not restore lifecycle ingress")
	}
	module.driveMu.Lock()
	closing = module.runtimeLeaseTimerClosing
	module.driveMu.Unlock()
	if closing {
		t.Fatal("successful Start retry did not atomically reopen callback scheduling")
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop after recovered Start: %v", err)
	}
}

func TestGMMR1SM4StartupRecoverySchedulesBootstrapOnlyActiveRun(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{Items: []runevents.RuntimeRecoveryInventoryItem{{
				WorkspaceID:         "ws-bootstrap-only",
				RunID:               "run-bootstrap-only",
				State:               common.RunStateActive,
				HasRuntimeBootstrap: true,
			}}}, nil)
		}
	})
	recoveryObserver.Subscribe(runevents.TopicRunCommandWorkspaceRetirementInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		}
	})
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: true}, nil)
		}
	})
	recoveryObserver.Subscribe(runevents.TopicRunCommandWorkspaceRetirementFact, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementInventory)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementFact)
		deps.CommandHub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("startup recovery schedules = %d, want one bootstrap-only drive", background.asyncFunctionCalls)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestGMMR3StartupRecoveryWaitsForRunCommandsReady(t *testing.T) {
	deps := moduleContractDependencies()
	background := &startupReadyRecordingBackgroundRoutine{scheduled: make(chan struct{}, 1)}
	deps.BackgroundRoutine = background
	runs := memoryrepo.NewRunRepository()
	bootstrap := runtimecontract.RuntimeBootstrap{}
	if err := runs.Create(context.Background(), &runmodel.Run{
		ID: "run-startup-ready", WorkspaceID: "ws-startup-ready", State: common.RunStateActive, RuntimeBootstrap: &bootstrap,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, deps.CommandHub)); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = module.Stop(context.Background())
		_ = runOwner.Stop(context.Background())
		deps.CommandHub.Terminate(context.Background())
	})
	if err := module.Start(); err != nil {
		t.Fatalf("orchestrator Start: %v", err)
	}
	select {
	case <-background.scheduled:
		t.Fatal("startup drive was scheduled before Run commands were ready")
	default:
	}
	if err := runOwner.Start(); err != nil {
		t.Fatalf("run owner Start: %v", err)
	}
	select {
	case <-background.scheduled:
	case <-time.After(time.Second):
		t.Fatal("startup drive was not released by Run commands-ready fact")
	}
	for _, causationID := range []string{"run-owner-commands-ready", "run-owner-commands-ready-duplicate"} {
		deps.CommandHub.Post(magicevent.NewEventWithContext(
			runevents.TopicRunRuntimeCommandsReady,
			runevents.CommandDestination,
			runevents.NotificationDestination,
			magicevent.NewHeader(),
			context.Background(),
			runevents.RuntimeCommandsReadyData{CausationID: causationID},
		))
	}
	select {
	case <-background.scheduled:
		t.Fatal("duplicate Run commands-ready fact released startup recovery twice")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := moduleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing canonical evidence owner", deps: func() Dependencies {
			d := base
			d.Evidence = repository.OrchestratorEvidenceOwnerRepositories{}
			return d
		}()},
		{name: "missing command hub", deps: func() Dependencies { d := base; d.CommandHub = nil; return d }()},
		{name: "missing background routine", deps: func() Dependencies { d := base; d.BackgroundRoutine = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestScheduleRuntimeDriveCoalescesDuplicateRunRequests(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(newRuntimeTestOwner(), hub, background)
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "first"); err != nil {
		t.Fatal(err)
	}
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "duplicate"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("background calls = %d, want one coalesced drive", background.asyncFunctionCalls)
	}
	if !module.beginRuntimeDrive("ws-drive-singleflight\x00run-other") {
		t.Fatal("different run was unexpectedly coalesced")
	}
}

func TestScheduleRuntimeDriveRetainsGateAndDefersRejectedSubmissionToSettlementPump(t *testing.T) {
	background := &recordingBackgroundRoutine{err: errors.New("scheduler unavailable")}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	aggregate := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, background)
	if err := module.scheduleRuntimeDrive(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "retry"); !errors.Is(err, background.err) || !runtimeDriveScheduleSettlementWasRetained(err) {
		t.Fatalf("first scheduling err = %v, want retained scheduler error", err)
	}
	if err := module.scheduleRuntimeDrive(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "duplicate"); err != nil {
		t.Fatalf("duplicate scheduling should coalesce behind retained gate: %v", err)
	}
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if background.asyncFunctionCalls != 1 || module.beginRuntimeDrive(key) {
		t.Fatalf("rejected submission did not retain one gate: calls=%d", background.asyncFunctionCalls)
	}
	if epoch, token := module.runtimeDriveGateSettlementIdentity(key); epoch == 0 || token == 0 {
		t.Fatal("rejected submission retained no lifecycle settlement wakeup")
	}
	background.err = nil
	module.reconcilePendingRuntimeDriveGateSettlements()
	if background.asyncFunctionCalls != 2 || repository.aggregate.DriveLease == nil ||
		repository.aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		t.Fatalf("settlement pump did not persist and mount one successor: calls=%d lease=%+v", background.asyncFunctionCalls, repository.aggregate.DriveLease)
	}
}

func TestRuntimeDriveGateSettlementPumpSurvivesOneShotSubmissionRejection(t *testing.T) {
	aggregate := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &rejectFirstRuntimeOneShotBackground{
		rejection: errors.New("first one-shot submission rejected"), remaining: 1, acceptedSignal: make(chan struct{}, 1),
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(owner, hub, background)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	if err := module.openRuntimeLifecycle(lifecycleCtx); err != nil {
		t.Fatal(err)
	}
	if err := module.startRuntimeDriveGateSettlementPump(module.runtimeLifecycleContext()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		module.rollbackRuntimeStart()
	})

	err := module.scheduleRuntimeDrive(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "one_shot_reject")
	if !errors.Is(err, background.rejection) || !runtimeDriveScheduleSettlementWasRetained(err) {
		t.Fatalf("schedule err = %v, want retained one-shot rejection", err)
	}
	select {
	case <-background.acceptedSignal:
	case <-time.After(3 * time.Second):
		calls, _ := background.snapshot()
		t.Fatalf("lifecycle settlement pump did not retry after one-shot rejection: calls=%d lease=%+v", calls, repository.aggregate.DriveLease)
	}
	calls, callback := background.snapshot()
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	module.driveMu.Lock()
	registration, mounted := module.retryLeaseTimers[key]
	gate, gateHeld := module.drivingRuns[key]
	module.driveMu.Unlock()
	if calls != 2 || callback == nil || repository.aggregate.DriveLease == nil ||
		repository.aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || !mounted ||
		registration.Generation != repository.aggregate.DriveLease.RecoveryGeneration || !gateHeld || gate.Token == 0 {
		t.Fatalf("pump retry did not mount exact durable successor: calls=%d callback=%v lease=%+v registration=%+v mounted=%v gate=%+v/%v",
			calls, callback != nil, repository.aggregate.DriveLease, registration, mounted, gate, gateHeld)
	}
}

func TestPersistedRuntimeRecoveryLeaseCoalescesResumeAndTimerChains(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	module := newTestBiz(owner, hub, background)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("expected initial drive gate")
	}
	request := runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "persisted-pending/test", Delay: time.Hour, Source: "test",
	}
	if !module.scheduleRuntimeRecoveryLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, request) {
		t.Fatal("initial persisted recovery lease was not scheduled")
	}
	if !module.scheduleRuntimeRecoveryLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, request) {
		t.Fatal("duplicate persisted recovery lease was not recognized")
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("timer tasks = %d, want exactly one persisted lease timer", background.asyncFunctionCalls)
	}
	if err := module.scheduleRuntimeDrive(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "duplicate_resume"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("duplicate resume scheduled a parallel drive/timer: %d", background.asyncFunctionCalls)
	}
	if repository.aggregate.DriveLease == nil || repository.aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || repository.aggregate.DriveLease.RecoveryGeneration != 1 {
		t.Fatalf("persisted recovery lease = %+v", repository.aggregate.DriveLease)
	}
}

func TestSemanticDocumentPendingSchedulesOnePersistedLeaseWithoutBusyLoop(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	bootstrap := runtimeBootstrap()
	bootstrap.Activation.ExecutionContract.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 2}
	runs := memoryrepo.NewRunRepository()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-semantic-pending", WorkspaceID: "ws-semantic-pending",
		GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := bootstrap.InitialFacts("ws-semantic-pending", "run-semantic-pending")
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	prepared := initialized
	prepared.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection: runtimecontract.ProviderInferenceSelection{Target: prepared.Facts.Activation.ProviderSelection.Target}, ContextWindowTokens: 8192,
		ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048, MaxInputTokens: 4096, MaxBytes: 65536,
		CapabilityRevision: "semantic-capability-v1", ConfigurationRevision: "semantic-config-v1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "semantic-file-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	prepared.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	prepared.StateRevision++
	if err := repository.Save(ctx, prepared, initialized.StateRevision); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		outcome := llmevents.RuntimeProviderDocumentOutcome{
			Framing: runtimecontract.RuntimeProviderDocumentFraming{
				Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: command.ExpectedDocument.Hash,
				ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte(`{"proposals":[`)), ResponseBytes: int64(len(`{"proposals":[`)),
				Code: "max_output_tokens", Signature: "provider-output-limit",
			},
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash,
			DocumentOutcome:      &outcome,
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	if err := module.scheduleRuntimeDrive(ctx, "ws-semantic-pending", "run-semantic-pending", "semantic-pending"); err != nil {
		t.Fatal(err)
	}
	queuedDrive := background.lastAsyncFunction
	if queuedDrive == nil || background.asyncFunctionCalls != 1 {
		t.Fatalf("initial semantic drive scheduling calls=%d", background.asyncFunctionCalls)
	}
	queuedDrive()
	if requestCount != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("semantic pending requests=%d background calls=%d, want one provider request and one lease timer", requestCount, background.asyncFunctionCalls)
	}
	persisted, err := owner.loadRuntime(ctx, "ws-semantic-pending", "run-semantic-pending")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonPersistedPending ||
		persisted.Facts.Recovery != prepared.Facts.Recovery || len(persisted.CandidateAttempts) != 0 || persisted.ActiveDispatch != nil {
		t.Fatalf("semantic pending lease/authority=%+v", persisted)
	}
	if err := module.scheduleRuntimeDrive(ctx, "ws-semantic-pending", "run-semantic-pending", "duplicate-resume"); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("duplicate resume created a second semantic chain: requests=%d background=%d", requestCount, background.asyncFunctionCalls)
	}
}

func TestScheduleRuntimeDriveSeparatesSameRunIDAcrossWorkspaces(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(newRuntimeTestOwner(), hub, background)
	for _, workspaceID := range []common.WorkspaceID{"ws-drive-a", "ws-drive-b"} {
		if err := module.scheduleRuntimeDrive(context.Background(), workspaceID, "run-shared", "workspace-isolation"); err != nil {
			t.Fatal(err)
		}
		if err := module.scheduleRuntimeDrive(context.Background(), workspaceID, "run-shared", "duplicate"); err != nil {
			t.Fatal(err)
		}
	}
	if background.asyncFunctionCalls != 2 {
		t.Fatalf("workspace-scoped drives=%d, want two independent chains", background.asyncFunctionCalls)
	}
}

func TestScheduledRuntimeDriveReleasesSingleflightKeyWhenOwnerStopsBeforeTaskStarts(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(newRuntimeTestOwner(), hub, background)
	const driveKey = "ws-drive-stopped\x00run-drive-stopped"
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-stopped", "run-drive-stopped", "late-start"); err != nil {
		t.Fatal(err)
	}
	if background.lastAsyncFunction == nil {
		t.Fatal("expected queued Runtime drive")
	}
	// Model the lifecycle state observed by a queued task after Stop has
	// acquired its write lock and cleared the owner authority.
	module.lifecycleMu.Lock()
	module.runtimeController = nil
	module.lifecycleMu.Unlock()
	background.lastAsyncFunction()

	module.driveMu.Lock()
	_, queued := module.drivingRuns[driveKey]
	module.driveMu.Unlock()
	if queued {
		t.Fatal("late-start drive retained its singleflight key after owner shutdown")
	}
}

func TestStopWaitsForRuntimeDriveLifecycleReadLockBeforeClearingDependencies(t *testing.T) {
	module := newTestBiz(newRuntimeTestOwner(), nil, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		module.lifecycleMu.RLock()
		close(entered)
		<-release
		module.lifecycleMu.RUnlock()
	}()
	<-entered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before active Runtime drive left lifecycle read lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil || module.evidenceRecords != nil {
		t.Fatalf("Stop retained owner dependencies: controller=%+v evidence=%+v", module.runtimeController, module.evidenceRecords)
	}
}

func TestStopDeadlineCancelsLifecycleAndRetainsDependenciesUntilBlockingDriveExits(t *testing.T) {
	repository := &lifecycleBlockingRuntimeStateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(owner, hub, background)
	if err := module.openRuntimeLifecycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	module.started = true
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-stop-deadline", "run-stop-deadline", "blocking-drive"); err != nil {
		t.Fatal(err)
	}
	if background.lastAsyncFunction == nil {
		t.Fatal("blocking drive was not submitted")
	}
	workerDone := make(chan struct{})
	go func() {
		background.lastAsyncFunction()
		close(workerDone)
	}()
	<-repository.loadEntered

	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := module.Stop(stopCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop err=%v, want deadline exceeded", err)
	}
	if module.runtimeController == nil {
		t.Fatal("deadline Stop cleared dependencies while drive was in flight")
	}
	if module.lifecycleCtx == nil || !errors.Is(module.lifecycleCtx.Err(), context.Canceled) {
		t.Fatalf("deadline Stop did not cancel lifecycle root: %v", module.lifecycleCtx)
	}
	before := background.asyncFunctionCalls
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-stop-deadline", "run-stop-deadline", "after-stop"); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-Stop schedule err=%v, want context canceled", err)
	}
	if background.asyncFunctionCalls != before {
		t.Fatal("post-Stop schedule reached BackgroundRoutine")
	}

	close(repository.releaseLoad)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("blocking drive did not exit after repository release")
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop did not finish teardown: %v", err)
	}
	if module.runtimeController != nil || module.lifecycleCtx != nil {
		t.Fatalf("completed Stop retained lifecycle dependencies: controller=%v lifecycle=%v", module.runtimeController, module.lifecycleCtx)
	}
}

func TestStopWaitsForQueuedDecisionEvidenceHandlerBeforeClearingDependencies(t *testing.T) {
	hub := magicevent.NewHub(8)
	installEmptyRecoveryInventory(t, hub)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	base := memoryrepo.NewEvidenceRecordRepository()
	blocking := &blockingEvidenceRecordRepository{
		EvidenceRecordRepository: base,
		createEntered:            make(chan struct{}),
		releaseCreate:            make(chan struct{}),
	}
	module := newTestBiz(nil, hub, nil)
	module.evidenceRecords = blocking
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence,
		decisioncommon.ModuleID,
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.RecordDecisionEvidenceCommand{
			EvidenceRecordID: "evidence-lifecycle", WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle",
			TicketID: "ticket-lifecycle", ResolutionID: "resolution-lifecycle", ResolutionType: "continue",
			DecisionKind: "runtime_governance", Reason: "test lifecycle lock",
		},
	)
	handled := make(chan struct{})
	go func() {
		module.handleDecisionEvidenceCommand(event, nil)
		close(handled)
	}()
	<-blocking.createEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued evidence handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(blocking.releaseCreate)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.evidenceRecords != nil || module.runtimeController != nil {
		t.Fatalf("Stop retained owner dependencies after queued handler: evidence=%+v controller=%+v", module.evidenceRecords, module.runtimeController)
	}
	items, err := base.ListByRun(common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-lifecycle"), "run-lifecycle")
	if err != nil || len(items) != 1 || items[0].Kind != runtimecontract.EvidenceKindDecision {
		t.Fatalf("queued evidence persistence=%+v err=%v", items, err)
	}
}

func TestStopWaitsForQueuedRuntimeOperationReceiptHandlerBeforeClearingDependencies(t *testing.T) {
	repository := &blockingRuntimeStateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, nil, nil)
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
		"test",
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.QueryRuntimeOperationReceiptCommand{
			WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle", DispatchID: "dispatch-lifecycle",
			OperationID: "operation-lifecycle", ExecutionKey: "execution-lifecycle", Fence: 1,
		},
	)
	handled := make(chan struct{})
	go func() {
		module.handleRuntimeOperationReceipt(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
		close(handled)
	}()
	<-repository.loadEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued operation-receipt handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(repository.releaseLoad)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil {
		t.Fatalf("Stop retained Runtime controller after queued receipt handler: %+v", module.runtimeController)
	}
}

func TestStopWaitsForQueuedRuntimeGovernanceTicketHandlerBeforeClearingDependencies(t *testing.T) {
	hub := magicevent.NewHub(8)
	installEmptyRecoveryInventory(t, hub)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	repository := &blockingRuntimeStateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		decisioncommon.ModuleID,
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand{
			ResolutionID: "resolution-lifecycle", TicketID: "ticket-lifecycle", WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle",
			Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "continue lifecycle test",
		},
	)
	handled := make(chan struct{})
	go func() {
		module.handleRuntimeGovernanceTicketResolution(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
		close(handled)
	}()
	<-repository.loadEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued governance handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(repository.releaseLoad)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil {
		t.Fatalf("Stop retained Runtime controller after queued governance handler: %+v", module.runtimeController)
	}
}

type blockingEvidenceRecordRepository struct {
	repository.EvidenceRecordRepository
	createEntered chan struct{}
	releaseCreate chan struct{}
}

func (r *blockingEvidenceRecordRepository) Create(ctx context.Context, item *runtimecontract.EvidenceRecord) error {
	close(r.createEntered)
	<-r.releaseCreate
	return r.EvidenceRecordRepository.Create(ctx, item)
}

type blockingRuntimeStateRepository struct {
	loadEntered chan struct{}
	releaseLoad chan struct{}
}

type lifecycleBlockingRuntimeStateRepository struct {
	loadOnce    sync.Once
	loadEntered chan struct{}
	releaseLoad chan struct{}
}

func (r *lifecycleBlockingRuntimeStateRepository) Load(context.Context, common.WorkspaceID, common.RunID) (runtime.RuntimeAggregate, error) {
	r.loadOnce.Do(func() { close(r.loadEntered) })
	<-r.releaseLoad
	return runtime.RuntimeAggregate{}, common.ErrNotFound
}

func (*lifecycleBlockingRuntimeStateRepository) Save(context.Context, runtime.RuntimeAggregate, uint64) error {
	return nil
}

func (r *blockingRuntimeStateRepository) Load(context.Context, common.WorkspaceID, common.RunID) (runtime.RuntimeAggregate, error) {
	close(r.loadEntered)
	<-r.releaseLoad
	return runtime.RuntimeAggregate{}, common.ErrNotFound
}

func (*blockingRuntimeStateRepository) Save(context.Context, runtime.RuntimeAggregate, uint64) error {
	return nil
}

func moduleContractDependencies() Dependencies {
	rawHub := magicevent.NewHub(32)
	return Dependencies{
		Evidence:          memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub:        rawHub,
		BackgroundRoutine: &recordingBackgroundRoutine{},
	}
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
	executeAsync       bool
}

type rejectFirstRuntimeOneShotBackground struct {
	recordingBackgroundRoutine
	mu             sync.Mutex
	rejection      error
	remaining      int
	oneShotCalls   int
	acceptedSignal chan struct{}
}

type rejectRuntimeSettlementTicksBackground struct {
	recordingBackgroundRoutine
	mu        sync.Mutex
	rejection error
	calls     int
}

func (b *rejectRuntimeSettlementTicksBackground) AsyncTask(task magictask.Task) error {
	if _, ok := task.(*runtimeDriveGateSettlementTask); ok {
		b.mu.Lock()
		b.calls++
		b.mu.Unlock()
		return b.rejection
	}
	return b.recordingBackgroundRoutine.AsyncTask(task)
}

func (b *rejectRuntimeSettlementTicksBackground) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func (b *rejectFirstRuntimeOneShotBackground) AsyncFunction(function func()) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.oneShotCalls++
	if b.remaining > 0 {
		b.remaining--
		return b.rejection
	}
	b.lastAsyncFunction = function
	select {
	case b.acceptedSignal <- struct{}{}:
	default:
	}
	return nil
}

func (b *rejectFirstRuntimeOneShotBackground) snapshot() (int, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.oneShotCalls, b.lastAsyncFunction
}

// startupReadyRecordingBackgroundRoutine observes scheduling without running
// the drive. It makes the startup handoff test independent from capability
// owner setup and avoids a test-local second recovery path.
type startupReadyRecordingBackgroundRoutine struct {
	recordingBackgroundRoutine
	scheduled chan struct{}
}

func (r *startupReadyRecordingBackgroundRoutine) AsyncFunction(function func()) error {
	if function == nil {
		return errors.New("function is nil")
	}
	select {
	case r.scheduled <- struct{}{}:
	default:
	}
	return nil
}

func (r *recordingBackgroundRoutine) AsyncTask(task magictask.Task) error {
	if _, ok := task.(*runtimeDriveGateSettlementTask); ok {
		if r.err != nil {
			return r.err
		}
		go task.Run()
		return nil
	}
	return r.AsyncFunction(task.Run)
}

func (r *recordingBackgroundRoutine) SyncTask(task magictask.Task) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) SyncTaskWithTimeOut(task magictask.Task, _ time.Duration) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) AsyncFunction(function func()) error {
	r.asyncFunctionCalls++
	r.lastAsyncFunction = function
	if r.err == nil && r.executeAsync && function != nil {
		function()
	}
	return r.err
}

func (r *recordingBackgroundRoutine) SyncFunction(function func()) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunctionWithTimeOut(function func(), _ time.Duration) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) Timer(context.Context, magictask.Task, time.Duration, time.Duration) error {
	return nil
}

func (r *recordingBackgroundRoutine) Shutdown(context.Context) bool {
	return true
}

func (b *rejectRuntimeSettlementTicksBackground) AsyncTaskContext(ctx context.Context, task magictask.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.AsyncTask(task)
}

func (r *recordingBackgroundRoutine) AsyncTaskContext(ctx context.Context, task magictask.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.AsyncTask(task)
}
