package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestStartupPreparationHTTPReadUsesOnlyPersistedStatePort(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	// Use the full production route set, but expose no state/probe/Run ports
	// other than this query. A regression invoking a different callback fails.
	state.applicationOperations = applicationOperations{}
	calls := 0
	state.readStartupPreparation = func(ctx context.Context, id string) (workorchmanagementcontract.StartupPreparationView, error) {
		if id != "ws-preparation" || ctx == nil {
			t.Fatal("wrong query scope")
		}
		calls++
		return workorchmanagementcontract.StartupPreparationView{WorkspaceID: id, WorkspaceRevision: 7, ActiveRunID: "actual-run", OperationState: "not_checked", CurrentValidity: "not_checked"}, nil
	}
	handler := newMuxWithStateNoStart(state)
	for n := 0; n < 3; n++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-preparation/activation-preparation", nil))
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read: %d %s", recorder.Code, recorder.Body.String())
		}
		var response startupPreparationResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Preparation.ValidateFor("ws-preparation") != nil || response.Preparation.ActiveRunID != "actual-run" {
			t.Fatalf("read projection lost Run/state: %+v %v", response, err)
		}
		for _, forbidden := range []string{`"ready"`, `"eligible"`, `"m3"`, `"noRun"`, `"WorkspaceID"`, `"record"`} {
			if strings.Contains(recorder.Body.String(), forbidden) {
				t.Fatalf("query invented fact: %s", recorder.Body.String())
			}
		}
	}
	if calls != 3 {
		t.Fatalf("query calls=%d", calls)
	}
	router := engine.NewRouteRegistry()
	registerStartupPreparationReadRoutes(router, state)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if router.ExistHandler("/api/v1/workorch/workspaces/:workspaceId/activation-preparation", method) {
			t.Fatal("unfinished mutation exposed")
		}
	}
}

func TestStartupPreparationHTTPRejectsUnavailableWrongScopeAndDiagnosticLeak(t *testing.T) {
	for _, scenario := range []string{"unavailable", "scope", "ready", "zero_revision", "unknown_error", "wrapped_conflict", "not_found"} {
		t.Run(scenario, func(t *testing.T) {
			state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
			state.applicationOperations = applicationOperations{}
			want := http.StatusConflict
			if scenario == "unavailable" {
				want = http.StatusServiceUnavailable
			} else {
				state.readStartupPreparation = func(context.Context, string) (workorchmanagementcontract.StartupPreparationView, error) {
					view := workorchmanagementcontract.StartupPreparationView{WorkspaceID: "ws", WorkspaceRevision: 1, OperationState: "not_checked", CurrentValidity: "not_checked"}
					switch scenario {
					case "scope":
						view.WorkspaceID = "other"
					case "ready":
						view.OperationState = "ready"
					case "zero_revision":
						view.WorkspaceRevision = 0
					case "unknown_error":
						return view, errors.New("secret-canary/raw diagnostic")
					case "wrapped_conflict":
						return view, fmt.Errorf("secret-canary: %w", common.ErrConflict)
					case "not_found":
						return view, common.ErrNotFound
					}
					return view, nil
				}
			}
			if scenario == "unknown_error" {
				want = http.StatusInternalServerError
			}
			if scenario == "not_found" {
				want = http.StatusNotFound
			}
			recorder := httptest.NewRecorder()
			newMuxWithStateNoStart(state).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws/activation-preparation", nil))
			if recorder.Code != want || strings.Contains(recorder.Body.String(), "secret-canary") || strings.Contains(recorder.Body.String(), `"preparation"`) {
				t.Fatalf("unsafe response: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStartupPreparationHTTPCompletionPreservesZeroEpoch(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../../pkg/workorchmanagementcontract/testdata/startup_preparation_completed.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, validity := range []string{"observed", "stale"} {
		for _, action := range []string{"read", "check", "recover"} {
			t.Run(validity+"/"+action, func(t *testing.T) {
				var view workorchmanagementcontract.StartupPreparationView
				if err := json.Unmarshal(raw, &view); err != nil {
					t.Fatal(err)
				}
				view.CurrentValidity = validity
				if validity == "stale" {
					view.ActiveRunID = "later-run"
					view.WorkspaceRevision++
				}
				if err := view.ValidateFor("ws"); err != nil {
					t.Fatal(err)
				}
				state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
				state.applicationOperations = applicationOperations{}
				calls := 0
				state.readStartupPreparation = func(_ context.Context, id string) (workorchmanagementcontract.StartupPreparationView, error) {
					calls++
					if id != view.WorkspaceID {
						t.Fatal("wrong query scope")
					}
					return view, nil
				}
				port := func(_ context.Context, id string, request workorchmanagementcontract.PreparationCheckRequest) (workorchmanagementcontract.PreparationOperationView, error) {
					calls++
					if id != view.WorkspaceID || request.RequestID != "original-request" || request.ExpectedWorkspaceRevision != 1 {
						t.Fatal("changed original operation")
					}
					return workorchmanagementcontract.PreparationOperationView{RequestID: request.RequestID, Preparation: view}, nil
				}
				state.checkStartupPreparation, state.recoverStartupPreparation = port, port
				method, url, body := http.MethodGet, "/api/v1/workorch/workspaces/ws/activation-preparation", ""
				if action != "read" {
					method, url, body = http.MethodPost, url+"/"+action, `{"requestId":"original-request","expectedWorkspaceRevision":1}`
				}
				recorder := httptest.NewRecorder()
				newMuxWithStateNoStart(state).ServeHTTP(recorder, httptest.NewRequest(method, url, strings.NewReader(body)))
				if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || calls != 1 {
					t.Fatalf("unexpected response: %d %s calls=%d", recorder.Code, recorder.Body, calls)
				}
				var response struct {
					Preparation struct {
						Record struct {
							Attempts []struct {
								Epoch json.RawMessage `json:"preparationEpoch"`
							} `json:"attempts"`
						} `json:"record"`
					} `json:"preparation"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if len(response.Preparation.Record.Attempts) != 1 || !bytes.Equal(response.Preparation.Record.Attempts[0].Epoch, []byte("0")) {
					t.Fatalf("zero epoch missing or changed: %s", recorder.Body)
				}
				var typed startupPreparationResponse
				if err := json.Unmarshal(recorder.Body.Bytes(), &typed); err != nil || typed.Preparation.ValidateFor("ws") != nil {
					t.Fatalf("completion wire rejected: %v", err)
				}
			})
		}
	}
}
