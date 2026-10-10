package mcp

import (
	"context"
	"time"

	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

// timer 在接纳调度前记账；Stop 返回 false 时只能由真实 callback 释放。
type idleEvictionTimer struct {
	timer *time.Timer
	leave func()
}

func (a *RuntimeAdapter) BeginShutdown() {
	if a == nil {
		return
	}
	a.sessionMu.Lock()
	a.closing = true
	a.idleLifecycle.BeginShutdown()
	for entry := range a.idleTimers {
		if entry.timer.Stop() {
			delete(a.idleTimers, entry)
			entry.leave()
		}
	}
	a.sessionMu.Unlock()
}

func (a *RuntimeAdapter) Quiesce(ctx context.Context) error {
	if a == nil {
		return nil
	}
	return a.idleLifecycle.Quiesce(ctx)
}

// CloseChecked 仅限制等待预算，不假装已停止 session Close。独立执行一次，
// 超时保留同一 completion 与 sessions，后续预算重试不会重复发起 Close。
func (a *RuntimeAdapter) CloseChecked(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		return basebiz.ErrNilLifecycleContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.BeginShutdown()
	if err := a.Quiesce(ctx); err != nil {
		return err
	}
	a.closeOnce.Do(func() {
		a.closeDone = make(chan struct{})
		go func() {
			a.sessionMu.Lock()
			sessions := make([]*stdioSession, 0, len(a.sessions))
			for _, session := range a.sessions {
				sessions = append(sessions, session)
			}
			a.sessionMu.Unlock()
			for _, session := range sessions {
				session.Close()
			}
			a.sessionMu.Lock()
			a.sessions = map[string]*stdioSession{}
			a.sessionMu.Unlock()
			close(a.closeDone)
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.closeDone:
		return ctx.Err()
	}
}

func (a *RuntimeAdapter) Close() {
	if a != nil {
		_ = a.CloseChecked(context.Background())
	}
}

func (a *RuntimeAdapter) getOrCreateStdioSession(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
	a.sessionMu.Lock()
	if session := a.sessions[serverID]; session != nil {
		a.sessionMu.Unlock()
		return session, nil
	}
	a.sessionMu.Unlock()

	session, err := newInitializedStdioSession(ctx, request, server, serverID)
	if err != nil {
		return nil, err
	}
	a.sessionMu.Lock()
	if existing := a.sessions[serverID]; existing != nil {
		a.sessionMu.Unlock()
		session.Close()
		return existing, nil
	}
	a.sessions[serverID] = session
	a.sessionMu.Unlock()
	a.scheduleStdioSessionIdleEviction(serverID, session)
	return session, nil
}

func (a *RuntimeAdapter) evictStdioSession(serverID string, target *stdioSession) bool {
	a.sessionMu.Lock()
	if a.sessions[serverID] == target {
		delete(a.sessions, serverID)
		a.sessionMu.Unlock()
		return true
	}
	a.sessionMu.Unlock()
	return false
}

func (a *RuntimeAdapter) scheduleStdioSessionIdleEviction(serverID string, session *stdioSession) {
	if a == nil || session == nil || a.idleSessionTimeout <= 0 {
		return
	}
	a.scheduleIdleEviction(serverID, session, a.idleSessionTimeout)
}

func (a *RuntimeAdapter) scheduleIdleEviction(serverID string, session *stdioSession, delay time.Duration) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	if a.closing {
		return
	}
	leave, err := a.idleLifecycle.Enter()
	if err != nil {
		return
	}
	entry := &idleEvictionTimer{leave: leave}
	if a.idleTimers == nil {
		a.idleTimers = map[*idleEvictionTimer]struct{}{}
	}
	a.idleTimers[entry] = struct{}{}
	entry.timer = time.AfterFunc(delay, func() {
		defer leave()
		a.sessionMu.Lock()
		delete(a.idleTimers, entry)
		closing := a.closing
		a.sessionMu.Unlock()
		if !closing {
			a.evictIdleStdioSession(serverID, session)
		}
	})
}

func (a *RuntimeAdapter) evictIdleStdioSession(serverID string, session *stdioSession) {
	if a == nil || session == nil || a.idleSessionTimeout <= 0 {
		return
	}
	session.mu.Lock()
	idleFor := time.Since(session.lastUsed)
	if idleFor < a.idleSessionTimeout {
		delay := a.idleSessionTimeout - idleFor
		session.mu.Unlock()
		a.scheduleIdleEviction(serverID, session, delay)
		return
	}
	session.mu.Unlock()
	if a.evictStdioSession(serverID, session) {
		session.Close()
	}
}
