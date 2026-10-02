package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredDefinitionWriteRoutesDoNotCallApplicationPorts(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.createWorkspace = nil
	state.defineGoal = nil
	state.updateGoal = nil
	state.defineDoD = nil
	state.updateDoD = nil
	handler := newMuxWithState(state)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/workorch/workspaces"},
		{http.MethodPost, "/api/v1/workorch/workspaces/retired/goals"},
		{http.MethodPut, "/api/v1/workorch/workspaces/retired/goals/goal"},
		{http.MethodPost, "/api/v1/workorch/workspaces/retired/dods"},
		{http.MethodPut, "/api/v1/workorch/workspaces/retired/dods/dod"},
		{http.MethodPost, "/api/v1/workorch/workspaces/retired/goal-dod-quality-check"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`)))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("退休写路由 %s %s: %d", route.method, route.path, recorder.Code)
		}
	}
}

func TestRetiredDraftRoutesHaveNoSessionSideEffects(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	for _, path := range []string{
		"/api/v1/workorch/workspace-setup-draft-sessions",
		"/api/v1/workorch/workspace-setup-draft-sessions/retired/messages",
		"/api/v1/workorch/workspace-setup-draft-sessions/retired/regenerate",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("retired route %s: %d", path, recorder.Code)
		}
	}
	if len(state.workspaceSetupDraftSessions) != 0 {
		t.Fatal("retired route created an in-memory session")
	}
}
