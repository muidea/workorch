package server

import (
	"context"
	"errors"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
		t.Fatal("Startup returned nil error without runtime session")
	}
	session, sessionErr := appservice.NewRuntimeSessionWithBackend("memory", "")
	if sessionErr != nil {
		t.Fatalf("new runtime session: %v", sessionErr)
	}
	t.Cleanup(func() { _ = session.Close(context.Background()) })
	if err := svc.Startup(context.Background(), "workorch", session, nil); err == nil {
		t.Fatal("Startup returned nil error without background routine")
	}
}

func TestServiceRunUsesBackgroundRoutine(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !routine.asyncFunctionCalled {
		t.Fatal("Run did not submit HTTP server through BackgroundRoutine.AsyncFunction")
	}
	if routine.asyncFunctionCalls != 2 {
		t.Fatalf("Run submitted %d background functions, want projection baseline and HTTP server", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 0 {
		t.Fatalf("Run submitted %d timers, want no legacy watchdog timer", routine.timerCalls)
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

func TestServiceRunDoesNotRecoverLegacyInactiveRunningWorkspace(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	const workspaceID = "ws-service-startup-inactive"
	const runID = "run-service-startup-inactive"
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	var err error
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "service startup inactive",
		DeliverableDirectory: "workspaces/service-startup-inactive",
		State:                common.WorkspaceStateActive,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "file", DataRoot: dataRoot}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if routine.asyncFunctionCalls != 2 {
		t.Fatalf("Run submitted %d background functions, want projection baseline and HTTP server", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 0 {
		t.Fatalf("Run submitted %d timers, want no legacy watchdog timer", routine.timerCalls)
	}

	readRepositories := testkit.OpenRepositories("file", dataRoot)
	workspaceRecord, err := readRepositories.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read recovered workspace: %v", err)
	}
	if workspaceRecord.State != common.WorkspaceStateActive || workspaceRecord.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want legacy state left untouched", workspaceRecord)
	}
	runRecord, err := readRepositories.Runs.GetByWorkspaceID(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), workspaceID, runID)
	if err != nil {
		t.Fatalf("read recovered run: %v", err)
	}
	if runRecord.State != common.RunStateActive {
		t.Fatalf("run = %+v, want legacy state left untouched", runRecord)
	}
	if err := svc.http.state.workspaceProjection.RebuildWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("RebuildWorkspace: %v", err)
	}
	snapshot, err := svc.ReadWorkspaceViewSnapshot(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceViewSnapshot: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) || snapshot.Workspace.ActiveRunID != runID {
		t.Fatalf("projected workspace = %+v, want legacy state left untouched", snapshot.Workspace)
	}
	if diagnostics := svc.http.state.schedulerDiagnostics.snapshot(workspaceID, runID); diagnostics != nil && diagnostics.LatestContinuation != nil {
		t.Fatalf("scheduler diagnostics = %+v, want no legacy startup recovery diagnostic", diagnostics)
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
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)

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

func TestServerStateScheduleBackgroundFailureReleasesWaitGroup(t *testing.T) {
	submitErr := errors.New("background routine is closed")
	routine := &recordingBackgroundRoutine{err: submitErr}
	state := newIsolatedTestServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), routine)

	if state.scheduleBackground("test", func(context.Context) {}) {
		t.Fatal("scheduleBackground returned true after background submission failure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := state.waitBackground(ctx); err != nil {
		t.Fatalf("waitBackground did not observe released wait group: %v", err)
	}
}
