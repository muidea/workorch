package workorch

import (
	"context"
	"errors"
	"testing"
)

type lifecycleFailureService struct {
	startupError  error
	runError      error
	shutdownError error
	panicStage    string
	panicValue    error
	shutdowns     int
	runs          int
}

func (s *lifecycleFailureService) Startup(context.Context) error {
	if s.panicStage == "startup" {
		panic(s.panicValue)
	}
	return s.startupError
}
func (s *lifecycleFailureService) Run(context.Context) error {
	s.runs++
	if s.panicStage == "run" {
		panic(s.panicValue)
	}
	return s.runError
}
func (s *lifecycleFailureService) Shutdown(ctx context.Context) error {
	s.shutdowns++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.panicStage == "shutdown" {
		panic(s.panicValue)
	}
	return s.shutdownError
}

func TestRunLifecycleAlwaysPerformsCheckedShutdownAndPreservesCauses(t *testing.T) {
	failure, cleanup := errors.New("lifecycle failure"), errors.New("cleanup failure")
	for _, stage := range []string{"success", "startup-error", "run-error", "startup", "run", "shutdown"} {
		t.Run(stage, func(t *testing.T) {
			svc := &lifecycleFailureService{panicStage: stage, panicValue: failure}
			switch stage {
			case "startup-error":
				svc.startupError = failure
			case "run-error":
				svc.runError = failure
			}
			if stage != "success" && stage != "shutdown" {
				svc.shutdownError = cleanup
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := runLifecycle(ctx, svc)
			if svc.shutdowns != 1 {
				t.Fatalf("shutdowns=%d", svc.shutdowns)
			}
			if stage == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatalf("cause lost: %v", err)
			}
			if svc.shutdownError != nil && !errors.Is(err, cleanup) {
				t.Fatalf("cleanup cause lost: %v", err)
			}
			if (stage == "startup-error" || stage == "startup") && svc.runs != 0 {
				t.Fatal("Run entered after failed Startup")
			}
		})
	}
}

func TestInteractiveShutdownRetainsFailedBackendAndLoggingForRetry(t *testing.T) {
	failure := errors.New("backend close failed")
	closes, restores := 0, 0
	svc := &interactiveService{
		closeBackend:   func(context.Context) error { closes++; return failure },
		restoreLogging: func() { restores++ },
	}
	if err := svc.Shutdown(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if svc.closeBackend == nil || svc.restoreLogging == nil || restores != 0 {
		t.Fatal("failed close discarded retry handles")
	}
	svc.closeBackend = func(context.Context) error { closes++; return nil }
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closes != 2 || restores != 1 {
		t.Fatalf("closes=%d restores=%d", closes, restores)
	}
}
