package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringHTTPRoutesUseIndependentContracts(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	calls := []string{}
	view := workorchmanagementcontract.DefinitionAuthoringView{WorkspaceID: "ws", WorkspaceRevision: 2, State: "draft", InputHash: "input"}
	state.authoring = DefinitionAuthoringPorts{
		Create: func(_ context.Context, backend, _ string, request workorchmanagementcontract.DefinitionAuthoringCreate) (workorchmanagementcontract.DefinitionAuthoringView, error) {
			if backend != "memory" || request.RequestID != "request" || request.Input.GoalText != "目标" || request.TemplateID != "template" {
				t.Fatalf("wrong create contract: %+v", request)
			}
			calls = append(calls, "create")
			return view, nil
		},
		Read: func(_ context.Context, id string) (workorchmanagementcontract.DefinitionAuthoringView, error) {
			if id != "ws" {
				t.Fatal("wrong workspace")
			}
			calls = append(calls, "read")
			return view, nil
		},
		Save: func(_ context.Context, id string, request workorchmanagementcontract.DefinitionAuthoringSave) (workorchmanagementcontract.DefinitionAuthoringView, error) {
			if id != "ws" || request.ExpectedWorkspaceRevision != 2 || request.ExpectedInputHash != "input" || request.TemplateID != "changed" {
				t.Fatalf("wrong save contract: %+v", request)
			}
			calls = append(calls, "save")
			return view, nil
		},
		Compile: func(_ context.Context, id string) (workorchmanagementcontract.DefinitionAuthoringView, error) {
			calls = append(calls, "compile")
			return view, nil
		},
		Confirm: func(_ context.Context, id string, request workorchmanagementcontract.DefinitionAuthoringConfirm) (workorchmanagementcontract.DefinitionAuthoringView, error) {
			if id != "ws" || request.InputHash != "input" || request.ProposalHash != "proposal" || request.SpecificationHash != "spec" || request.DerivationPermitID != "permit" || request.ExpectedWorkspaceRevision != 2 {
				t.Fatalf("wrong exact confirmation: %+v", request)
			}
			calls = append(calls, "confirm")
			return view, nil
		},
	}
	handler := newMuxWithState(state)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/api/v1/workorch/workspace-authoring", `{"requestId":"request","title":"任务","templateId":"template","input":{"goalText":"目标"}}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/workorch/workspaces/ws/authoring", "", http.StatusOK},
		{http.MethodPut, "/api/v1/workorch/workspaces/ws/authoring", `{"templateId":"changed","input":{"goalText":"新目标"},"expectedWorkspaceRevision":2,"expectedInputHash":"input"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/workorch/workspaces/ws/authoring/compile", "", http.StatusOK},
		{http.MethodPost, "/api/v1/workorch/workspaces/ws/authoring/confirm", `{"inputHash":"input","derivationPermitId":"permit","proposalHash":"proposal","specificationHash":"spec","expectedWorkspaceRevision":2}`, http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), `"authoring"`) {
			t.Fatalf("%s: %d %s", tc.path, recorder.Code, recorder.Body.String())
		}
	}
	if len(calls) != 5 {
		t.Fatalf("missing ports: %v", calls)
	}
	state.authoring.Confirm = func(context.Context, string, workorchmanagementcontract.DefinitionAuthoringConfirm) (workorchmanagementcontract.DefinitionAuthoringView, error) {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws/authoring/confirm", strings.NewReader(`{"inputHash":"stale"}`)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale confirm: %d %s", recorder.Code, recorder.Body.String())
	}
}
