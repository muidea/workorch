package server

import (
	"context"
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

func TestRetiredDraftRoutesHaveNoOwnerSideEffects(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	for _, body := range []string{`{}`, `{"templateId":"retired","title":"草稿","content":"新要求","draft":{"goalText":"旧目标","doDItems":["旧验收"]}}`} {
		for _, path := range []string{
			"/api/v1/workorch/workspace-setup-draft-sessions",
			"/api/v1/workorch/workspace-setup-draft-sessions/retired/messages",
			"/api/v1/workorch/workspace-setup-draft-sessions/retired/regenerate",
		} {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("retired route %s: %d", path, recorder.Code)
			}
		}
	}
	workspaces, err := state.listWorkspaces(context.Background(), state.config.Backend, state.config.DataRoot)
	if err != nil || len(workspaces) != 0 {
		t.Fatalf("退休草稿路由写入 Workspace owner: %+v / %v", workspaces, err)
	}
}
