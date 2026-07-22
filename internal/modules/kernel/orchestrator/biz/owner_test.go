package orchestrator

// Owner Biz lifecycle tests.

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
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
				WorkspaceID: "ws-bootstrap-only",
				RunID:       "run-bootstrap-only",
				Class:       runevents.RuntimeV2RecoveryActiveNoDispatch,
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

func TestModuleStartRunCommandAcceptsStateAndSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-start")
	workspaceID := common.WorkspaceID("ws-module-background-start")
	runRepo := memoryrepo.NewRunRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := newTestBiz(orch, nil, bg)

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandStartRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		common.ContextWithRuntimeWorkspaceID(ctx, workspaceID),
		orchestratorevents.StartRunCommand{RunID: runID},
	))
	if err != nil {
		t.Fatalf("start command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled dispatch", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.State != common.RunStateActive {
		t.Fatalf("run state = %s, want running before background dispatch executes", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.State != common.WorkspaceStateActive || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want active workspace with active run", workspace)
	}
}

func TestModuleResumeRunCommandSchedulesRuntimeV2Drive(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume")
	workspaceID := common.WorkspaceID("ws-module-background-resume")
	runRepo := memoryrepo.NewRunRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := newTestBiz(orch, nil, bg)

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.ResumeRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		},
	))
	if err != nil {
		t.Fatalf("resume command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled resume", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestModuleResumeRunCommandRejectsTerminalRunBeforeBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume-terminal")
	workspaceID := common.WorkspaceID("ws-module-background-resume-terminal")
	runRepo := memoryrepo.NewRunRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := newTestBiz(orch, nil, bg)

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID},
	))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resume terminal err = %v, want conflict", err)
	}
	if bg.asyncFunctionCalls != 0 {
		t.Fatalf("background calls = %d, want terminal run rejected before scheduling", bg.asyncFunctionCalls)
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
