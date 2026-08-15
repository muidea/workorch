package mcp

import (
	"context"
	"time"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

func (a *RuntimeAdapter) Close() {
	if a == nil {
		return
	}
	a.sessionMu.Lock()
	sessions := a.sessions
	a.sessions = map[string]*stdioSession{}
	a.sessionMu.Unlock()
	for _, session := range sessions {
		session.Close()
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
	time.AfterFunc(a.idleSessionTimeout, func() {
		a.evictIdleStdioSession(serverID, session)
	})
}

func (a *RuntimeAdapter) evictIdleStdioSession(serverID string, session *stdioSession) {
	if a == nil || session == nil {
		return
	}
	ttl := a.idleSessionTimeout
	if ttl <= 0 {
		return
	}
	session.mu.Lock()
	idleFor := time.Since(session.lastUsed)
	if idleFor < ttl {
		delay := ttl - idleFor
		session.mu.Unlock()
		time.AfterFunc(delay, func() {
			a.evictIdleStdioSession(serverID, session)
		})
		return
	}
	session.mu.Unlock()
	if a.evictStdioSession(serverID, session) {
		session.Close()
	}
}
