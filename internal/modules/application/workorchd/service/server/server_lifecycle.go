package server

import (
	"context"
	"log/slog"
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
	s.closeMu.Lock()
	if s.closeGate == nil {
		s.closeGate = make(chan struct{}, 1)
	}
	gate := s.closeGate
	s.closeMu.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if s.closed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.backgroundMu.Lock()
	s.backgroundStopping = true
	s.backgroundMu.Unlock()
	if s.backgroundCancel != nil {
		s.backgroundCancel()
	}
	// 超时不是完成回执；已接纳 transport 任务真实返回前，
	// 保留订阅和所有依赖句柄，允许用新的预算重试。
	if err := s.waitBackground(ctx); err != nil {
		return err
	}
	for len(s.eventUnsubscribes) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		unsubscribe := s.eventUnsubscribes[0]
		if unsubscribe != nil {
			unsubscribe()
		}
		// 仅在成功返回后登记清理完成；未知 panic 原样传播并保留当前
		// 清理项，不能静默宣称该项已释放。
		s.eventUnsubscribes = s.eventUnsubscribes[1:]
	}
	s.closed = true
	return nil
}

func (s *serverState) waitBackground(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		s.backgroundMu.Lock()
		if s.backgroundActive == 0 {
			s.backgroundMu.Unlock()
			return nil
		}
		if s.backgroundChanged == nil {
			s.backgroundChanged = make(chan struct{})
		}
		changed := s.backgroundChanged
		s.backgroundMu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *serverState) finishBackground() {
	s.backgroundMu.Lock()
	s.backgroundActive--
	if s.backgroundChanged != nil {
		close(s.backgroundChanged)
		s.backgroundChanged = nil
	}
	s.backgroundMu.Unlock()
}

func (s *serverState) scheduleBackground(name string, fn func(context.Context)) bool {
	if s == nil || fn == nil {
		return false
	}
	s.backgroundMu.Lock()
	if s.backgroundStopping {
		s.backgroundMu.Unlock()
		return false
	}
	s.backgroundActive++
	s.backgroundMu.Unlock()
	ctx := s.backgroundContext()
	run := func() {
		defer s.finishBackground()
		fn(ctx)
	}
	if s.backgroundRoutine != nil {
		if err := s.backgroundRoutine.AsyncFunction(run); err != nil {
			s.finishBackground()
			slog.Warn("workorch background task scheduling failed", "task", name, "err", err)
			return false
		}
		return true
	}
	s.finishBackground()
	slog.Warn("workorch background task scheduling skipped because background routine is not configured", "task", name)
	return false
}

func (s *serverState) backgroundContext() context.Context {
	if s != nil && s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}
