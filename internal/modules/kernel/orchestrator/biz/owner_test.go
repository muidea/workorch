package orchestrator

// Owner Biz lifecycle tests.

import (
	"context"
	"errors"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"

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
	firstSubscriptionCount := len(module.rawTopics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe orchestrator commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.rawTopics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.rawTopics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.runtimeController != nil || module.started || len(module.rawTopics) != 0 {
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
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: deps.CommandHub}); err != nil {
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

func TestScheduleRuntimeDriveReleasesKeyWhenSchedulingFails(t *testing.T) {
	background := &recordingBackgroundRoutine{err: errors.New("scheduler unavailable")}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	module := newTestBiz(newRuntimeTestOwner(), hub, background)
	for attempt := 0; attempt < 2; attempt++ {
		if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-retry", "run-drive-retry", "retry"); !errors.Is(err, background.err) {
			t.Fatalf("attempt %d err = %v, want scheduler error", attempt, err)
		}
	}
	if background.asyncFunctionCalls != 2 {
		t.Fatalf("background calls = %d, want scheduling retried after key release", background.asyncFunctionCalls)
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
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2", Delay: time.Hour, Source: "test",
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
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
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

func TestStopWaitsForQueuedRuntimeGovernanceResolutionHandlerBeforeClearingDependencies(t *testing.T) {
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
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID,
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: runtimecontract.RuntimeGovernanceResolution{
			ResolutionID: "resolution-lifecycle", TicketID: "ticket-lifecycle", WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle",
			AuthoritySnapshotID: "authority-lifecycle", DispatchID: "dispatch-lifecycle", DependencyKind: "decision", SubjectRef: "subject-lifecycle",
			Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "continue lifecycle test",
		}},
	)
	handled := make(chan struct{})
	go func() {
		module.handleRuntimeGovernanceResolution(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
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
		Evidence:   memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub: rawHub,
	}
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
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
