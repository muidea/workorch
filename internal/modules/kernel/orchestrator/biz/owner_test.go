package orchestrator

// Owner Biz lifecycle tests.

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
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
	if module.bizPtr != nil {
		t.Fatal("new module biz pointer is non-nil before Configure")
	}

	deps := moduleContractDependencies()
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeV2RecoveryInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory)
		deps.CommandHub.Terminate(context.Background())
	})
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil {
		t.Fatal("biz pointer is nil after Configure")
	}
	if module.bizPtr.RuntimeV2RepositoryConfigured() == false {
		t.Fatal("Runtime V2 repository is not configured")
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
	if module.bizPtr != nil || module.started || len(module.rawTopics) != 0 {
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
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeV2RecoveryInventoryResult{Items: []runevents.RuntimeV2RecoveryInventoryItem{{
				WorkspaceID:         "ws-bootstrap-only",
				RunID:               "run-bootstrap-only",
				State:               common.RunStateActive,
				HasRuntimeBootstrap: true,
			}}}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory)
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

func TestGMMR2StartupUnsupportedSchemaMarksRunFreshActivationWithoutDrive(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	item := runevents.RuntimeV2RecoveryInventoryItem{
		WorkspaceID:               "ws-schema-unsupported",
		RunID:                     "run-schema-unsupported",
		State:                     common.RunStateActive,
		HasRuntimeBootstrap:       true,
		HasRuntimeState:           true,
		RuntimeStateRevision:      11,
		RuntimeStateSchemaVersion: runtimecontract.RuntimeStateSchemaVersionV2,
		RuntimeStateRef:           "runtime-state/ws-schema-unsupported/run-schema-unsupported/11",
	}
	var received *runevents.ApplyRuntimeStateIncompatibleFactCommand
	observer.Subscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeV2RecoveryInventoryResult{Items: []runevents.RuntimeV2RecoveryInventoryItem{item}}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.RunCommandResult{Run: &runmodel.Run{
				ID: item.RunID, WorkspaceID: item.WorkspaceID, State: common.RunStateActive,
				// The audit marker is intentionally blank: schema gate must still
				// create the Run-owned fresh-activation outcome.
				RuntimeState: &runtimecontract.RuntimeStateRecord{SchemaVersion: runtimecontract.RuntimeStateSchemaVersionV2, StateRevision: item.RuntimeStateRevision},
			}}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandApplyRuntimeStateIncompatible, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.ApplyRuntimeStateIncompatibleFactCommand)
		if !ok {
			if result != nil {
				result.Set(nil, nil)
			}
			return
		}
		copy := command
		received = &copy
		if result != nil {
			result.Set(runevents.RunLifecycleCommandResult{Transitioned: true, RuntimeActivationDisposition: runmodel.RuntimeActivationDispositionFreshActivationRequired}, nil)
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeV2RecoveryInventory)
		observer.Unsubscribe(runevents.TopicRunCommandGet)
		observer.Unsubscribe(runevents.TopicRunCommandApplyRuntimeStateIncompatible)
		deps.CommandHub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("unsupported schema scheduled %d background drives", background.asyncFunctionCalls)
	}
	if received == nil || received.WorkspaceID != item.WorkspaceID || received.RunID != item.RunID ||
		received.RuntimeStateRef != item.RuntimeStateRef || received.ActualSchemaVersion != runtimecontract.RuntimeStateSchemaVersionV2 ||
		received.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		received.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainDrive || received.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported ||
		received.Feedback.OriginalDiagnostic == nil {
		t.Fatalf("schema incompatibility fact=%+v", received)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := moduleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing tool calls", deps: func() Dependencies { d := base; d.ToolCalls = nil; return d }()},
		{name: "missing patches", deps: func() Dependencies { d := base; d.Patches = nil; return d }()},
		{name: "missing validation results", deps: func() Dependencies { d := base; d.ValidationResults = nil; return d }()},
		{name: "missing evidence", deps: func() Dependencies { d := base; d.EvidenceRecords = nil; return d }()},
		{name: "missing coverage", deps: func() Dependencies { d := base; d.CoverageWorkItems = nil; return d }()},
		{name: "missing artifact continuations", deps: func() Dependencies { d := base; d.ArtifactContinuations = nil; return d }()},
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

func TestScheduleRuntimeV2DriveCoalescesDuplicateRunRequests(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(&RuntimeOrchestrator{}, nil, background)
	if err := module.scheduleRuntimeV2Drive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "first"); err != nil {
		t.Fatal(err)
	}
	if err := module.scheduleRuntimeV2Drive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "duplicate"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("background calls = %d, want one coalesced drive", background.asyncFunctionCalls)
	}
	if !module.beginRuntimeV2Drive("ws-drive-singleflight\x00run-other") {
		t.Fatal("different run was unexpectedly coalesced")
	}
}

func TestScheduleRuntimeV2DriveReleasesKeyWhenSchedulingFails(t *testing.T) {
	background := &recordingBackgroundRoutine{err: errors.New("scheduler unavailable")}
	module := newTestBiz(&RuntimeOrchestrator{}, nil, background)
	for attempt := 0; attempt < 2; attempt++ {
		if err := module.scheduleRuntimeV2Drive(context.Background(), "ws-drive-retry", "run-drive-retry", "retry"); !errors.Is(err, background.err) {
			t.Fatalf("attempt %d err = %v, want scheduler error", attempt, err)
		}
	}
	if background.asyncFunctionCalls != 2 {
		t.Fatalf("background calls = %d, want scheduling retried after key release", background.asyncFunctionCalls)
	}
}

func TestScheduledRuntimeV2DriveReleasesSingleflightKeyWhenOwnerStopsBeforeTaskStarts(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(&RuntimeOrchestrator{}, nil, background)
	const driveKey = "ws-drive-stopped\x00run-drive-stopped"
	if err := module.scheduleRuntimeV2Drive(context.Background(), "ws-drive-stopped", "run-drive-stopped", "late-start"); err != nil {
		t.Fatal(err)
	}
	if background.lastAsyncFunction == nil {
		t.Fatal("expected queued Runtime V2 drive")
	}
	// Model the lifecycle state observed by a queued task after Stop has
	// acquired its write lock and cleared the owner authority.
	module.lifecycleMu.Lock()
	module.bizPtr = nil
	module.lifecycleMu.Unlock()
	background.lastAsyncFunction()

	module.driveMu.Lock()
	_, queued := module.drivingRuns[driveKey]
	module.driveMu.Unlock()
	if queued {
		t.Fatal("late-start drive retained its singleflight key after owner shutdown")
	}
}

func TestStopWaitsForRuntimeV2DriveLifecycleReadLockBeforeClearingDependencies(t *testing.T) {
	module := newTestBiz(&RuntimeOrchestrator{}, nil, nil)
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
		t.Fatalf("Stop returned before active Runtime V2 drive left lifecycle read lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil || module.evidenceRecords != nil {
		t.Fatalf("Stop retained owner dependencies: biz=%+v evidence=%+v", module.bizPtr, module.evidenceRecords)
	}
}

func TestStopWaitsForQueuedDecisionEvidenceHandlerBeforeClearingDependencies(t *testing.T) {
	base := memoryrepo.NewEvidenceRecordRepository()
	blocking := &blockingEvidenceRecordRepository{
		EvidenceRecordRepository: base,
		createEntered:            make(chan struct{}),
		releaseCreate:            make(chan struct{}),
	}
	module := newTestBiz(nil, nil, nil)
	module.evidenceRecords = blocking
	event := magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandRecordEvidence,
		"test",
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		decisionevents.RecordDecisionEvidenceCommand{
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
	if module.evidenceRecords != nil || module.bizPtr != nil {
		t.Fatalf("Stop retained owner dependencies after queued handler: evidence=%+v biz=%+v", module.evidenceRecords, module.bizPtr)
	}
	items, err := base.ListByRun(common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-lifecycle"), "run-lifecycle")
	if err != nil || len(items) != 1 || items[0].Kind != runtimecontract.EvidenceKindDecision {
		t.Fatalf("queued evidence persistence=%+v err=%v", items, err)
	}
}

func TestStopWaitsForQueuedRuntimeOperationReceiptHandlerBeforeClearingDependencies(t *testing.T) {
	repository := &blockingRuntimeV2StateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := &RuntimeOrchestrator{}
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
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
		module.handleRuntimeV2OperationReceipt(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
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
	if module.bizPtr != nil {
		t.Fatalf("Stop retained Runtime V2 owner after queued receipt handler: %+v", module.bizPtr)
	}
}

func TestStopWaitsForQueuedRuntimeGovernanceResolutionHandlerBeforeClearingDependencies(t *testing.T) {
	repository := &blockingRuntimeV2StateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := &RuntimeOrchestrator{}
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, nil, nil)
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		"test",
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
		module.handleRuntimeV2GovernanceResolution(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
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
	if module.bizPtr != nil {
		t.Fatalf("Stop retained Runtime V2 owner after queued governance handler: %+v", module.bizPtr)
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

type blockingRuntimeV2StateRepository struct {
	loadEntered chan struct{}
	releaseLoad chan struct{}
}

func (r *blockingRuntimeV2StateRepository) Load(context.Context, common.WorkspaceID, common.RunID) (runtimev2.RuntimeAggregate, error) {
	close(r.loadEntered)
	<-r.releaseLoad
	return runtimev2.RuntimeAggregate{}, common.ErrNotFound
}

func (*blockingRuntimeV2StateRepository) Save(context.Context, runtimev2.RuntimeAggregate, uint64) error {
	return nil
}

func moduleContractDependencies() Dependencies {
	rawHub := magicevent.NewHub(32)
	return Dependencies{
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
	}
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
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
