package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func preparationOperationHTTPFixture(id string, request management.PreparationCheckRequest, state string) management.PreparationOperationView {
	at := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	checkState, reason := "pending", "not_observed"
	var completed *time.Time
	if state == "blocked" {
		checkState, reason, completed = "blocked", "check_pipeline_incomplete", &at
	}
	return management.PreparationOperationView{RequestID: request.RequestID, Preparation: management.StartupPreparationView{
		WorkspaceID: id, WorkspaceRevision: request.ExpectedWorkspaceRevision + 2, OperationState: state, CurrentValidity: "not_evaluated",
		Record: &management.PreparationRecordView{Version: "workorch-preparation-record/v2", Attempts: []management.PreparationAttemptView{{
			RequestID: request.RequestID, PayloadHash: "payload", ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: request.ExpectedWorkspaceRevision + 2,
			StartedAt: at, CompletedAt: completed, State: state, CreationEpoch: strings.Repeat("a", 32),
			Binding: management.PreparationBindingView{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec", CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
			Checks:  []management.PreparationCheckView{{ID: "exact_seal", Owner: "workspace", Required: true, State: checkState, Reason: reason}},
		}}},
	}}
}

func TestPreparationWorkflowHTTPCommandsStrictIdentityAndSafeResponses(t *testing.T) {
	for _, action := range []string{"check", "recover"} {
		for _, scenario := range []string{"blocked", "accepted", "unknown", "unavailable", "invalid", "unknown_field", "trailing", "overflow", "scope", "request", "ready", "error"} {
			t.Run(action+"/"+scenario, func(t *testing.T) {
				state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
				state.applicationOperations = applicationOperations{}
				calls := 0
				port := func(ctx context.Context, id string, request management.PreparationCheckRequest) (management.PreparationOperationView, error) {
					calls++
					if id != "ws" || request.RequestID != "original" || request.ExpectedWorkspaceRevision != 7 || ctx == nil {
						t.Fatal("transport replaced scope or original revision")
					}
					value := preparationOperationHTTPFixture(id, request, "blocked")
					switch scenario {
					case "accepted", "unknown":
						value = preparationOperationHTTPFixture(id, request, scenario)
					case "scope":
						value.Preparation.WorkspaceID = "other"
					case "request":
						value.RequestID = "other"
					case "ready":
						value.Preparation.OperationState = "ready"
					case "error":
						return value, errors.New("raw-secret-canary")
					}
					return value, nil
				}
				if scenario != "unavailable" {
					if action == "check" {
						state.checkStartupPreparation = port
					} else {
						state.recoverStartupPreparation = port
					}
				}
				body := `{"requestId":"original","expectedWorkspaceRevision":7}`
				want := http.StatusOK
				switch scenario {
				case "accepted", "unknown":
					want = http.StatusAccepted
				case "unavailable":
					want = http.StatusServiceUnavailable
				case "invalid":
					body, want = `{"requestId":"original"}`, http.StatusBadRequest
				case "unknown_field":
					body, want = `{"requestId":"original","expectedWorkspaceRevision":7,"checks":[{"state":"PASS"}]}`, http.StatusBadRequest
				case "trailing":
					body, want = body+` {"ready":true}`, http.StatusBadRequest
				case "overflow":
					body, want = body+strings.Repeat(" ", 4096), http.StatusBadRequest
				case "scope", "request", "ready":
					want = http.StatusConflict
				case "error":
					want = http.StatusInternalServerError
				}
				recorder := httptest.NewRecorder()
				newMuxWithStateNoStart(state).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws/activation-preparation/"+action, strings.NewReader(body)))
				if recorder.Code != want || recorder.Header().Get("Cache-Control") != "no-store" || strings.Contains(recorder.Body.String(), "raw-secret-canary") {
					t.Fatalf("unsafe/unexpected response: %d %s", recorder.Code, recorder.Body.String())
				}
				wantCalls := 1
				if want == http.StatusBadRequest || scenario == "unavailable" {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatalf("unexpected dispatch count %d", calls)
				}
			})
		}
	}
}
