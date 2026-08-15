package server

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Lifecycle helpers manage process-local transport/background resources. They
// do not own Run or Runtime state machines.

func (s *serverState) close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	if s.backgroundCancel != nil {
		s.backgroundCancel()
	}
	for _, unsubscribe := range s.eventUnsubscribes {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	s.eventUnsubscribes = nil
	// Each teardown stage gets its own deadline so an earlier timeout cannot leave
	// later Close/Terminate/Drain calls running against an already-cancelled ctx
	// (which can hang under full-suite load).
	waitCtx, waitCancel := context.WithTimeout(context.Background(), closeStageTimeout(ctx))
	defer waitCancel()
	if err := s.waitBackground(waitCtx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func closeStageTimeout(ctx context.Context) time.Duration {
	const defaultTimeout = 5 * time.Second
	if ctx == nil {
		return defaultTimeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return defaultTimeout
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return defaultTimeout
	}
	if remaining > defaultTimeout {
		return defaultTimeout
	}
	// Keep a usable floor so parallel suite cleanup still terminates.
	if remaining < 500*time.Millisecond {
		return 500 * time.Millisecond
	}
	return remaining
}

func (s *serverState) waitBackground(ctx context.Context) error {
	if s == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		s.backgroundWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *serverState) scheduleBackground(name string, fn func(context.Context)) bool {
	if s == nil || fn == nil {
		return false
	}
	ctx := s.backgroundContext()
	s.backgroundWG.Add(1)
	run := func() {
		defer s.backgroundWG.Done()
		fn(ctx)
	}
	if s.backgroundRoutine != nil {
		if err := s.backgroundRoutine.AsyncFunction(run); err != nil {
			s.backgroundWG.Done()
			slog.Warn("workorch background task scheduling failed", "task", name, "err", err)
			return false
		}
		return true
	}
	s.backgroundWG.Done()
	slog.Warn("workorch background task scheduling skipped because background routine is not configured", "task", name)
	return false
}

func (s *serverState) backgroundContext() context.Context {
	if s != nil && s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}
