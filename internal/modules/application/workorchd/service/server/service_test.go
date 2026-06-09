package server

import (
	"context"
	"errors"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
)

type recordingBackgroundRoutine struct {
	asyncFunctionCalled bool
	asyncFunctionCalls  int
	timerCalls          int
	function            func()
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
	return nil
}

func (r *recordingBackgroundRoutine) Shutdown(ctx context.Context) bool {
	return true
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

func TestHTTPServerStartupRecoveryUsesBackgroundRoutine(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	httpServer := NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, nil, routine)

	httpServer.state.scheduleStartupRecovery()
	httpServer.state.scheduleStartupRecovery()

	if routine.asyncFunctionCalls != 1 {
		t.Fatalf("startup recovery submitted %d background functions, want 1", routine.asyncFunctionCalls)
	}
	if routine.timerCalls != 1 {
		t.Fatalf("startup recovery submitted %d timers, want stale dispatched watchdog timer", routine.timerCalls)
	}
	if routine.function == nil {
		t.Fatal("startup recovery submitted a nil background function")
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
