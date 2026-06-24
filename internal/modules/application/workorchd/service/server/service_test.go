package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type recordingBackgroundRoutine struct {
	asyncFunctionCalled bool
	asyncFunctionCalls  int
	timerCalls          int
	function            func()
	timerTask           magictask.Task
	err                 error
}

func (r *recordingBackgroundRoutine) AsyncTask(task magictask.Task) error {
	return nil
}

func (r *recordingBackgroundRoutine) SyncTask(task magictask.Task) error {
	return nil
}

func (r *recordingBackgroundRoutine) SyncTaskWithTimeOut(task magictask.Task, timeout time.Duration) error {
	return nil
}

func (r *recordingBackgroundRoutine) AsyncFunction(function func()) error {
	r.asyncFunctionCalled = true
	r.asyncFunctionCalls++
	r.function = function
	return r.err
}

func (r *recordingBackgroundRoutine) SyncFunction(function func()) error {
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunctionWithTimeOut(function func(), timeout time.Duration) error {
	return nil
}

func (r *recordingBackgroundRoutine) Timer(ctx context.Context, task magictask.Task, intervalValue time.Duration, offsetValue time.Duration) error {
	r.timerCalls++
	r.timerTask = task
	return nil
}

func (r *recordingBackgroundRoutine) Shutdown(ctx context.Context) bool {
	return true
}

func TestServiceStartupRequiresFrameworkRuntime(t *testing.T) {
	svc := NewService(Config{Addr: "127.0.0.1:0", Backend: "memory"})
	routine := &recordingBackgroundRoutine{}

	if err := svc.Startup(context.Background(), "workorch", nil, routine); err == nil {
		t.Fatal("Startup returned nil error without event hub")
	}
	if err := svc.Startup(context.Background(), "workorch", event.NewHub(8), nil); err == nil {
		t.Fatal("Startup returned nil error without background routine")
	}
}

func TestServiceRunUsesBackgroundRoutine(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http:              NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, nil, routine),
		backgroundRoutine: routine,
		done:              make(chan error, 1),
	}

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !routine.asyncFunctionCalled {
		t.Fatal("Run did not submit HTTP server through BackgroundRoutine.AsyncFunction")
	}
	if routine.asyncFunctionCalls != 2 {
		t.Fatalf("Run submitted %d background functions, want projection baseline and HTTP server", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 1 {
		t.Fatalf("Run submitted %d timers, want stale dispatched watchdog timer", routine.timerCalls)
	}
	if routine.function == nil {
		t.Fatal("Run submitted a nil background function")
	}
	if !svc.runStarted.Load() {
		t.Fatal("Run did not mark service as started after background submission")
	}
	select {
	case err := <-svc.done:
		t.Fatalf("background function executed unexpectedly, err=%v", err)
	default:
	}
}

func TestServiceRunRecoversStartupInactiveRunningWorkspaceBeforeProjection(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	const workspaceID = "ws-service-startup-inactive"
	const runID = "run-service-startup-inactive"
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "service startup inactive",
		DeliverableDirectory: "workspaces/service-startup-inactive",
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http:              NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "file", DataRoot: dataRoot}, nil, routine),
		backgroundRoutine: routine,
		done:              make(chan error, 1),
	}
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if routine.asyncFunctionCalls != 2 {
		t.Fatalf("Run submitted %d background functions, want projection baseline and HTTP server only", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 1 {
		t.Fatalf("Run submitted %d timers, want watchdog timer only", routine.timerCalls)
	}

	workspace, err := appservice.ReadWorkspaceWithBackend(ctx, "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if workspace.State != string(common.WorkspaceStateActive) || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want active without active run", workspace)
	}
	run, err := appservice.ReadRunWithBackend(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunWithBackend: %v", err)
	}
	if run.State != string(common.RunStateCancelled) {
		t.Fatalf("run = %+v, want cancelled startup inactive run", run)
	}
	if err := svc.http.state.workspaceProjection.RebuildWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("RebuildWorkspace: %v", err)
	}
	snapshot, err := svc.ReadWorkspaceViewSnapshot(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceViewSnapshot: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) || snapshot.Workspace.ActiveRunID != "" {
		t.Fatalf("projected workspace = %+v, want active without active run", snapshot.Workspace)
	}
	if snapshot.TaskProgress.Recovery.Mode != "idle" || snapshot.TaskProgress.Recovery.NextAction != "start_workspace_execution" {
		t.Fatalf("projected recovery = %+v, want idle start action", snapshot.TaskProgress.Recovery)
	}
	diagnostics := svc.http.state.schedulerDiagnostics.snapshot(workspaceID, runID)
	if diagnostics == nil || diagnostics.LatestContinuation == nil {
		t.Fatalf("scheduler diagnostics = %+v, want restart not scheduled diagnostic", diagnostics)
	}
	latest := diagnostics.LatestContinuation
	if latest.State != schedulerDiagnosticStateNotScheduled ||
		latest.Reason != schedulerReasonServiceRestartedPendingWorkNotRecovered ||
		latest.Recoverability != schedulerRecoverabilityManualRetry {
		t.Fatalf("latest diagnostic = %+v, want service restart manual retry diagnostic", latest)
	}
	if len(latest.RecommendedActions) != 2 || latest.RecommendedActions[0] != "run_resume" || latest.RecommendedActions[1] != "workspace_restart" {
		t.Fatalf("recommended actions = %+v, want run_resume/workspace_restart", latest.RecommendedActions)
	}
}

func TestServiceRunRequiresBackgroundRoutine(t *testing.T) {
	svc := &Service{
		http: NewHTTPServer(Config{Addr: "127.0.0.1:0", Backend: "memory"}),
		done: make(chan error, 1),
	}

	if err := svc.Run(context.Background()); err == nil {
		t.Fatal("Run returned nil error without background routine")
	}
	if svc.runStarted.Load() {
		t.Fatal("Run left service marked as started without background routine")
	}
}

func TestServiceRunReturnsErrorWhenBackgroundSubmitFails(t *testing.T) {
	submitErr := errors.New("background routine is closed")
	routine := &recordingBackgroundRoutine{err: submitErr}
	svc := &Service{
		http:              NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, nil, routine),
		backgroundRoutine: routine,
		done:              make(chan error, 1),
	}

	if err := svc.Run(context.Background()); err == nil {
		t.Fatal("Run returned nil error after background submission failure")
	}
	if !routine.asyncFunctionCalled {
		t.Fatal("Run did not attempt to submit HTTP server through BackgroundRoutine.AsyncFunction")
	}
	if svc.runStarted.Load() {
		t.Fatal("Run left service marked as started after background submission failure")
	}
}

func TestHTTPServerStartupOnlySchedulesWatchdogTimer(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	httpServer := NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, nil, routine)

	httpServer.state.scheduleWatchdog()
	httpServer.state.scheduleWatchdog()

	if routine.asyncFunctionCalls != 0 {
		t.Fatalf("startup scheduling submitted %d background functions, want no automatic recovery", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 1 {
		t.Fatalf("startup scheduling submitted %d timers, want stale dispatched watchdog timer", routine.timerCalls)
	}
	if routine.function != nil {
		t.Fatal("startup scheduling submitted an automatic recovery function")
	}
}

func TestServerStateScheduleBackgroundFailureReleasesWaitGroup(t *testing.T) {
	submitErr := errors.New("background routine is closed")
	routine := &recordingBackgroundRoutine{err: submitErr}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)

	if state.scheduleBackground("test", func(context.Context) {}) {
		t.Fatal("scheduleBackground returned true after background submission failure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := state.waitBackground(ctx); err != nil {
		t.Fatalf("waitBackground did not observe released wait group: %v", err)
	}
}
