package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
)

func TestHealthzReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	newMux(Config{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Service != "workorch" || body.Status != "ok" {
		t.Fatalf("body = %+v, want workorch/ok", body)
	}
}

func TestReadyzReturnsServiceUnavailableWhenBackendInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "unsupported"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestReadyzNativeRouteInjectsEventHub(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), hub)
	state.validate = func(ctx context.Context, _, _ string) error {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("event hub from context = %#v, want injected hub", got)
		}
		return nil
	}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestConfigurationEndpointReturnsRedactedLLMProviders(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.readConfiguration = func() (appservice.ServiceConfiguration, error) {
		return appservice.ServiceConfiguration{
			Server:     appservice.ServerConfiguration{Addr: ":8080", MaxConcurrentRuns: 1},
			Repository: appservice.RepositoryConfiguration{Backend: "file", DataDir: ".workorch-data"},
			LLM: appservice.LLMConfiguration{
				DefaultProviderID: "local",
				Providers: []appservice.LLMProviderConfiguration{{
					ID:               "local",
					Provider:         "compatible",
					BaseURL:          "http://127.0.0.1:11434/v1",
					Model:            "local-model",
					Timeout:          "60s",
					Ready:            true,
					Default:          true,
					APIKeyConfigured: true,
				}},
			},
		}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/configuration", nil)
	rec := httptest.NewRecorder()

	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var body configurationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal configuration: %v", err)
	}
	if body.Configuration.LLM.DefaultProviderID != "local" ||
		len(body.Configuration.LLM.Providers) != 1 ||
		!body.Configuration.LLM.Providers[0].APIKeyConfigured ||
		strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("body = %s, want redacted configuration response", rec.Body.String())
	}
}

func TestUpdateLLMConfigurationEndpointReturnsRedactedConfiguration(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.updateLLMConfiguration = func(cmd appservice.UpdateLLMConfigurationCommand) (appservice.ServiceConfiguration, error) {
		if cmd.DefaultProviderID != "local" || len(cmd.Providers) != 1 || cmd.Providers[0].APIKey != "secret-value" {
			t.Fatalf("update command = %+v, want local provider command", cmd)
		}
		return appservice.ServiceConfiguration{
			LLM: appservice.LLMConfiguration{
				DefaultProviderID: "local",
				Providers: []appservice.LLMProviderConfiguration{{
					ID:               "local",
					Provider:         "compatible",
					BaseURL:          "http://127.0.0.1:11434/v1",
					Model:            "local-model",
					Timeout:          "60s",
					Ready:            true,
					Default:          true,
					APIKeyConfigured: true,
				}},
			},
		}, nil
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/configuration/llm", strings.NewReader(`{
		"defaultProviderId":"local",
		"providers":[{
			"id":"local",
			"provider":"compatible",
			"baseUrl":"http://127.0.0.1:11434/v1",
			"apiKey":"secret-value",
			"model":"local-model",
			"timeout":"60s"
		}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-value") || !strings.Contains(rec.Body.String(), `"apiKeyConfigured": true`) {
		t.Fatalf("body = %s, want redacted updated configuration", rec.Body.String())
	}
}

func TestMemoryBackendPersistsStateAcrossHTTPRequests(t *testing.T) {
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "memory http", "workspaces/ws-memory-http")
	workspaceID := workspace.ID

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body = %s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"id": "`+workspaceID+`"`) {
		t.Fatalf("list body = %s, want created workspace", listRec.Body.String())
	}

	goalID, dodID := defineGoalAndDoDForHTTP(t, mux, workspaceID)
	createRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/runs", strings.NewReader(`{"runId":"run-memory-http","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	createRunReq.Header.Set("Content-Type", "application/json")
	createRunRec := httptest.NewRecorder()
	mux.ServeHTTP(createRunRec, createRunReq)
	if createRunRec.Code != http.StatusCreated {
		t.Fatalf("create run status = %d, want 201, body = %s", createRunRec.Code, createRunRec.Body.String())
	}

	listRunReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs?workspaceId="+workspaceID, nil)
	listRunRec := httptest.NewRecorder()
	mux.ServeHTTP(listRunRec, listRunReq)
	if listRunRec.Code != http.StatusOK {
		t.Fatalf("list run status = %d, want 200, body = %s", listRunRec.Code, listRunRec.Body.String())
	}
	if !strings.Contains(listRunRec.Body.String(), `"id": "run-memory-http"`) {
		t.Fatalf("list run body = %s, want created run", listRunRec.Body.String())
	}

	startRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-memory-http/start", nil)
	startRunRec := httptest.NewRecorder()
	mux.ServeHTTP(startRunRec, startRunReq)
	if startRunRec.Code != http.StatusOK {
		t.Fatalf("start run status = %d, want 200, body = %s", startRunRec.Code, startRunRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-memory-http/status", nil)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("run status code = %d, want 200, body = %s", statusRec.Code, statusRec.Body.String())
	}
	if !strings.Contains(statusRec.Body.String(), `"state": "completed"`) {
		t.Fatalf("run status body = %s, want completed", statusRec.Body.String())
	}

	observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-memory-http/observability", nil)
	observabilityRec := httptest.NewRecorder()
	mux.ServeHTTP(observabilityRec, observabilityReq)
	if observabilityRec.Code != http.StatusOK {
		t.Fatalf("run observability code = %d, want 200, body = %s", observabilityRec.Code, observabilityRec.Body.String())
	}
	if !strings.Contains(observabilityRec.Body.String(), `"terminal": true`) || !strings.Contains(observabilityRec.Body.String(), `"runtimeCheckpointTotal"`) {
		t.Fatalf("run observability body = %s, want terminal progress and evidence counters", observabilityRec.Body.String())
	}
}

func TestContextRegistryAPIsPersistAcrossMemoryHTTPRequests(t *testing.T) {
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "context registry http", "workspaces/ws-context-registry")

	postAndExpect(t, mux, "/api/v1/workorch/souls", `{
		"id":"soul-engineering-delivery",
		"name":"engineering delivery",
		"systemInstruction":"Plan and cite evidence.",
		"allowedSkillRefs":["skill-local-go-service"],
		"allowedMCPServerRefs":["mcp-local-context"],
		"allowedToolKinds":["shell/command"]
	}`, http.StatusCreated)
	postAndExpect(t, mux, "/api/v1/workorch/skills", `{
		"id":"skill-local-go-service",
		"name":"local go service",
		"version":"v1",
		"instruction":"Use small verified steps.",
		"requiredToolKinds":["shell/command"],
		"requiredMCPServerRefs":["mcp-local-context"]
	}`, http.StatusCreated)
	postAndExpect(t, mux, "/api/v1/workorch/mcp/servers", `{
		"id":"mcp-local-context",
		"name":"local context",
		"transport":"stdio",
		"enabled":true,
		"allowedTools":["repo.summary"]
	}`, http.StatusCreated)
	postAndExpect(t, mux, "/api/v1/workorch/workspaces/"+workspace.ID+"/memory", `{
		"sourceType":"user",
		"memoryType":"project_context",
		"summary":"This workspace is verified through local workorchd HTTP APIs.",
		"tags":["local","acceptance"]
	}`, http.StatusCreated)

	for _, spec := range []struct {
		path string
		want string
	}{
		{path: "/api/v1/workorch/souls", want: `"id": "soul-engineering-delivery"`},
		{path: "/api/v1/workorch/souls/soul-engineering-delivery", want: `"allowedSkillRefs": [`},
		{path: "/api/v1/workorch/skills", want: `"id": "skill-local-go-service"`},
		{path: "/api/v1/workorch/skills/skill-local-go-service", want: `"requiredMCPServerRefs": [`},
		{path: "/api/v1/workorch/mcp/servers", want: `"id": "mcp-local-context"`},
		{path: "/api/v1/workorch/mcp/servers/mcp-local-context/tools", want: `"name": "repo.summary"`},
		{path: "/api/v1/workorch/memory?workspaceId=" + workspace.ID + "&q=HTTP", want: `"memoryType": "project_context"`},
	} {
		req := httptest.NewRequest(http.MethodGet, spec.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200, body = %s", spec.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), spec.want) {
			t.Fatalf("%s body = %s, want %q", spec.path, rec.Body.String(), spec.want)
		}
	}
}

func TestWorkspaceExecutionActivationEndpointCreatesAndStartsRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "activation http", "workspaces/ws-activation-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, mux, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{"llmProviderId":"reasoning"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activation status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var body workspaceExecutionActivationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal activation: %v", err)
	}
	if !strings.HasPrefix(body.Activation.Run.ID, "run-") ||
		body.Activation.Run.WorkspaceID != workspace.ID ||
		body.Activation.Run.GoalID != goalID ||
		body.Activation.Run.DoDID != dodID ||
		body.Activation.Run.State != "failed" {
		t.Fatalf("activation run = %+v, want generated failed llm run with definitions", body.Activation.Run)
	}
	if !strings.Contains(body.Activation.Run.FailureReason, `llm provider "reasoning" is not configured`) {
		t.Fatalf("activation failure reason = %q, want selected llm provider configuration failure", body.Activation.Run.FailureReason)
	}
	if body.Activation.Run.ExecutionContext[llmcommon.ExecutionContextLLMProviderID] != "reasoning" {
		t.Fatalf("executionContext = %+v, want selected llm provider", body.Activation.Run.ExecutionContext)
	}
	if len(body.Activation.Snapshot.Runs) != 1 || body.Activation.Snapshot.Runs[0].ID != body.Activation.Run.ID {
		t.Fatalf("activation snapshot runs = %+v, want created run", body.Activation.Snapshot.Runs)
	}
}

func useUnavailableLLMProvider(t *testing.T) {
	t.Helper()
	t.Setenv(llmcommon.EnvProvider, "")
	t.Setenv(llmcommon.EnvBaseURL, "")
	t.Setenv(llmcommon.EnvAPIKey, "")
	t.Setenv(llmcommon.EnvModel, "")
	t.Setenv(llmcommon.EnvDisabled, "")
	t.Setenv(llmcommon.EnvTimeout, "")
}

func TestAcceptedAPIRoutesAreRegisteredNatively(t *testing.T) {
	router := engine.NewRouteRegistry()
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	registerSystemRoutes(router, state)
	registerReadRoutes(router, state)
	registerBusinessWriteRoutes(router, state)
	registerConsoleRoutes(router)

	for _, route := range acceptedAPINativeRouteSpecs() {
		if !router.ExistHandler(route.pattern, route.method) {
			t.Fatalf("native route %s %s is not registered", route.method, route.pattern)
		}
	}
	for _, method := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodConnect,
		http.MethodTrace,
	} {
		if router.ExistHandler("/**", method) {
			t.Fatalf("wildcard fallback route %s /** must not be registered", method)
		}
	}
}

type routeSpec struct {
	method  string
	pattern string
}

func acceptedAPINativeRouteSpecs() []routeSpec {
	return []routeSpec{
		{method: http.MethodGet, pattern: "/healthz"},
		{method: http.MethodGet, pattern: "/readyz"},
		{method: http.MethodGet, pattern: "/metrics"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/configuration"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/snapshot"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/goals/:goalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/dods/:dodId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-activations"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/snapshot"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/status"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/observability"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/runs/:runId/start"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/runs/:runId/resume"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/decisions"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/decisions/:ticketId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/decisions/:ticketId/resolve"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/approvals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/approvals/:approvalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/approvals/:approvalId/approve"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/approvals/:approvalId/reject"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/souls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/souls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/souls/:soulId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/skills"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/skills"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/skills/:skillId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/mcp/servers"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId/tools"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/memory"},
		{method: http.MethodGet, pattern: "/console"},
		{method: http.MethodGet, pattern: "/console/"},
		{method: http.MethodGet, pattern: "/console/**"},
	}
}

func TestConsoleRoutesServeEmbeddedAssets(t *testing.T) {
	mux := newMux(Config{Backend: "memory"})

	for _, uri := range []string{"/console", "/console/", "/console/workspaces"} {
		req := httptest.NewRequest(http.MethodGet, uri, nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", uri, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "workorch Console") {
			t.Fatalf("%s body does not contain console HTML", uri)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/console/assets/workorch-console.css", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "workorch-section") {
		t.Fatalf("asset body does not contain console css")
	}

	req = httptest.NewRequest(http.MethodGet, "/console/assets/workorch-console.js", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("console js status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Workspace Overview", "Decision Queue", "Workspace Console", "Service Health"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console js body does not contain %q", want)
		}
	}
	for _, want := range []string{"Create workspace", "Define goal", "Define DoD", "Start execution", "Runtime Summary", "Latest Run"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console js body does not contain required business action %q", want)
		}
	}
	for _, forbidden := range []string{"workorch management overview", "Create run", "Run slice", "Available slice catalog"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("console js body exposes retired entry %q", forbidden)
		}
	}
}

func TestNativeBusinessRoutesInjectEventHub(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), hub)
	state.validate = func(context.Context, string, string) error { return nil }
	state.listWorkspaces = func(ctx context.Context, _, _ string) ([]appservice.Workspace, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace list event hub = %#v, want injected hub", got)
		}
		return []appservice.Workspace{{ID: "ws-native-read", Title: "native read"}}, nil
	}
	state.listRuns = func(ctx context.Context, _, _, workspaceID string) ([]appservice.Run, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run list event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return []appservice.Run{{ID: "run-native-read", WorkspaceID: workspaceID, State: "created"}}, nil
	}
	state.readWorkspace = func(ctx context.Context, _, _, workspaceID string) (appservice.Workspace, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace get event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return appservice.Workspace{ID: workspaceID, Title: "native read", State: "active"}, nil
	}
	state.readWorkspaceSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace snapshot event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return appservice.WorkspaceSnapshot{Workspace: appservice.Workspace{ID: workspaceID}}, nil
	}
	state.readRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run get event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: "created"}, nil
	}
	state.readRunSnapshot = func(ctx context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run snapshot event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.RunSnapshot{Run: appservice.Run{ID: runID}}, nil
	}
	state.readRunStatus = func(ctx context.Context, _, _, runID string) (appservice.RunStatus, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run status event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-native-read", State: "created"}, nil
	}
	state.readRunObservability = func(ctx context.Context, _, _, runID string) (appservice.RunObservability, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run observability event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.RunObservability{Run: appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: "created"}}, nil
	}
	state.listDecisions = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Decision, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("decision list event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" || runID != "run-native-read" {
			t.Fatalf("decision filters = %q/%q, want ws-native-read/run-native-read", workspaceID, runID)
		}
		return []appservice.Decision{{ID: "ticket-native-read", WorkspaceID: workspaceID, RunID: runID, State: "open"}}, nil
	}
	state.readDecision = func(ctx context.Context, _, _, ticketID string) (appservice.Decision, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("decision get event hub = %#v, want injected hub", got)
		}
		if ticketID != "ticket-native-read" {
			t.Fatalf("ticketID = %q, want ticket-native-read", ticketID)
		}
		return appservice.Decision{ID: ticketID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "open"}, nil
	}
	state.listApprovals = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval list event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" || runID != "run-native-read" {
			t.Fatalf("approval filters = %q/%q, want ws-native-read/run-native-read", workspaceID, runID)
		}
		return []appservice.Approval{{ID: "approval-native-read", WorkspaceID: workspaceID, RunID: runID, State: "pending"}}, nil
	}
	state.readApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval get event hub = %#v, want injected hub", got)
		}
		if approvalID != "approval-native-read" {
			t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
		}
		return appservice.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "pending"}, nil
	}
	state.createWorkspace = func(ctx context.Context, _, _, title, description, workingDirectory string) (appservice.Workspace, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace create event hub = %#v, want injected hub", got)
		}
		if title != "native write" || description != "native description" || workingDirectory != "workspaces/native-write" {
			t.Fatalf("workspace create input = %q/%q/%q, want native write/native description/workspaces/native-write", title, description, workingDirectory)
		}
		return appservice.Workspace{ID: "ws-native-write", Title: title, Description: description, WorkingDirectory: workingDirectory, State: "active"}, nil
	}
	state.createRun = func(ctx context.Context, _, _, workspaceID, runID, goalID, dodID string) (appservice.Run, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run create event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" || runID != "run-native-write" || goalID != "goal-native" || dodID != "dod-native" {
			t.Fatalf("run create input = %q/%q/%q/%q, want ws-native-read/run-native-write/goal-native/dod-native", workspaceID, runID, goalID, dodID)
		}
		return appservice.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: "created"}, nil
	}
	state.startRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run start event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}, nil
	}
	state.resumeRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run resume event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}, nil
	}
	state.activateWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace activation event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" ||
			cmd.GoalID != "goal-native" ||
			cmd.DoDID != "dod-native" ||
			cmd.SoulID != "soul-native" ||
			!stringSliceEqual(cmd.SkillIDs, []string{"skill-native"}) ||
			!stringSliceEqual(cmd.MCPServerIDs, []string{"mcp-native"}) ||
			!stringSliceEqual(cmd.MemoryIDs, []string{"memory-native"}) {
			t.Fatalf("workspace activation input = %+v for workspace %q, want full context selection", cmd, workspaceID)
		}
		return appservice.WorkspaceExecutionActivation{
			Workspace: appservice.Workspace{ID: workspaceID, ActiveRunID: "run-native-activation"},
			Run:       appservice.Run{ID: "run-native-activation", WorkspaceID: workspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID, State: "completed"},
			Snapshot:  appservice.WorkspaceSnapshot{Workspace: appservice.Workspace{ID: workspaceID}},
		}, nil
	}
	state.resolveDecision = func(ctx context.Context, _, _, ticketID string, cmd appservice.ResolveDecisionCommand) (appservice.Decision, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("decision resolve event hub = %#v, want injected hub", got)
		}
		if ticketID != "ticket-native-read" {
			t.Fatalf("ticketID = %q, want ticket-native-read", ticketID)
		}
		return appservice.Decision{ID: ticketID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "resolved", ResolutionType: cmd.ResolutionType}, nil
	}
	state.approveApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval approve event hub = %#v, want injected hub", got)
		}
		if approvalID != "approval-native-read" {
			t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
		}
		return appservice.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "approved"}, nil
	}
	state.rejectApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval reject event hub = %#v, want injected hub", got)
		}
		if approvalID != "approval-native-read" {
			t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
		}
		return appservice.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "rejected"}, nil
	}
	handler := newMuxWithState(state)

	for _, tc := range []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces", body: `{"title":"native write","description":"native description","workingDirectory":"workspaces/native-write"}`, status: http.StatusCreated},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/runs", body: `{"runId":"run-native-write","goalId":"goal-native","doDId":"dod-native"}`, status: http.StatusCreated},
		{method: http.MethodGet, path: "/api/v1/workorch/runs?workspaceId=ws-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/status", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/observability", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/runs/run-native-read/start", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/runs/run-native-read/resume", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/execution-activations", body: `{"goalId":"goal-native","doDId":"dod-native","soulId":"soul-native","skillIds":["skill-native"],"mcpServerIds":["mcp-native"],"memoryIds":["memory-native"]}`, status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions?workspaceId=ws-native-read&runId=run-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions/ticket-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/decisions/ticket-native-read/resolve", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals?workspaceId=ws-native-read&runId=run-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals/approval-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/approve", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/reject", status: http.StatusOK},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, rec.Code, tc.status)
		}
	}
}

func TestWorkspaceCreateEndpointReturnsCreatedWorkspace(t *testing.T) {
	handler := newMux(Config{Backend: "memory"})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"http workspace","description":"http description","workingDirectory":"workspaces/ws-http"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	var created workspaceResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created workspace: %v", err)
	}
	if !strings.HasPrefix(created.Workspace.ID, "ws-") ||
		created.Workspace.Title != "http workspace" ||
		created.Workspace.Description != "http description" ||
		created.Workspace.WorkingDirectory != "workspaces/ws-http" ||
		created.Workspace.State != "active" {
		t.Fatalf("created workspace = %+v, want ws-http/http workspace/workspaces/ws-http/active", created.Workspace)
	}
}

func TestWorkspaceCreateEndpointDefaultsWorkingDirectory(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"http default directory workspace"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var body workspaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace: %v", err)
	}
	if body.Workspace.ID == "" {
		t.Fatalf("workspace id is empty")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	want := filepath.Join(wd, "workspaces", body.Workspace.ID)
	if body.Workspace.WorkingDirectory != want {
		t.Fatalf("workingDirectory = %q, want %q", body.Workspace.WorkingDirectory, want)
	}
}

func TestWorkspaceListAndGetEndpointsReturnPersistedWorkspaces(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "http workspace", "workspaces/ws-http")
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed workspacesResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal listed workspaces: %v", err)
	}
	if len(listed.Workspaces) != 1 || listed.Workspaces[0].ID != workspace.ID {
		t.Fatalf("listed workspaces = %+v, want %s", listed.Workspaces, workspace.ID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched workspaceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal fetched workspace: %v", err)
	}
	if fetched.Workspace.ID != workspace.ID || fetched.Workspace.Title != "http workspace" || fetched.Workspace.WorkingDirectory != "workspaces/ws-http" {
		t.Fatalf("fetched workspace = %+v, want %s/http workspace/workspaces/ws-http", fetched.Workspace, workspace.ID)
	}
}

func TestWorkspaceCreateEndpointRejectsClientWorkspaceID(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-client","title":"client workspace","workingDirectory":"workspaces/ws-client"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkspaceCreateEndpointReturnsBadRequestForInvalidBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"","title":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkspaceGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-missing", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunCreateEndpointReturnsCreatedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "run http workspace", "workspaces/run-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-http","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)

	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(runRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal created run: %v", err)
	}
	if body.Run.ID != "run-http" || body.Run.WorkspaceID != workspace.ID || body.Run.State != "created" {
		t.Fatalf("created run = %+v, want run-http/%s/created", body.Run, workspace.ID)
	}
	if body.Run.GoalID != goalID || body.Run.DoDID != dodID {
		t.Fatalf("created run goal/dod = %+v, want %s/%s", body.Run, goalID, dodID)
	}
}

func TestRunListAndGetEndpointsReturnPersistedRuns(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceA := createWorkspaceForHTTP(t, handler, "run workspace a", "workspaces/run-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "run workspace b", "workspaces/run-b")
	goalA, dodA := defineGoalAndDoDForHTTP(t, handler, workspaceA.ID)
	goalB, dodB := defineGoalAndDoDForHTTP(t, handler, workspaceB.ID)

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/workorch/workspaces/" + workspaceB.ID + "/runs", body: `{"runId":"run-b","goalId":"` + goalB + `","doDId":"` + dodB + `"}`},
		{path: "/api/v1/workorch/workspaces/" + workspaceA.ID + "/runs", body: `{"runId":"run-a","goalId":"` + goalA + `","doDId":"` + dodA + `"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("run list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed runsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal run list: %v", err)
	}
	if len(listed.Runs) != 2 || listed.Runs[0].ID != "run-a" || listed.Runs[1].ID != "run-b" {
		t.Fatalf("listed runs = %+v, want run-a/run-b", listed.Runs)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs?workspaceId="+workspaceB.ID, nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered run list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered runsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered runs: %v", err)
	}
	if len(filtered.Runs) != 1 || filtered.Runs[0].ID != "run-b" {
		t.Fatalf("filtered runs = %+v, want run-b", filtered.Runs)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-a", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched runInstanceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal run get: %v", err)
	}
	if fetched.Run.ID != "run-a" || fetched.Run.WorkspaceID != workspaceA.ID {
		t.Fatalf("fetched run = %+v, want run-a/%s", fetched.Run, workspaceA.ID)
	}
}

func TestRunSnapshotEndpointReturnsPersistedRunSnapshot(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-snapshot-http"

	workspace := createWorkspaceAndRunForHTTP(t, handler, "run snapshot http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID+"/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body runSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ID != workspace.ID {
		t.Fatalf("workspace = %+v, want %s", body.Snapshot.Workspace, workspace.ID)
	}
	if body.Snapshot.Run.ID != runID || body.Snapshot.Run.State != "blocked_on_decision" {
		t.Fatalf("run = %+v, want blocked decision run", body.Snapshot.Run)
	}
	if body.Snapshot.Step == nil || body.Snapshot.Step.ID == "" {
		t.Fatalf("step = %+v, want current step", body.Snapshot.Step)
	}
	if len(body.Snapshot.Steps) == 0 {
		t.Fatalf("steps = %+v, want non-empty", body.Snapshot.Steps)
	}
	if len(body.Snapshot.Decisions) != 1 || body.Snapshot.Decisions[0].ID != started.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", body.Snapshot.Decisions, started.BlockingTicketID)
	}
}

func TestRunSnapshotEndpointReturnsExecutionEvidence(t *testing.T) {
	dataRoot := t.TempDir()
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-run-evidence-http")
	runID := common.RunID("run-evidence-http")
	stepID := common.StepID("run-evidence-http/step-1")
	toolCallID := common.ToolCallID("tool-call-evidence-http")

	if err := fileRepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspace.json")).Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "run evidence http",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := fileRepo.NewRunRepository(filepath.Join(dataRoot, "run.json")).Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := fileRepo.NewStepRepository(filepath.Join(dataRoot, "step.json")).Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "validation",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := fileRepo.NewToolCallRepository(filepath.Join(dataRoot, "tool_call.json")).Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		State:       common.ToolCallStateCompleted,
		Output:      map[string]any{"exitCode": 0},
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := fileRepo.NewPatchRepository(filepath.Join(dataRoot, "patch.json")).Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-evidence-http"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := fileRepo.NewValidationResultRepository(filepath.Join(dataRoot, "validation_result.json")).Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-evidence-http"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "go_test",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := fileRepo.NewRuntimeCheckpointRepository(filepath.Join(dataRoot, "runtime_checkpoint.json")).Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-evidence-http"),
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}

	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+string(runID)+"/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body runSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run snapshot: %v", err)
	}
	if len(body.Snapshot.ToolCalls) != 1 || body.Snapshot.ToolCalls[0].ID != string(toolCallID) {
		t.Fatalf("toolCalls = %+v, want %s", body.Snapshot.ToolCalls, toolCallID)
	}
	if len(body.Snapshot.Patches) != 1 || body.Snapshot.Patches[0].ToolCallID != string(toolCallID) {
		t.Fatalf("patches = %+v, want tool call ref %s", body.Snapshot.Patches, toolCallID)
	}
	if len(body.Snapshot.ValidationResults) != 1 || body.Snapshot.ValidationResults[0].State != "passed" {
		t.Fatalf("validationResults = %+v, want passed", body.Snapshot.ValidationResults)
	}
	if len(body.Snapshot.RuntimeCheckpoints) != 1 || body.Snapshot.RuntimeCheckpoints[0].RunStateSnapshot != "completed" {
		t.Fatalf("runtimeCheckpoints = %+v, want completed checkpoint", body.Snapshot.RuntimeCheckpoints)
	}
}

func TestRunStatusEndpointReturnsPersistedRunStatus(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-status-http"

	workspace := createWorkspaceAndRunForHTTP(t, handler, "run status http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID+"/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.ID != runID || body.Status.WorkspaceID != workspace.ID || body.Status.State != "blocked_on_approval" {
		t.Fatalf("status = %+v, want blocked approval status", body.Status)
	}
	if body.Status.StepCount == 0 || body.Status.CurrentStepID == "" || body.Status.CurrentStepState == "" {
		t.Fatalf("status = %+v, want populated step summary", body.Status)
	}
	if body.Status.ApprovalCount != 1 || body.Status.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("status = %+v, want approval count 1 and blocking approval %s", body.Status, started.BlockingApprovalID)
	}
}

func TestWorkspaceSnapshotEndpointReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "workspace snapshot http", "workspaces/workspace-snapshot-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	for _, runID := range []string{
		"run-workspace-snapshot-http-completed",
		"run-" + legacycommon.RuntimeDecisionBlockRunToken + "-workspace-snapshot-http",
		"run-" + legacycommon.RuntimeApprovalBlockRunToken + "-workspace-snapshot-http",
	} {
		runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"`+runID+`","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
		runReq.Header.Set("Content-Type", "application/json")
		runRec := httptest.NewRecorder()
		handler.ServeHTTP(runRec, runReq)
		if runRec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
		}
	}

	completed := startRunForHTTP(t, handler, "run-workspace-snapshot-http-completed")
	if completed.State != "completed" {
		t.Fatalf("completed run = %+v, want completed", completed)
	}
	decisionRun := startRunForHTTP(t, handler, "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-workspace-snapshot-http")
	resolveDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+decisionRun.BlockingTicketID+"/resolve", nil)
	resolveDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveDecisionRec, resolveDecisionReq)
	if resolveDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", resolveDecisionRec.Code, resolveDecisionRec.Body.String())
	}
	resumeDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+"run-"+legacycommon.RuntimeDecisionBlockRunToken+"-workspace-snapshot-http"+"/resume", nil)
	resumeDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeDecisionRec, resumeDecisionReq)
	if resumeDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision run resume status = %d, want 200, body=%s", resumeDecisionRec.Code, resumeDecisionRec.Body.String())
	}
	approvalRun := startRunForHTTP(t, handler, "run-"+legacycommon.RuntimeApprovalBlockRunToken+"-workspace-snapshot-http")

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ID != workspace.ID {
		t.Fatalf("workspace = %+v, want %s", body.Snapshot.Workspace, workspace.ID)
	}
	if body.Snapshot.Workspace.State != "blocked" || body.Snapshot.Workspace.ActiveRunID != approvalRun.ID {
		t.Fatalf("workspace projection = %+v, want blocked active run %s", body.Snapshot.Workspace, approvalRun.ID)
	}
	if len(body.Snapshot.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", body.Snapshot.Runs)
	}
	if len(body.Snapshot.Decisions) != 1 || body.Snapshot.Decisions[0].ID != decisionRun.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", body.Snapshot.Decisions, decisionRun.BlockingTicketID)
	}
	if len(body.Snapshot.Approvals) != 1 || body.Snapshot.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", body.Snapshot.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestRunCreateEndpointReturnsConflictForDuplicateID(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "run dup workspace", "workspaces/run-dup")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	for idx := 0; idx < 2; idx++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-dup","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if idx == 0 && rec.Code != http.StatusCreated {
			t.Fatalf("first run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
		if idx == 1 && rec.Code != http.StatusConflict {
			t.Fatalf("second run create status = %d, want 409, body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestRunCreateEndpointReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-missing/runs", strings.NewReader(`{"runId":"run-missing","goalId":"goal-missing","doDId":"dod-missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-missing", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunStartEndpointReturnsCompletedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "run start http workspace", "workspaces/run-start-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-start-http","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-start-http/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal started run: %v", err)
	}
	if body.Run.ID != "run-start-http" || body.Run.State != "completed" {
		t.Fatalf("started run = %+v, want run-start-http/completed", body.Run)
	}
}

func TestRunStartEndpointReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-http"

	createWorkspaceAndRunForHTTP(t, handler, "run start blocked http workspace", runID)

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal blocked run: %v", err)
	}
	if body.Run.State != "blocked_on_decision" || body.Run.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", body.Run)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestRunResumeEndpointReturnsCompletedRunAfterDecisionResolved(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-resume-http"

	createWorkspaceAndRunForHTTP(t, handler, "run resume decision http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}

	resolveDecisionForHTTP(t, dataRoot, started.BlockingTicketID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if body.Run.State != "completed" || body.Run.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed with cleared ticket", body.Run)
	}
}

func TestRunResumeEndpointReturnsCompletedRunAfterApprovalApproved(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-resume-http"

	createWorkspaceAndRunForHTTP(t, handler, "run resume approval http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed approval run: %v", err)
	}
	if body.Run.State != "completed" || body.Run.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want completed with cleared approval", body.Run)
	}
}

func TestRunResumeEndpointReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-rejected-http"

	createWorkspaceAndRunForHTTP(t, handler, "run resume approval rejected http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, true)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed rejected approval run: %v", err)
	}
	if body.Run.State != "failed" || body.Run.BlockingApprovalID != "" || body.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with cleared approval and reason", body.Run)
	}
}

func TestRunResumeEndpointReturnsConflictWhenDependencyStillBlocked(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-pending-http"

	createWorkspaceAndRunForHTTP(t, handler, "run resume pending http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestRunResumeEndpointReturnsConflictForCreatedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	createWorkspaceAndRunForHTTP(t, handler, "run resume created http workspace", "run-resume-created-http")

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-resume-created-http/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestDecisionListAndGetEndpointsReturnPersistedTickets(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runA := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-list-a"
	runB := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-list-b"

	createWorkspaceAndRunForHTTP(t, handler, "decision list http workspace a", runA)
	startedA := startRunForHTTP(t, handler, runA)
	workspaceB := createWorkspaceAndRunForHTTP(t, handler, "decision list http workspace b", runB)
	startedB := startRunForHTTP(t, handler, runB)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("decision list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed decisionsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal decision list: %v", err)
	}
	if len(listed.Decisions) != 2 || listed.Decisions[0].ID != startedA.BlockingTicketID || listed.Decisions[1].ID != startedB.BlockingTicketID {
		t.Fatalf("listed decisions = %+v, want sorted tickets", listed.Decisions)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions?workspaceId="+workspaceB.ID, nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered decision list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered decisionsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered decision list: %v", err)
	}
	if len(filtered.Decisions) != 1 || filtered.Decisions[0].ID != startedB.BlockingTicketID {
		t.Fatalf("filtered decisions = %+v, want ticket %s", filtered.Decisions, startedB.BlockingTicketID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions/"+startedA.BlockingTicketID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("decision get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched decisionResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal decision get: %v", err)
	}
	if fetched.Decision.ID != startedA.BlockingTicketID || fetched.Decision.State != "open" {
		t.Fatalf("decision = %+v, want open ticket %s", fetched.Decision, startedA.BlockingTicketID)
	}
}

func TestDecisionResolveEndpointResolvesTicketAndAllowsRunResume(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-resolve-http"

	createWorkspaceAndRunForHTTP(t, handler, "decision resolve http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
	var resolved decisionResponse
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal decision resolve: %v", err)
	}
	if resolved.Decision.State != "resolved" || resolved.Decision.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", resolved.Decision)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if resumed.Run.State != "completed" || resumed.Run.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed with cleared ticket", resumed.Run)
	}
}

func TestDecisionResolveEndpointRejectCurrentPathAndRunResumeFailsTerminally(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-reject-http"

	createWorkspaceAndRunForHTTP(t, handler, "decision reject http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectBody := strings.NewReader(`{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"tester"}}`)
	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", rejectBody)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var rejected decisionResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("unmarshal decision resolve: %v", err)
	}
	if rejected.Decision.State != "resolved" || rejected.Decision.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", rejected.Decision)
	}
	if rejected.Decision.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("resolution payload = %+v, want operator tester", rejected.Decision.ResolutionPayload)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed rejected run: %v", err)
	}
	if resumed.Run.State != "failed" || resumed.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed.Run)
	}
}

func TestDecisionResolveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-conflict-http"

	createWorkspaceAndRunForHTTP(t, handler, "decision conflict http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	rejectDecisionViaHTTP(t, handler, started.BlockingTicketID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusConflict {
		t.Fatalf("decision resolve status = %d, want 409, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
}

func TestApprovalListAndGetEndpointsReturnPersistedApprovals(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runA := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-list-a"
	runB := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-list-b"

	createWorkspaceAndRunForHTTP(t, handler, "approval list http workspace a", runA)
	startedA := startRunForHTTP(t, handler, runA)
	workspaceB := createWorkspaceAndRunForHTTP(t, handler, "approval list http workspace b", runB)
	startedB := startRunForHTTP(t, handler, runB)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("approval list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed approvalsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal approval list: %v", err)
	}
	if len(listed.Approvals) != 2 {
		t.Fatalf("len(approvals) = %d, want 2", len(listed.Approvals))
	}
	foundA := false
	foundB := false
	for _, item := range listed.Approvals {
		if item.ID == startedA.BlockingApprovalID {
			foundA = true
		}
		if item.ID == startedB.BlockingApprovalID {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("listed approvals = %+v, want approvals %s and %s", listed.Approvals, startedA.BlockingApprovalID, startedB.BlockingApprovalID)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals?workspaceId="+workspaceB.ID, nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered approval list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered approvalsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered approval list: %v", err)
	}
	if len(filtered.Approvals) != 1 || filtered.Approvals[0].ID != startedB.BlockingApprovalID {
		t.Fatalf("filtered approvals = %+v, want approval %s", filtered.Approvals, startedB.BlockingApprovalID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals/"+startedA.BlockingApprovalID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("approval get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched approvalResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal approval get: %v", err)
	}
	if fetched.Approval.ID != startedA.BlockingApprovalID || fetched.Approval.State != "pending" {
		t.Fatalf("approval = %+v, want pending approval %s", fetched.Approval, startedA.BlockingApprovalID)
	}
}

func TestApprovalApproveEndpointApprovesApprovalAndAllowsRunResume(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-approve-http"

	createWorkspaceAndRunForHTTP(t, handler, "approval approve http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", nil)
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approval approve status = %d, want 200, body=%s", approveRec.Code, approveRec.Body.String())
	}
	var approved approvalResponse
	if err := json.Unmarshal(approveRec.Body.Bytes(), &approved); err != nil {
		t.Fatalf("unmarshal approval approve: %v", err)
	}
	if approved.Approval.State != "approved" {
		t.Fatalf("approval = %+v, want approved", approved.Approval)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed approval run: %v", err)
	}
	if resumed.Run.State != "completed" || resumed.Run.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want completed with cleared approval", resumed.Run)
	}
}

func TestApprovalRejectEndpointRejectsApprovalAndRunResumeFailsTerminally(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-reject-http"

	createWorkspaceAndRunForHTTP(t, handler, "approval reject http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("approval reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var rejected approvalResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("unmarshal approval reject: %v", err)
	}
	if rejected.Approval.State != "rejected" {
		t.Fatalf("approval = %+v, want rejected", rejected.Approval)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed rejected approval run: %v", err)
	}
	if resumed.Run.State != "failed" || resumed.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed.Run)
	}
}

func TestApprovalApproveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-conflict-http"

	createWorkspaceAndRunForHTTP(t, handler, "approval conflict http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("approval reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", nil)
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusConflict {
		t.Fatalf("approval approve status = %d, want 409, body=%s", approveRec.Code, approveRec.Body.String())
	}
}

func TestMetricsEndpointReportsCounters(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	workspace := createWorkspaceForHTTP(t, handler, "metrics workspace", "workspaces/metrics")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)
	runCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-metrics","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runCreateReq.Header.Set("Content-Type", "application/json")
	runCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(runCreateRec, runCreateReq)
	if runCreateRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201", runCreateRec.Code)
	}
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-metrics/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("run start status = %d, want 200", startRec.Code)
	}

	blockedWorkspace := createWorkspaceForHTTP(t, handler, "metrics blocked workspace", "workspaces/metrics-blocked")
	blockedGoalID, blockedDoDID := defineGoalAndDoDForHTTP(t, handler, blockedWorkspace.ID)
	blockedRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-metrics"
	blockedRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+blockedWorkspace.ID+"/runs", strings.NewReader(`{"runId":"`+blockedRunID+`","goalId":"`+blockedGoalID+`","doDId":"`+blockedDoDID+`"}`))
	blockedRunReq.Header.Set("Content-Type", "application/json")
	blockedRunRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRunRec, blockedRunReq)
	if blockedRunRec.Code != http.StatusCreated {
		t.Fatalf("blocked run create status = %d, want 201", blockedRunRec.Code)
	}
	blockedStartReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+blockedRunID+"/start", nil)
	blockedStartRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedStartRec, blockedStartReq)
	if blockedStartRec.Code != http.StatusOK {
		t.Fatalf("blocked run start status = %d, want 200", blockedStartRec.Code)
	}
	var blockedBody runInstanceResponse
	if err := json.Unmarshal(blockedStartRec.Body.Bytes(), &blockedBody); err != nil {
		t.Fatalf("unmarshal blocked metrics run: %v", err)
	}
	resolveDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+blockedBody.Run.BlockingTicketID+"/resolve", nil)
	resolveDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveDecisionRec, resolveDecisionReq)
	if resolveDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200", resolveDecisionRec.Code)
	}
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+blockedRunID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200", resumeRec.Code)
	}

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/workorch/workspaces", "/api/v1/workorch/workspaces/" + workspace.ID, "/api/v1/workorch/runs", "/api/v1/workorch/runs/run-metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}

	for _, path := range []string{
		"/api/v1/workorch/workspaces/" + workspace.ID + "/snapshot",
		"/api/v1/workorch/runs/run-metrics/snapshot",
		"/api/v1/workorch/runs/run-metrics/status",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", metricsRec.Code)
	}
	if contentType := metricsRec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", contentType)
	}

	body := metricsRec.Body.String()
	for _, want := range []string{
		`workorch_http_requests_total{endpoint="healthz"} 1`,
		`workorch_http_requests_total{endpoint="readyz"} 1`,
		`workorch_http_requests_total{endpoint="metrics"} 1`,
		`workorch_http_requests_total{endpoint="workspace_create"} 2`,
		`workorch_http_requests_total{endpoint="workspace_list"} 1`,
		`workorch_http_requests_total{endpoint="workspace_get"} 1`,
		`workorch_http_requests_total{endpoint="workspace_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_create"} 2`,
		`workorch_http_requests_total{endpoint="run_list"} 1`,
		`workorch_http_requests_total{endpoint="run_get"} 1`,
		`workorch_http_requests_total{endpoint="run_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_status"} 1`,
		`workorch_http_requests_total{endpoint="run_start"} 2`,
		`workorch_http_requests_total{endpoint="run_resume"} 1`,
		`workorch_http_requests_total{endpoint="decision_list"} 0`,
		`workorch_http_requests_total{endpoint="decision_get"} 0`,
		`workorch_http_requests_total{endpoint="decision_resolve"} 1`,
		`workorch_http_requests_total{endpoint="approval_list"} 0`,
		`workorch_http_requests_total{endpoint="approval_get"} 0`,
		`workorch_http_requests_total{endpoint="approval_approve"} 0`,
		`workorch_http_requests_total{endpoint="approval_reject"} 0`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_queue_total 0`,
		`workorch_run_outcomes_total{status="succeeded"} 3`,
		`workorch_run_outcomes_total{status="canceled"} 0`,
		`workorch_server_max_concurrent_runs 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

func createWorkspaceForHTTP(t *testing.T, handler http.Handler, title string, workingDirectory string) appservice.Workspace {
	t.Helper()

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"`+title+`","workingDirectory":"`+workingDirectory+`"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}
	var body workspaceResponse
	if err := json.Unmarshal(workspaceRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace create response: %v", err)
	}
	if body.Workspace.ID == "" || body.Workspace.Title != title || body.Workspace.WorkingDirectory != workingDirectory || body.Workspace.State != "active" {
		t.Fatalf("workspace = %+v, want generated/%s/%s/active", body.Workspace, title, workingDirectory)
	}
	return body.Workspace
}

func postAndExpect(t *testing.T, handler http.Handler, path string, body string, wantStatus int) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d, body = %s", path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec.Body.String()
}

func createWorkspaceAndRunForHTTP(t *testing.T, handler http.Handler, title string, runID string) appservice.Workspace {
	t.Helper()

	workspace := createWorkspaceForHTTP(t, handler, title, "workspaces/"+runID)
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"`+runID+`","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}
	return workspace
}

func defineGoalAndDoDForHTTP(t *testing.T, handler http.Handler, workspaceID string) (string, string) {
	t.Helper()

	goalReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/goals", strings.NewReader(`{"text":"verify run goal"}`))
	goalReq.Header.Set("Content-Type", "application/json")
	goalRec := httptest.NewRecorder()
	handler.ServeHTTP(goalRec, goalReq)
	if goalRec.Code != http.StatusCreated {
		t.Fatalf("goal define status = %d, want 201, body=%s", goalRec.Code, goalRec.Body.String())
	}
	var goalBody goalResponse
	if err := json.Unmarshal(goalRec.Body.Bytes(), &goalBody); err != nil {
		t.Fatalf("unmarshal goal response: %v", err)
	}

	dodReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/dods", strings.NewReader(`{"items":["verify run completed"]}`))
	dodReq.Header.Set("Content-Type", "application/json")
	dodRec := httptest.NewRecorder()
	handler.ServeHTTP(dodRec, dodReq)
	if dodRec.Code != http.StatusCreated {
		t.Fatalf("dod define status = %d, want 201, body=%s", dodRec.Code, dodRec.Body.String())
	}
	var dodBody dodResponse
	if err := json.Unmarshal(dodRec.Body.Bytes(), &dodBody); err != nil {
		t.Fatalf("unmarshal dod response: %v", err)
	}
	return goalBody.Goal.ID, dodBody.DoD.ID
}

func startRunForHTTP(t *testing.T, handler http.Handler, runID string) appservice.Run {
	t.Helper()

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal started run: %v", err)
	}
	return body.Run
}

func resolveDecisionForHTTP(t *testing.T, dataRoot string, ticketID string, reject bool) {
	t.Helper()

	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("close runtime session: %v", err)
		}
	}()

	resolutionType := decisioncommon.ResolutionTypeProvideMissingInfo
	resolutionID := "resolution-" + ticketID
	if reject {
		resolutionType = decisioncommon.ResolutionTypeRejectCurrentPath
		resolutionID += "-reject"
	}
	if err := session.Bootstrap.Exports().Services.DecisionService.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: common.ResolutionID(resolutionID),
		TicketID:     common.TicketID(ticketID),
		Type:         resolutionType,
	}); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}
}

func rejectDecisionViaHTTP(t *testing.T, handler http.Handler, ticketID string) appservice.Decision {
	t.Helper()

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+ticketID+"/resolve", strings.NewReader(`{"resolutionType":"reject_current_path"}`))
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("decision reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var body decisionResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal decision reject response: %v", err)
	}
	return body.Decision
}

func resolveApprovalForHTTP(t *testing.T, dataRoot string, approvalID string, reject bool) {
	t.Helper()

	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("close runtime session: %v", err)
		}
	}()

	cmd := approvalmodule.ResolveApprovalCommand{ApprovalID: common.ApprovalID(approvalID)}
	if reject {
		if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureRejected(ctx, cmd); err != nil {
			t.Fatalf("EnsureRejected: %v", err)
		}
		return
	}
	if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureApproved(ctx, cmd); err != nil {
		t.Fatalf("EnsureApproved: %v", err)
	}
}

func TestRunStartEndpointWaitsWhenConcurrentRunLimitReached(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	state.validate = func(context.Context, string, string) error { return nil }
	state.startRun = func(_ context.Context, _, _, runID string) (appservice.Run, error) {
		started <- struct{}{}
		<-release
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/runs/run-held/start", nil)
		if err != nil {
			t.Errorf("new request: %v", err)
			close(done)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("first run request: %v", err)
			close(done)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("first run status = %d, want 200", resp.StatusCode)
		}
		close(done)
	}()

	<-started

	go func() {
		secondResp, err := http.Post(server.URL+"/api/v1/workorch/runs/run-second/start", "application/json", nil)
		if err != nil {
			t.Errorf("second run request: %v", err)
			secondDone <- 0
			return
		}
		defer secondResp.Body.Close()
		secondDone <- secondResp.StatusCode
	}()

	select {
	case status := <-secondDone:
		t.Fatalf("second run completed before slot released, status = %d", status)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-done
	secondStatus := <-secondDone
	if secondStatus != http.StatusOK {
		t.Fatalf("second run status = %d, want 200", secondStatus)
	}

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_run_queue_total 1`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_outcomes_total{status="succeeded"} 2`,
		`workorch_run_outcomes_total{status="rejected"} 0`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestRunStartEndpointWaitsForAvailableExecutionSlot(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	state.validate = func(context.Context, string, string) error { return nil }
	state.startRun = func(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
		if runID == "run-1" {
			firstStarted <- struct{}{}
			<-release
			return appservice.Run{ID: runID, State: "completed"}, nil
		}
		secondStarted <- struct{}{}
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/runs/run-1/start", nil)
		if err != nil {
			t.Errorf("new first start request: %v", err)
			close(firstDone)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("first start request: %v", err)
			close(firstDone)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("first start status = %d, want 200", resp.StatusCode)
		}
		close(firstDone)
	}()

	<-firstStarted

	go func() {
		resp, err := http.Post(server.URL+"/api/v1/workorch/runs/run-2/start", "application/json", nil)
		if err != nil {
			t.Errorf("second start request: %v", err)
			secondDone <- 0
			return
		}
		defer resp.Body.Close()
		secondDone <- resp.StatusCode
	}()

	select {
	case <-secondStarted:
		t.Fatal("second start reached executor before slot released")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-firstDone

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second start did not begin after slot released")
	}

	secondStatus := <-secondDone
	if secondStatus != http.StatusOK {
		t.Fatalf("second start status = %d, want 200", secondStatus)
	}

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_http_requests_total{endpoint="run_start"} 2`,
		`workorch_run_queue_total 1`,
		`workorch_run_outcomes_total{status="succeeded"} 2`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestRunResumeEndpointReturnsRequestTimeoutWhenQueueWaitCanceled(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	state.validate = func(context.Context, string, string) error { return nil }
	state.resumeRun = func(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
		if runID == "run-held" {
			started <- struct{}{}
			<-release
			return appservice.Run{ID: runID, State: "completed"}, nil
		}
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	handler := newMuxWithState(state)

	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-held/resume", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("held resume status = %d, want 200", rec.Code)
		}
		close(done)
	}()

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-queued/resume", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("queued resume status = %d, want 408, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "waiting for available execution slot") {
		t.Fatalf("queued resume body = %s, want queue wait message", rec.Body.String())
	}

	close(release)
	<-done

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_run_queue_total 1`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_outcomes_total{status="canceled"} 1`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestConfigFromValuesReturnsErrorForInvalidDuration(t *testing.T) {
	_, err := ConfigFromValues(map[string]string{
		configServerReadTimeout: "definitely-not-a-duration",
	})
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), configServerReadTimeout) {
		t.Fatalf("error = %v, want config key %s", err, configServerReadTimeout)
	}
}

func TestConfigFromValuesReturnsErrorForInvalidConcurrentLimit(t *testing.T) {
	_, err := ConfigFromValues(map[string]string{
		configServerMaxConcurrentRuns: "0",
	})
	if err == nil {
		t.Fatal("expected invalid concurrent limit error")
	}
	if !strings.Contains(err.Error(), configServerMaxConcurrentRuns) {
		t.Fatalf("error = %v, want config key %s", err, configServerMaxConcurrentRuns)
	}
}

func stringSliceEqual(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for idx := range got {
		if got[idx] != want[idx] {
			return false
		}
	}
	return true
}
