package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
