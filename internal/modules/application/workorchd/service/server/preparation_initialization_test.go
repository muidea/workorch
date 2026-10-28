package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationInitializationHTTPStrictAndReconciliable(t *testing.T) {
	for _, action := range []string{"initialize-resources", "recover-resources"} {
		for _, scenario := range []string{"completed", "pending", "blocked", "unknown_field", "trailing", "invalid", "wrong_scope", "wrong_request", "wrong_revision", "raw_error", "unavailable"} {
			t.Run(action+"/"+scenario, func(t *testing.T) {
				state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
				state.applicationOperations = applicationOperations{}
				calls := 0
				port := func(_ context.Context, id string, c management.PreparationCheckRequest) (management.PreparationResourceInitializationOperation, error) {
					calls++
					v := management.PreparationResourceInitializationOperation{RequestID: c.RequestID, Preparation: management.StartupPreparationView{WorkspaceID: id, WorkspaceRevision: c.ExpectedWorkspaceRevision + 2, OperationState: "not_checked", CurrentValidity: "not_checked", ResourceInitializations: []management.PreparationResourceInitializationView{{RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: c.ExpectedWorkspaceRevision + 2, State: "completed", Attempts: 1}}}}
					switch scenario {
					case "pending":
						v.Preparation.ResourceInitializations[0].State = "pending"
						v.Preparation.ResourceInitializations[0].Attempts = 1
					case "blocked":
						v.Preparation.ResourceInitializations[0].State = "blocked"
					case "wrong_scope":
						v.Preparation.WorkspaceID = "other"
					case "wrong_request":
						v.RequestID = "other"
					case "wrong_revision":
						v.Preparation.ResourceInitializations[0].ExpectedWorkspaceRevision++
					case "raw_error":
						return v, errors.New("raw-initialization-secret")
					}
					return v, nil
				}
				if scenario != "unavailable" {
					if action == "initialize-resources" {
						state.initializeStartupPreparationResources = port
					} else {
						state.recoverStartupPreparationResources = port
					}
				}
				body := `{"requestId":"original","expectedWorkspaceRevision":7}`
				want := http.StatusOK
				switch scenario {
				case "pending":
					want = http.StatusAccepted
				case "unknown_field":
					body = `{"requestId":"original","expectedWorkspaceRevision":7,"defaults":{"injected":true}}`
					want = http.StatusBadRequest
				case "trailing":
					body += " {}"
					want = http.StatusBadRequest
				case "invalid":
					body = `{"requestId":"original"}`
					want = http.StatusBadRequest
				case "wrong_scope", "wrong_request", "wrong_revision":
					want = http.StatusConflict
				case "raw_error":
					want = http.StatusInternalServerError
				case "unavailable":
					want = http.StatusServiceUnavailable
				}
				rr := httptest.NewRecorder()
				newMuxWithStateNoStart(state).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws/activation-preparation/"+action, strings.NewReader(body)))
				if rr.Code != want || rr.Header().Get("Cache-Control") != "no-store" || strings.Contains(rr.Body.String(), "raw-initialization-secret") {
					t.Fatalf("response %d %s", rr.Code, rr.Body.String())
				}
				if (want == http.StatusBadRequest || scenario == "unavailable") && calls != 0 {
					t.Fatal("invalid request forwarded")
				}
			})
		}
	}
}
