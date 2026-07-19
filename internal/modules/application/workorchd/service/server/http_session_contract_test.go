package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

// SM-4 protocol boundary: a read route may use a session-backed application
// operation, but the HTTP request context must not expose EventHub.
func TestWorkspaceListRouteDoesNotExposeEventHub(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.listWorkspaces = func(ctx context.Context, _, _ string) ([]appservice.Workspace, error) {
		if got := appservice.CommandHubFromContext(ctx); got != nil {
			t.Fatalf("HTTP request context exposed EventHub: %#v", got)
		}
		return []appservice.Workspace{{ID: "ws-session", Title: "session backed"}}, nil
	}

	recorder := httptest.NewRecorder()
	newMuxWithState(state).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace list status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
}
