package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	mux := newMuxWithState(state)
	// The production route intentionally rejects projection reads while its
	// SM-4 read model is building. This contract test concerns request-context
	// isolation, so wait for that owner-local rebuild rather than racing it.
	waitForCondition(t, time.Second, func() bool {
		return state.workspaceProjectionStatus().State == workspaceProjectionStateReady
	})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace list status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
}
