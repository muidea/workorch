package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	plugininitiator "github.com/muidea/magicCommon/framework/plugin/initiator"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	magictask "github.com/muidea/magicCommon/task"
	engine "github.com/muidea/magicEngine/http"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	appbootstrap "github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/blocks/llm"
	_ "github.com/muidea/workorch/internal/modules/blocks/mcp"
	_ "github.com/muidea/workorch/internal/modules/blocks/shell"
	_ "github.com/muidea/workorch/internal/modules/kernel/mcp"
	_ "github.com/muidea/workorch/internal/modules/kernel/memory"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/run"
	_ "github.com/muidea/workorch/internal/modules/kernel/skill"
	_ "github.com/muidea/workorch/internal/modules/kernel/soul"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition was not met within %v", timeout)
}

func newFileMuxWithCleanup(t *testing.T, dataRoot string) http.Handler {
	t.Helper()
	state := newServerState(normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	return newMuxWithState(state)
}

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

func TestServerStatePrefersApplicationRuntimeSession(t *testing.T) {
	useWorkorchRuntimeTestConfig(t, "memory", "")
	hub := magicevent.NewHub(16)
	routine := magictask.NewBackgroundRoutine(16)
	defer routine.Shutdown(time.Second)
	defer hub.Terminate()
	defer plugininitiator.Teardown()
	defer pluginmodule.Teardown()

	if err := plugininitiator.Setup(hub, routine); err != nil {
		t.Fatalf("setup runtime initiator: %v", err)
	}
	if err := pluginmodule.Setup(hub, routine); err != nil {
		t.Fatalf("setup runtime modules: %v", err)
	}

	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	if state.sharedSession == nil {
		t.Fatal("expected shared runtime session")
	}
	if state.sharedSession.Bootstrap != nil {
		t.Fatal("expected application runtime session to be non-owning")
	}
	if state.sharedSession.Runtime == nil {
		t.Fatal("expected managed runtime adapter")
	}

	ctx := context.Background()
	workspace, err := state.createWorkspace(ctx, "", "", appservice.CreateWorkspaceCommand{
		Title:                "application runtime",
		DeliverableDirectory: "workspaces/application-runtime",
	})
	if err != nil {
		t.Fatalf("create workspace through server state: %v", err)
	}

	var helper runtimecommon.Helper
	runtime, runtimeErr := plugininitiator.GetEntity(runtimecommon.InitiatorID, helper)
	if runtimeErr != nil {
		t.Fatalf("get runtime initiator: %v", runtimeErr)
	}
	if _, err := runtime.Repositories().Workspaces.Get(ctx, common.WorkspaceID(workspace.ID)); err != nil {
		t.Fatalf("workspace was not persisted in application runtime repositories: %v", err)
	}

	if err := state.close(ctx); err != nil {
		t.Fatalf("close server state: %v", err)
	}
	if _, err := runtime.Repositories().Workspaces.Get(ctx, common.WorkspaceID(workspace.ID)); err != nil {
		t.Fatalf("server state close must not teardown application runtime: %v", err)
	}
}

func TestConfigurationEndpointReturnsRedactedLLMProviders(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.readConfiguration = func() (appservice.ServiceConfiguration, error) {
		return appservice.ServiceConfiguration{
			Server:     appservice.ServerConfiguration{Addr: ":8080", MaxConcurrentRuns: 1},
			Repository: appservice.RepositoryConfiguration{Backend: "file", DataDir: ".workorch-data"},
			Runtime:    appservice.RuntimeConfiguration{DefaultSoulID: "soul-system"},
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
		body.Configuration.Runtime.DefaultSoulID != "soul-system" ||
		len(body.Configuration.LLM.Providers) != 1 ||
		!body.Configuration.LLM.Providers[0].APIKeyConfigured ||
		strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("body = %s, want redacted configuration response", rec.Body.String())
	}
}

func TestUpdateLLMConfigurationEndpointReturnsRedactedConfiguration(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.updateLLMConfiguration = func(cmd appservice.UpdateLLMConfigurationCommand) (appservice.ServiceConfiguration, error) {
		if cmd.DefaultProviderID == nil || *cmd.DefaultProviderID != "local" ||
			cmd.Providers == nil || len(*cmd.Providers) != 1 ||
			(*cmd.Providers)[0].APIKey == nil || *(*cmd.Providers)[0].APIKey != "secret-value" {
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
	if !strings.Contains(statusRec.Body.String(), `"state": "blocked_on_decision"`) ||
		!strings.Contains(statusRec.Body.String(), `"blockingTicketId": "run-memory-http-step-1-provider-retry-exhausted-decision"`) {
		t.Fatalf("run status body = %s, want provider decision blocker without configured LLM provider", statusRec.Body.String())
	}

	observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-memory-http/observability", nil)
	observabilityRec := httptest.NewRecorder()
	mux.ServeHTTP(observabilityRec, observabilityReq)
	if observabilityRec.Code != http.StatusOK {
		t.Fatalf("run observability code = %d, want 200, body = %s", observabilityRec.Code, observabilityRec.Body.String())
	}
	if !strings.Contains(observabilityRec.Body.String(), `"terminal": false`) || !strings.Contains(observabilityRec.Body.String(), `"runtimeCheckpointTotal"`) {
		t.Fatalf("run observability body = %s, want non-terminal provider blocker and evidence counters", observabilityRec.Body.String())
	}
}

func TestContextRegistryAPIsPersistAcrossMemoryHTTPRequests(t *testing.T) {
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "context registry http", "workspaces/ws-context-registry")

	postAndExpect(t, mux, "/api/v1/workorch/souls", `{
		"id":"soul-engineering-delivery",
		"name":"engineering delivery",
		"soulMd":"# Engineering Delivery\n\nPlan and cite evidence.",
		"allowedSkillRefs":["skill-local-go-service"],
		"allowedMCPServerRefs":["mcp-local-context"],
		"allowedToolKinds":["shell/command"]
	}`, http.StatusCreated)
	putAndExpect(t, mux, "/api/v1/workorch/souls/soul-engineering-delivery", `{
		"name":"engineering delivery updated",
		"description":"updated through HTTP API",
		"businessDomains":["engineering"],
		"soulMd":"# Engineering Delivery\n\nPlan, cite evidence, and keep capability boundaries explicit.",
		"allowedSkillRefs":["skill-local-go-service"],
		"allowedMCPServerRefs":["mcp-local-context"],
		"allowedToolKinds":["shell/command"]
	}`, http.StatusOK)
	postAndExpect(t, mux, "/api/v1/workorch/souls", `{
		"id":"soul-delete-http",
		"name":"delete through HTTP",
		"soulMd":"# Delete Through HTTP\n\nTemporary soul for DELETE coverage."
	}`, http.StatusCreated)
	deleteAndExpect(t, mux, "/api/v1/workorch/souls/soul-delete-http", http.StatusNoContent)
	postAndExpect(t, mux, "/api/v1/workorch/workspace-templates", `{
		"id":"workspace-template-delivery",
		"name":"delivery workspace template",
		"description":"reuse goal, dod and soul bindings",
		"goalText":"Deliver a verified local service",
		"doDItems":["Goal is implemented","Evidence is available"],
		"defaultSoulId":"soul-engineering-delivery",
		"soulBindings":{"planning":"soul-engineering-delivery","execution":"soul-engineering-delivery"}
	}`, http.StatusCreated)
	putAndExpect(t, mux, "/api/v1/workorch/workspace-templates/workspace-template-delivery", `{
		"name":"delivery workspace template updated",
		"goalText":"Deliver an updated verified local service",
		"doDItems":["Goal is implemented","Evidence is available","Result is accepted"],
		"defaultSoulId":"soul-engineering-delivery",
		"soulBindings":{"planning":"soul-engineering-delivery","validation":"soul-engineering-delivery"}
	}`, http.StatusOK)
	postAndExpect(t, mux, "/api/v1/workorch/workspace-templates", `{
		"id":"workspace-template-delete-http",
		"name":"delete workspace template",
		"goalText":"Delete this template after creation",
		"doDItems":["Template can be deleted"],
		"defaultSoulId":"soul-engineering-delivery"
	}`, http.StatusCreated)
	deleteAndExpect(t, mux, "/api/v1/workorch/workspace-templates/workspace-template-delete-http", http.StatusNoContent)
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
			"allowedTools":["repo.summary"],
			"allowedResources":["file://README.md"],
			"allowedPrompts":["delivery.plan"]
		}`, http.StatusCreated)
	putAndExpect(t, mux, "/api/v1/workorch/mcp/servers/mcp-local-context", `{
			"name":"local context updated",
			"transport":"stdio",
			"endpoint":".workorch-data/system/mcp-endpoints/mcp-local-context/context-helper.py",
			"enabled":true,
			"allowedTools":["repo.summary"],
			"allowedResources":["file://README.md"],
			"allowedPrompts":["delivery.plan"],
			"sessionPolicy":"transient"
		}`, http.StatusOK)
	postAndExpect(t, mux, "/api/v1/workorch/mcp/capabilities", `{
		"id":"mcp-local-context:tool:repo.details",
		"serverId":"mcp-local-context",
		"kind":"tool",
		"name":"repo.details",
		"displayName":"仓库详情",
		"description":"获取代码仓库更详细的上下文信息。",
		"category":"代码仓库",
		"tags":["repo","details"],
		"stageHints":["planning","execution"],
		"riskLevel":"read_only",
		"enabled":true
	}`, http.StatusCreated)
	postAndExpect(t, mux, "/api/v1/workorch/workspaces/"+workspace.ID+"/memory", `{
		"sourceType":"user",
		"memoryType":"project_context",
		"summary":"This workspace is verified through local workorchd HTTP APIs.",
		"tags":["local","acceptance"],
		"embeddingProviderId":"verify-embedding",
		"embeddingModel":"verify-embedding-model",
		"embeddingText":"local workorchd HTTP API verification",
		"vector":[0.9,0.1,0]
	}`, http.StatusCreated)

	for _, spec := range []struct {
		path       string
		want       string
		wantAbsent string
	}{
		{path: "/api/v1/workorch/souls", want: `"id": "soul-engineering-delivery"`},
		{path: "/api/v1/workorch/souls/soul-engineering-delivery", want: `"name": "engineering delivery updated"`},
		{path: "/api/v1/workorch/souls/soul-delete-http", want: `"error": "not found"`},
		{path: "/api/v1/workorch/workspace-templates", want: `"id": "workspace-template-delivery"`},
		{path: "/api/v1/workorch/workspace-templates", want: `"id": "workspace-template-delivery"`, wantAbsent: `"workspace-template-delete-http"`},
		{path: "/api/v1/workorch/skills", want: `"id": "skill-local-go-service"`},
		{path: "/api/v1/workorch/skills/skill-local-go-service", want: `"requiredMCPServerRefs": [`},
		{path: "/api/v1/workorch/mcp/servers", want: `".workorch-data/system/mcp-endpoints/mcp-local-context/context-helper.py"`},
		{path: "/api/v1/workorch/mcp/capabilities?kind=tool&q=%E4%BB%93%E5%BA%93", want: `"displayName": "仓库详情"`},
		{path: "/api/v1/workorch/mcp/capabilities/mcp-local-context:tool:repo.details", want: `"category": "代码仓库"`},
		{path: "/api/v1/workorch/mcp/servers/mcp-local-context/tools", want: `"name": "repo.summary"`},
		{path: "/api/v1/workorch/mcp/servers/mcp-local-context/resources", want: `"uri": "file://README.md"`},
		{path: "/api/v1/workorch/mcp/servers/mcp-local-context/prompts", want: `"name": "delivery.plan"`},
		{path: "/api/v1/workorch/memory?workspaceId=" + workspace.ID + "&q=HTTP", want: `"vectorReady": true`},
	} {
		req := httptest.NewRequest(http.MethodGet, spec.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		wantStatus := http.StatusOK
		if strings.Contains(spec.want, `"error": "not found"`) {
			wantStatus = http.StatusNotFound
		}
		if rec.Code != wantStatus {
			t.Fatalf("%s status = %d, want %d, body = %s", spec.path, rec.Code, wantStatus, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), spec.want) {
			t.Fatalf("%s body = %s, want %q", spec.path, rec.Body.String(), spec.want)
		}
		if spec.wantAbsent != "" && strings.Contains(rec.Body.String(), spec.wantAbsent) {
			t.Fatalf("%s body = %s, want to not contain %q", spec.path, rec.Body.String(), spec.wantAbsent)
		}
	}
}

func TestWorkspaceExecutionActivationEndpointCreatesAndStartsRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "activation http", "workspaces/ws-activation-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, mux, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{"priority":7}`))
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
		body.Activation.Run.Priority != 7 ||
		body.Activation.Run.State != "blocked_on_decision" ||
		body.Activation.Run.BlockingTicketID == "" {
		t.Fatalf("activation run = %+v, want generated provider-blocked llm run with definitions", body.Activation.Run)
	}
	if body.Activation.Run.FailureReason != "" {
		t.Fatalf("activation failure reason = %q, want provider blocker without business failure reason", body.Activation.Run.FailureReason)
	}
	if body.Activation.Run.ExecutionContext[llmcommon.ExecutionContextLLMProviderID] != nil {
		t.Fatalf("executionContext = %+v, want no persisted llm provider binding", body.Activation.Run.ExecutionContext)
	}
	if len(body.Activation.Snapshot.Runs) != 1 || body.Activation.Snapshot.Runs[0].ID != body.Activation.Run.ID {
		t.Fatalf("activation snapshot runs = %+v, want created run", body.Activation.Snapshot.Runs)
	}
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	mux.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", metricsRec.Code)
	}
	for _, want := range []string{
		`workorch_run_outcomes_total{status="succeeded"} 0`,
		`workorch_run_outcomes_total{status="failed"} 0`,
	} {
		if !strings.Contains(metricsRec.Body.String(), want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metricsRec.Body.String())
		}
	}
}

func TestWorkspaceExecutionActivationSchedulesReplanContinuationForSupersededRun(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	handler := newMuxWithState(state)
	continuation := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-http-replan",
				WorkspaceID:    "ws-http-replan",
				RunID:          "run-http-replan",
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	var calls atomic.Int32
	state.activateWorkspace = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if workspaceID != "ws-http-replan" {
			t.Fatalf("workspaceID = %q, want ws-http-replan", workspaceID)
		}
		if calls.Add(1) == 1 {
			return appservice.WorkspaceExecutionActivation{
				Run: appservice.Run{ID: "run-http-replan", WorkspaceID: workspaceID, State: string(common.RunStateSuperseded), Priority: cmd.Priority},
			}, nil
		}
		continuation <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-http-replan-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning), Priority: cmd.Priority},
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-http-replan/execution-activations", strings.NewReader(`{"goalId":"goal-http-replan","doDId":"dod-http-replan","priority":6}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activation status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var body workspaceExecutionActivationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal activation: %v", err)
	}
	if body.Activation.Run.ID != "run-http-replan" || body.Activation.Run.State != string(common.RunStateSuperseded) {
		t.Fatalf("activation run = %+v, want superseded replan run", body.Activation.Run)
	}
	select {
	case cmd := <-continuation:
		if cmd.Priority != 6 {
			t.Fatalf("continuation priority = %d, want 6", cmd.Priority)
		}
	case <-time.After(time.Second):
		t.Fatalf("activation endpoint did not schedule replan continuation")
	}
}

func TestWorkspaceExecutionActivationReportsDisallowedSoulCapability(t *testing.T) {
	useUnavailableLLMProvider(t)
	mux := newMux(Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, mux, "activation soul capability", "workspaces/ws-activation-soul-capability")
	defineGoalAndDoDForHTTP(t, mux, workspace.ID)
	postAndExpect(t, mux, "/api/v1/workorch/souls", `{
		"id":"soul-restricted",
		"name":"restricted soul",
		"soulMd":"# Restricted Soul\n\nOnly use explicitly allowed capabilities."
	}`, http.StatusCreated)
	postAndExpect(t, mux, "/api/v1/workorch/skills", `{
		"id":"skill-denied",
		"name":"denied skill",
		"version":"v1",
		"instruction":"This skill is intentionally not allowed by the selected soul."
	}`, http.StatusCreated)

	body := postAndExpect(t, mux, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", `{
		"soulId":"soul-restricted",
		"skillIds":["skill-denied"]
	}`, http.StatusBadRequest)
	for _, want := range []string{"invalid argument", "soul-restricted", "skill-denied"} {
		if !strings.Contains(body, want) {
			t.Fatalf("activation capability error body = %s, want %q", body, want)
		}
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

func useWorkorchRuntimeTestConfig(t *testing.T, backend string, dataRoot string) {
	t.Helper()

	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

[server]
addr = "127.0.0.1:0"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = %q
dataDir = %q

[llm]
timeout = "60s"
disabled = false
`, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write runtime test config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
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
		{method: http.MethodDelete, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/snapshot"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/task-progress"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/context-bundle"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/task-decompositions"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory-recalls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/goals/:goalId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/goals/:goalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/dods/:dodId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/dods/:dodId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-activations"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/snapshot"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/steps"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/tool-calls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/runtime-checkpoints"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/results"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/patches"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/validation-results"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/artifacts"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/status"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/observability"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/memory-recalls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/task-progress"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/context-bundle"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs/:runId/task-decompositions"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/runs/:runId/start"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/runs/:runId/resume"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runtime/payloads"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/events/stream"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/decisions"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/decisions/:ticketId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/decisions/:ticketId/resolve"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/decisions/resolve-batch"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/approvals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/approvals/:approvalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/approvals/:approvalId/approve"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/approvals/:approvalId/reject"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/approvals/resolve-batch"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/souls"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/souls/:soulId"},
		{method: http.MethodDelete, pattern: "/api/v1/workorch/souls/:soulId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/souls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/souls/:soulId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspace-templates"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspace-templates/:templateId"},
		{method: http.MethodDelete, pattern: "/api/v1/workorch/workspace-templates/:templateId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspace-templates"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/skills"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/skills"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/skills/:skillId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/mcp/servers"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/mcp/servers/:serverId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/mcp/capabilities"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/capabilities"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/capabilities/:capabilityId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/mcp/capabilities/:capabilityId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId/tools"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId/resources"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/mcp/servers/:serverId/prompts"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/memory"},
		{method: http.MethodGet, pattern: "/favicon.ico"},
		{method: http.MethodGet, pattern: "/console"},
		{method: http.MethodGet, pattern: "/console/"},
		{method: http.MethodGet, pattern: "/console/**"},
	}
}

func TestMemoryRecallTraceEndpointsReturnFilteredTraces(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	state.listMemoryRecallTraces = func(_ context.Context, _, _ string, query appservice.MemoryRecallTraceQuery) ([]appservice.MemoryRecallTrace, error) {
		if query.RunID != "run-recall-http" && query.WorkspaceID != "ws-recall-http" {
			t.Fatalf("memory recall query = %+v, want run or workspace filter", query)
		}
		return []appservice.MemoryRecallTrace{
			{
				ID:                 "run-recall-http/memory-recall/activation",
				WorkspaceID:        "ws-recall-http",
				RunID:              "run-recall-http",
				Stage:              "planning",
				Source:             "activation",
				Status:             "selected",
				RetrievedMemoryIDs: []string{"memory-recall-http"},
			},
		}, nil
	}
	handler := newMuxWithState(state)

	for _, path := range []string{
		"/api/v1/workorch/runs/run-recall-http/memory-recalls",
		"/api/v1/workorch/workspaces/ws-recall-http/memory-recalls",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", path, rec.Code, rec.Body.String())
		}
		var body memoryRecallTracesResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s response: %v", path, err)
		}
		if len(body.MemoryRecallTraces) != 1 || body.MemoryRecallTraces[0].RetrievedMemoryIDs[0] != "memory-recall-http" {
			t.Fatalf("%s memoryRecallTraces = %+v, want recalled memory", path, body.MemoryRecallTraces)
		}
	}
}

func TestEventStreamRoutePublishesFilteredHubEvents(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	lines := make(chan string, 16)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		readErr <- scanner.Err()
	}()

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       events.TopicWorkspaceCreated,
		WorkspaceID: "ws-other",
		Payload:     events.WorkspacePayload("other", "", "/tmp/other", "", common.WorkspaceStateActive, ""),
	}); err != nil {
		t.Fatalf("publish nonmatching event: %v", err)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       events.TopicWorkspaceCreated,
		WorkspaceID: "ws-stream",
		Payload:     events.WorkspacePayload("stream workspace", "", "/tmp/stream", "", common.WorkspaceStateActive, ""),
	}); err != nil {
		t.Fatalf("publish matching event: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case line := <-lines:
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var envelope events.Envelope
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &envelope); err != nil {
				t.Fatalf("decode stream data %q: %v", line, err)
			}
			if envelope.WorkspaceID != "ws-stream" || envelope.Topic != events.TopicWorkspaceCreated {
				t.Fatalf("stream envelope = %+v, want filtered workspace-created event", envelope)
			}
			return
		case err := <-readErr:
			t.Fatalf("stream closed before event: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for streamed event")
		}
	}
}

func TestConsoleRoutesServeEmbeddedAssets(t *testing.T) {
	mux := newMux(Config{Backend: "memory"})

	faviconReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	faviconRec := httptest.NewRecorder()
	mux.ServeHTTP(faviconRec, faviconReq)
	if faviconRec.Code != http.StatusNoContent {
		t.Fatalf("favicon status = %d, want 204", faviconRec.Code)
	}

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
	for _, want := range []string{"业务总览", "治理队列", "工作空间", "服务状态"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console js body does not contain %q", want)
		}
	}
	for _, want := range []string{"创建工作空间", "定义目标", "定义验收标准", "启动执行", "运行摘要", "当前运行"} {
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
	state.readWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSoulBindings, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace soul bindings read event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return appservice.WorkspaceSoulBindings{
			WorkspaceID:   workspaceID,
			DefaultSoulID: "soul-native",
			Bindings: map[string]string{
				"planning": "soul-native",
			},
		}, nil
	}
	state.updateWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string, cmd appservice.WorkspaceSoulBindings) (appservice.WorkspaceSoulBindings, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace soul bindings update event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" || cmd.DefaultSoulID != "soul-native" || cmd.Bindings["execution"] != "soul-native-execution" {
			t.Fatalf("soul bindings input = %+v for workspace %q, want native soul bindings", cmd, workspaceID)
		}
		cmd.WorkspaceID = workspaceID
		return cmd, nil
	}
	state.checkGoalDoDQuality = func(ctx context.Context, _, _ string, cmd appservice.GoalDoDQualityCheckCommand) (appservice.GoalDoDQualityCheck, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("goal/dod quality check event hub = %#v, want injected hub", got)
		}
		if cmd.WorkspaceID != "ws-native-read" || cmd.GoalID != "goal-native" || cmd.DoDID != "dod-native" {
			t.Fatalf("quality check cmd = %+v, want ws-native-read/goal-native/dod-native", cmd)
		}
		return appservice.GoalDoDQualityCheck{
			WorkspaceID:       cmd.WorkspaceID,
			GoalID:            cmd.GoalID,
			DoDID:             cmd.DoDID,
			Executable:        true,
			Verifiable:        true,
			ExecutionScore:    92,
			VerificationScore: 90,
			Status:            "ready",
		}, nil
	}
	state.readWorkspaceTaskProgress = func(ctx context.Context, _, _, workspaceID string) (appservice.TaskProgress, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace task progress event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return appservice.TaskProgress{WorkspaceID: workspaceID, State: "running", Recovery: appservice.RecoveryPlan{Mode: "continue"}}, nil
	}
	state.readWorkspaceContextBundle = func(ctx context.Context, _, _, workspaceID string) (appservice.ContextBundle, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace context bundle event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return appservice.ContextBundle{WorkspaceID: workspaceID, Mode: "workspace", Recovery: appservice.RecoveryPlan{Mode: "continue"}}, nil
	}
	state.readWorkspaceTaskDecompositions = func(ctx context.Context, _, _, workspaceID string) ([]appservice.TaskDecomposition, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace task decomposition event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
		}
		return []appservice.TaskDecomposition{{ID: "task-native", WorkspaceID: workspaceID, RunID: "run-native-read", State: "running"}}, nil
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
		if runID == "run-native-read" {
			got := appservice.EventHubFromContext(ctx)
			if got != nil && got != hub {
				t.Fatalf("run snapshot event hub = %#v, want injected hub", got)
			}
			state := "created"
			if got == nil {
				state = "completed"
			}
			return appservice.RunSnapshot{Run: appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: state}}, nil
		}
		if runID == "run-native-activation" {
			return appservice.RunSnapshot{Run: appservice.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}}, nil
		}
		t.Fatalf("runID = %q, want run-native-read or run-native-activation", runID)
		return appservice.RunSnapshot{}, nil
	}
	state.listRunSteps = func(ctx context.Context, _, _ string, query appservice.StepQuery) (appservice.StepPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run steps event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "step-native-1" || query.Limit != 2 {
			t.Fatalf("run steps query = %+v, want run-native-read cursor step-native-1 limit 2", query)
		}
		return appservice.StepPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "step-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunToolCalls = func(ctx context.Context, _, _ string, query appservice.ToolCallQuery) (appservice.ToolCallPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run tool calls event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "tool-native-1" || query.Limit != 2 {
			t.Fatalf("run tool calls query = %+v, want run-native-read cursor tool-native-1 limit 2", query)
		}
		return appservice.ToolCallPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "tool-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunRuntimeCheckpoints = func(ctx context.Context, _, _ string, query appservice.RuntimeCheckpointQuery) (appservice.RuntimeCheckpointPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run runtime checkpoints event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "checkpoint-native-1" || query.Limit != 2 {
			t.Fatalf("run runtime checkpoints query = %+v, want run-native-read cursor checkpoint-native-1 limit 2", query)
		}
		return appservice.RuntimeCheckpointPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "checkpoint-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunResults = func(ctx context.Context, _, _ string, query appservice.ResultQuery) (appservice.ResultPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run results event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "result-native-1" || query.Limit != 2 {
			t.Fatalf("run results query = %+v, want run-native-read cursor result-native-1 limit 2", query)
		}
		return appservice.ResultPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "result-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunPatches = func(ctx context.Context, _, _ string, query appservice.PatchQuery) (appservice.PatchPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run patches event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "patch-native-1" || query.Limit != 2 {
			t.Fatalf("run patches query = %+v, want run-native-read cursor patch-native-1 limit 2", query)
		}
		return appservice.PatchPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "patch-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunValidationResults = func(ctx context.Context, _, _ string, query appservice.ValidationResultQuery) (appservice.ValidationResultPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run validation results event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "validation-native-1" || query.Limit != 2 {
			t.Fatalf("run validation results query = %+v, want run-native-read cursor validation-native-1 limit 2", query)
		}
		return appservice.ValidationResultPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "validation-native-2",
			HasMore:           true,
		}, nil
	}
	state.listRunArtifacts = func(ctx context.Context, _, _ string, query appservice.ArtifactQuery) (appservice.ArtifactPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run artifacts event hub = %#v, want injected hub", got)
		}
		if query.RunID != "run-native-read" || query.CursorAfterID != "artifact-native-1" || query.Limit != 2 {
			t.Fatalf("run artifacts query = %+v, want run-native-read cursor artifact-native-1 limit 2", query)
		}
		return appservice.ArtifactPage{
			RunID:             query.RunID,
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "artifact-native-2",
			HasMore:           true,
		}, nil
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
	state.readRunTaskProgress = func(ctx context.Context, _, _, runID string) (appservice.TaskProgress, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run task progress event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.TaskProgress{WorkspaceID: "ws-native-read", RunID: runID, State: "running", Recovery: appservice.RecoveryPlan{Mode: "continue"}}, nil
	}
	state.readRunContextBundle = func(ctx context.Context, _, _, runID string) (appservice.ContextBundle, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run context bundle event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.ContextBundle{WorkspaceID: "ws-native-read", RunID: runID, Mode: "run_recovery", Recovery: appservice.RecoveryPlan{Mode: "continue"}}, nil
	}
	state.readRunTaskDecompositions = func(ctx context.Context, _, _, runID string) ([]appservice.TaskDecomposition, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run task decomposition event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return []appservice.TaskDecomposition{{ID: "task-native", WorkspaceID: "ws-native-read", RunID: runID, State: "running"}}, nil
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
	state.listDecisionPage = func(ctx context.Context, _, _ string, query appservice.DecisionQuery) (appservice.DecisionPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("decision page event hub = %#v, want injected hub", got)
		}
		if query.WorkspaceID != "ws-native-read" || query.RunID != "run-native-read" || query.CursorAfterID != "ticket-native-cursor" || query.Limit != 2 {
			t.Fatalf("decision page query = %+v, want ws-native-read/run-native-read cursor ticket-native-cursor limit 2", query)
		}
		return appservice.DecisionPage{
			WorkspaceID:       query.WorkspaceID,
			RunID:             query.RunID,
			Decisions:         []appservice.Decision{{ID: "ticket-native-read", WorkspaceID: query.WorkspaceID, RunID: query.RunID, State: "open"}},
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "ticket-native-read",
		}, nil
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
	state.listApprovalPage = func(ctx context.Context, _, _ string, query appservice.ApprovalQuery) (appservice.ApprovalPage, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval page event hub = %#v, want injected hub", got)
		}
		if query.WorkspaceID != "ws-native-read" || query.RunID != "run-native-read" || query.CursorAfterID != "approval-native-cursor" || query.Limit != 2 {
			t.Fatalf("approval page query = %+v, want ws-native-read/run-native-read cursor approval-native-cursor limit 2", query)
		}
		return appservice.ApprovalPage{
			WorkspaceID:       query.WorkspaceID,
			RunID:             query.RunID,
			Approvals:         []appservice.Approval{{ID: "approval-native-read", WorkspaceID: query.WorkspaceID, RunID: query.RunID, State: "pending"}},
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: "approval-native-read",
		}, nil
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
	state.createWorkspace = func(ctx context.Context, _, _ string, cmd appservice.CreateWorkspaceCommand) (appservice.Workspace, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace create event hub = %#v, want injected hub", got)
		}
		if cmd.Title != "native write" || cmd.Description != "native description" || cmd.DeliverableDirectory != "workspaces/native-write" || cmd.TemplateID != "software-dev" {
			t.Fatalf("workspace create input = %q/%q/%q/%q, want native write/native description/workspaces/native-write/software-dev", cmd.Title, cmd.Description, cmd.DeliverableDirectory, cmd.TemplateID)
		}
		return appservice.Workspace{ID: "ws-native-write", Title: cmd.Title, Description: cmd.Description, DeliverableDirectory: cmd.DeliverableDirectory, State: "active"}, nil
	}
	state.deleteWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appservice.DeleteWorkspaceCommand) error {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace delete event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" || !cmd.Purge {
			t.Fatalf("workspace delete input = %q/%+v, want ws-native-read purge", workspaceID, cmd)
		}
		return nil
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
	state.resumeRun = func(ctx context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
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
			cmd.LLMProviderID != "deepseek" ||
			cmd.SoulID != "soul-native" ||
			cmd.SoulBindings["validation"] != "soul-native-validation" ||
			cmd.DialogLanguage != "zh-CN" ||
			cmd.Priority != 9 ||
			!stringSliceEqual(cmd.SkillIDs, []string{"skill-native"}) ||
			!stringSliceEqual(cmd.MCPServerIDs, []string{"mcp-native"}) ||
			!stringSliceEqual(cmd.MemoryIDs, []string{"memory-native"}) ||
			cmd.ExecutionContextPatch["latestOperatorInput"].(map[string]any)["content"] != "restart from tui" {
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
	state.batchResolveDecisions = func(ctx context.Context, _, _ string, cmd appservice.BatchResolveDecisionCommand) ([]appservice.Decision, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("decision batch resolve event hub = %#v, want injected hub", got)
		}
		if !stringSliceEqual(cmd.TicketIDs, []string{"ticket-native-read"}) || cmd.ResolutionType != decisioncommon.ResolutionTypeReplan {
			t.Fatalf("batch resolve cmd = %+v, want ticket-native-read replan", cmd)
		}
		return []appservice.Decision{{ID: "ticket-native-read", WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "resolved", ResolutionType: cmd.ResolutionType}}, nil
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
	state.batchResolveApprovals = func(ctx context.Context, _, _ string, cmd appservice.BatchResolveApprovalCommand) ([]appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval batch resolve event hub = %#v, want injected hub", got)
		}
		if !stringSliceEqual(cmd.ApprovalIDs, []string{"approval-native-read"}) || cmd.Action != "approve" {
			t.Fatalf("batch approval cmd = %+v, want approval-native-read approve", cmd)
		}
		return []appservice.Approval{{ID: "approval-native-read", WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "approved"}}, nil
	}
	handler := newMuxWithState(state)

	for _, tc := range []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces", body: `{"title":"native write","description":"native description","deliverableDirectory":"workspaces/native-write","templateId":"software-dev"}`, status: http.StatusCreated},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read", status: http.StatusOK},
		{method: http.MethodDelete, path: "/api/v1/workorch/workspaces/ws-native-read?purge=true", status: http.StatusNoContent},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/task-progress", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/context-bundle", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/task-decompositions", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/soul-bindings", status: http.StatusOK},
		{method: http.MethodPut, path: "/api/v1/workorch/workspaces/ws-native-read/soul-bindings", body: `{"defaultSoulId":"soul-native","bindings":{"execution":"soul-native-execution"}}`, status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/goal-dod-quality-check", body: `{"goalId":"goal-native","doDId":"dod-native"}`, status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/runs", body: `{"runId":"run-native-write","goalId":"goal-native","doDId":"dod-native"}`, status: http.StatusCreated},
		{method: http.MethodGet, path: "/api/v1/workorch/runs?workspaceId=ws-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/steps?cursorAfterId=step-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/tool-calls?cursorAfterId=tool-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/runtime-checkpoints?cursorAfterId=checkpoint-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/results?cursorAfterId=result-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/patches?cursorAfterId=patch-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/validation-results?cursorAfterId=validation-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/artifacts?cursorAfterId=artifact-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/status", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/observability", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/task-progress", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/context-bundle", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/runs/run-native-read/task-decompositions", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/runs/run-native-read/start", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/runs/run-native-read/resume", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/execution-activations", body: `{"goalId":"goal-native","doDId":"dod-native","llmProviderId":"deepseek","soulId":"soul-native","soulBindings":{"validation":"soul-native-validation"},"skillIds":["skill-native"],"mcpServerIds":["mcp-native"],"memoryIds":["memory-native"],"dialogLanguage":"zh-CN","priority":9,"executionContextPatch":{"latestOperatorInput":{"source":"tui","content":"restart from tui"}}}`, status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions?workspaceId=ws-native-read&runId=run-native-read&cursorAfterId=ticket-native-cursor&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions/ticket-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/decisions/ticket-native-read/resolve", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/decisions/resolve-batch", body: `{"ticketIds":["ticket-native-read"],"resolutionType":"replan"}`, status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals?workspaceId=ws-native-read&runId=run-native-read&cursorAfterId=approval-native-cursor&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals/approval-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/approve", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/reject", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/resolve-batch", body: `{"approvalIds":["approval-native-read"],"action":"approve"}`, status: http.StatusOK},
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"http workspace","description":"http description","deliverableDirectory":"workspaces/ws-http"}`))
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
		created.Workspace.DeliverableDirectory != "workspaces/ws-http" ||
		created.Workspace.State != "active" {
		t.Fatalf("created workspace = %+v, want ws-http/http workspace/workspaces/ws-http/active", created.Workspace)
	}
}

func TestWorkspaceSetupDraftSessionSupportsMultiTurnDraft(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.listWorkspaceTemplates = func(context.Context, string, string) ([]appservice.WorkspaceTemplate, error) {
		return []appservice.WorkspaceTemplate{{
			ID:       "generic-template",
			Name:     "Generic Template",
			GoalText: "Template goal",
			DoDItems: []string{"Template DoD"},
		}}, nil
	}
	generateCalls := 0
	state.generateWorkspaceSetupDraft = func(_ context.Context, _, _ string, cmd appservice.WorkspaceSetupDraftGenerationCommand) (appservice.WorkspaceSetupDraft, error) {
		generateCalls++
		if cmd.TemplateID != "generic-template" || len(cmd.Messages) == 0 {
			t.Fatalf("draft command = %+v, want template and user messages", cmd)
		}
		if generateCalls == 1 && cmd.CurrentDraft.GoalText != "Template goal" {
			t.Fatalf("first draft current draft = %+v, want template draft baseline", cmd.CurrentDraft)
		}
		if generateCalls == 2 {
			if cmd.CurrentDraft.GoalText != "Generated goal from first requirement" {
				t.Fatalf("second draft current draft = %+v, want previous generated draft", cmd.CurrentDraft)
			}
			if len(cmd.Messages) != 3 || cmd.Messages[1].Role != "assistant" {
				t.Fatalf("second draft messages = %+v, want full multi-turn history with assistant draft", cmd.Messages)
			}
		}
		return appservice.WorkspaceSetupDraft{
			GoalText:          "Generated goal from " + cmd.Messages[len(cmd.Messages)-1].Content,
			GenerationSummary: "Draft revised from current context",
			DoDItems: []string{`
### Acceptance
- [ ] **Group**:
- [ ] first item
- [ ] second item
`},
			Confidence: "high",
		}, nil
	}
	handler := newMuxWithState(state)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-setup-draft-sessions", strings.NewReader(`{"templateId":"generic-template","title":"draft workspace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	var created workspaceSetupDraftSessionResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created draft session: %v", err)
	}
	if created.DraftSession.ID == "" || created.DraftSession.Draft.GoalText != "Template goal" {
		t.Fatalf("created draft session = %+v, want template draft", created.DraftSession)
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-setup-draft-sessions/"+created.DraftSession.ID+"/messages", strings.NewReader(`{"content":"first requirement"}`))
	msgReq.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("message status = %d, want 200, body=%s", msgRec.Code, msgRec.Body.String())
	}
	var updated workspaceSetupDraftSessionResponse
	if err := json.Unmarshal(msgRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated draft session: %v", err)
	}
	wantItems := []string{"Acceptance / Group： first item", "Acceptance / Group： second item"}
	if updated.DraftSession.State != "draft_ready" ||
		updated.DraftSession.Draft.GoalText != "Generated goal from first requirement" ||
		len(updated.DraftSession.Messages) != 2 ||
		!reflect.DeepEqual(updated.DraftSession.Draft.DoDItems, wantItems) {
		t.Fatalf("updated draft session = %+v, want normalized multi-turn draft", updated.DraftSession)
	}
	if !strings.Contains(updated.DraftSession.Messages[1].Content, "DoD:") {
		t.Fatalf("assistant message = %q, want DoD content for future turns", updated.DraftSession.Messages[1].Content)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-setup-draft-sessions/"+created.DraftSession.ID+"/messages", strings.NewReader(`{"content":"second requirement"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second message status = %d, want 200, body=%s", secondRec.Code, secondRec.Body.String())
	}
	var second workspaceSetupDraftSessionResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second draft session: %v", err)
	}
	if second.DraftSession.Draft.GoalText != "Generated goal from second requirement" || len(second.DraftSession.Messages) != 4 {
		t.Fatalf("second draft session = %+v, want revised draft with full history", second.DraftSession)
	}
	if second.DraftSession.Draft.GenerationSummary == "" {
		t.Fatalf("second draft = %+v, want generation summary for display", second.DraftSession.Draft)
	}
}

func TestWorkspaceSetupDraftMessageCanRestoreTemplateContext(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	state.listWorkspaceTemplates = func(context.Context, string, string) ([]appservice.WorkspaceTemplate, error) {
		return []appservice.WorkspaceTemplate{{
			ID:       "generic-template",
			Name:     "Generic Template",
			GoalText: "Template goal",
			DoDItems: []string{"Template DoD"},
		}}, nil
	}
	state.generateWorkspaceSetupDraft = func(_ context.Context, _, _ string, cmd appservice.WorkspaceSetupDraftGenerationCommand) (appservice.WorkspaceSetupDraft, error) {
		if cmd.TemplateID != "generic-template" {
			t.Fatalf("draft command template = %q, want restored template", cmd.TemplateID)
		}
		if cmd.CurrentDraft.GoalText != "Template goal" || !reflect.DeepEqual(cmd.CurrentDraft.DoDItems, []string{"Template DoD"}) {
			t.Fatalf("draft command current draft = %+v, want restored template draft", cmd.CurrentDraft)
		}
		if len(cmd.CurrentDraft.ClarifyingQuestions) != 0 {
			t.Fatalf("draft command current draft = %+v, want stale clarifications cleared before revision", cmd.CurrentDraft)
		}
		return appservice.WorkspaceSetupDraft{ClarifyingQuestions: []string{"Need more details"}}, nil
	}
	handler := newMuxWithState(state)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-setup-draft-sessions", strings.NewReader(`{"title":"draft workspace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	var created workspaceSetupDraftSessionResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created draft session: %v", err)
	}

	body := `{"templateId":"generic-template","title":"draft workspace","draft":{"clarifyingQuestions":["old question"]},"content":"new requirement"}`
	msgReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-setup-draft-sessions/"+created.DraftSession.ID+"/messages", strings.NewReader(body))
	msgReq.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("message status = %d, want 200, body=%s", msgRec.Code, msgRec.Body.String())
	}
	var updated workspaceSetupDraftSessionResponse
	if err := json.Unmarshal(msgRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated draft session: %v", err)
	}
	if updated.DraftSession.TemplateID != "generic-template" ||
		updated.DraftSession.Draft.GoalText != "Template goal" ||
		!reflect.DeepEqual(updated.DraftSession.Draft.DoDItems, []string{"Template DoD"}) {
		t.Fatalf("updated draft session = %+v, want restored template goal and DoD", updated.DraftSession)
	}
	if len(updated.DraftSession.Draft.ClarifyingQuestions) == 0 {
		t.Fatalf("updated draft = %+v, want clarification retained", updated.DraftSession.Draft)
	}
}

func TestWorkspaceCreateEndpointDefaultsDeliverableDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

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
	want := filepath.Join(filepath.Dir(dataRoot), ".workorch-workspaces", body.Workspace.ID)
	if body.Workspace.DeliverableDirectory != want {
		t.Fatalf("deliverableDirectory = %q, want %q", body.Workspace.DeliverableDirectory, want)
	}
	for _, dir := range []string{
		filepath.Join(filepath.Dir(dataRoot), ".workorch-workspaces", body.Workspace.ID),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "artifacts"),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "runs"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
}

func TestArtifactContentEndpointReturnsStoredArtifactFile(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	artifact := &resultmodel.Artifact{
		ID:           common.ArtifactID("run-http-artifact/artifact/result-summary"),
		WorkspaceID:  common.WorkspaceID("ws-http-artifact"),
		RunID:        common.RunID("run-http-artifact"),
		ResultID:     common.ResultID("run-http-artifact/result/dod-assessment"),
		ArtifactType: resultcommon.ArtifactTypeText,
		Name:         "Accepted Result Summary",
		Summary:      "HTTP artifact payload",
		State:        common.ArtifactStateAvailable,
	}
	if err := exports.Repositories.Artifacts.Create(ctx, artifact); err != nil {
		t.Fatalf("Create artifact: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("Close session: %v", err)
	}

	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	artifactURI := "workorch://workspaces/ws-http-artifact/artifacts/run-http-artifact/result-summary.txt"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/artifacts/content?uri="+url.QueryEscape(artifactURI), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/plain", got)
	}
	if got := rec.Header().Get("X-Workorch-Artifact-URI"); got != artifactURI {
		t.Fatalf("artifact uri header = %q, want %q", got, artifactURI)
	}
	if rec.Body.String() != "HTTP artifact payload\n" {
		t.Fatalf("body = %q, want artifact payload", rec.Body.String())
	}
}

func TestRuntimePayloadEndpointReturnsLLMRawRefContent(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-http-raw"
	runID := "run-http-raw"
	stepPath := "run-http-raw/llm-1"
	rawDir := filepath.Join(dataRoot, "internal", "workspaces", workspaceID, "runs", runID, "llm-calls", filepath.FromSlash(stepPath))
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("mkdir raw dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "request.json"), []byte(`{"model":"test"}`), 0o600); err != nil {
		t.Fatalf("write request.json: %v", err)
	}

	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	ref := "workorch://workspaces/ws-http-raw/runs/run-http-raw/llm-calls/" + stepPath + "/request.json"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runtime/payloads?ref="+url.QueryEscape(ref), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if got := rec.Header().Get("X-Workorch-Raw-Ref"); got != ref {
		t.Fatalf("raw ref header = %q, want %q", got, ref)
	}
	if rec.Body.String() != `{"model":"test"}` {
		t.Fatalf("body = %q, want raw request payload", rec.Body.String())
	}
}

func TestArtifactContentEndpointReusesCachedRunSnapshotArtifacts(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceID := "ws-http-artifact-cache"
	runID := "run-http-artifact-cache"
	artifactURI := "workorch://workspaces/ws-http-artifact-cache/artifacts/run-http-artifact-cache/result-summary.txt"
	artifactDir := filepath.Join(dataRoot, "internal", "workspaces", workspaceID, "artifacts", runID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "result-summary.txt"), []byte("cached HTTP artifact payload"), 0o600); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	state := newServerState(normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
	state.configureSessionBackedOperations(session)
	state.cacheRunSnapshot(appservice.RunSnapshot{
		Run: appservice.Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateCompleted)},
		Artifacts: []appservice.Artifact{{
			WorkspaceID: workspaceID,
			RunID:       runID,
			URI:         artifactURI,
			ContentType: "text/markdown; charset=utf-8",
			State:       string(common.ArtifactStateAvailable),
		}},
	})
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := state.close(closeCtx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/artifacts/content?uri="+url.QueryEscape(artifactURI), nil)
	rec := httptest.NewRecorder()
	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("content-type = %q, want cached metadata content type", got)
	}
	if rec.Body.String() != "cached HTTP artifact payload" {
		t.Fatalf("body = %q, want cached artifact payload", rec.Body.String())
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
	if fetched.Workspace.ID != workspace.ID || fetched.Workspace.Title != "http workspace" || fetched.Workspace.DeliverableDirectory != "workspaces/ws-http" {
		t.Fatalf("fetched workspace = %+v, want %s/http workspace/workspaces/ws-http", fetched.Workspace, workspace.ID)
	}
}

func TestWorkspaceDeleteEndpointDeletesWorkspaceAndSupportsPurge(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"http delete workspace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	var created workspaceResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created workspace: %v", err)
	}
	workspace := created.Workspace
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workorch/workspaces/"+workspace.ID+"?purge=true", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get deleted status = %d, want 404, body=%s", getRec.Code, getRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "internal", "workspaces", workspace.ID)); !os.IsNotExist(err) {
		t.Fatalf("workspace data dir err = %v, want not exist", err)
	}
}

func TestWorkspaceCreateEndpointRejectsClientWorkspaceID(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-client","title":"client workspace","deliverableDirectory":"workspaces/ws-client"}`))
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

func TestGoalAndDoDUpdateEndpointsReturnUpdatedDefinitions(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	workspace := createWorkspaceForHTTP(t, handler, "definition update workspace", "workspaces/definition-update")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	goalReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/goals/"+goalID, strings.NewReader(`{"text":"updated goal text"}`))
	goalReq.Header.Set("Content-Type", "application/json")
	goalRec := httptest.NewRecorder()
	handler.ServeHTTP(goalRec, goalReq)
	if goalRec.Code != http.StatusOK {
		t.Fatalf("goal update status = %d, want 200, body=%s", goalRec.Code, goalRec.Body.String())
	}
	var goalBody goalResponse
	if err := json.Unmarshal(goalRec.Body.Bytes(), &goalBody); err != nil {
		t.Fatalf("unmarshal goal response: %v", err)
	}
	if goalBody.Goal.ID != goalID || goalBody.Goal.Text != "updated goal text" {
		t.Fatalf("goal response = %+v, want updated text", goalBody.Goal)
	}

	dodReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/dods/"+dodID, strings.NewReader(`{"items":["updated dod","second dod"]}`))
	dodReq.Header.Set("Content-Type", "application/json")
	dodRec := httptest.NewRecorder()
	handler.ServeHTTP(dodRec, dodReq)
	if dodRec.Code != http.StatusOK {
		t.Fatalf("dod update status = %d, want 200, body=%s", dodRec.Code, dodRec.Body.String())
	}
	var dodBody dodResponse
	if err := json.Unmarshal(dodRec.Body.Bytes(), &dodBody); err != nil {
		t.Fatalf("unmarshal dod response: %v", err)
	}
	if dodBody.DoD.ID != dodID || len(dodBody.DoD.Items) != 2 || dodBody.DoD.Items[0] != "updated dod" || dodBody.DoD.State != "ready" {
		t.Fatalf("dod response = %+v, want updated ready DoD", dodBody.DoD)
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

func TestEventStreamEndpointReplaysFileBackedRecordsAfterRestart(t *testing.T) {
	dataRoot := t.TempDir()
	ctx := context.Background()
	first := NewHTTPServer(Config{Backend: "file", DataRoot: dataRoot})
	firstHandler := first.http.Handler

	workspace := createWorkspaceForHTTP(t, firstHandler, "event replay workspace", "workspaces/event-replay")
	goalID, dodID := defineGoalAndDoDForHTTP(t, firstHandler, workspace.ID)
	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-event-replay","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	firstHandler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}
	if err := first.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown first server: %v", err)
	}

	second := NewHTTPServer(Config{Backend: "file", DataRoot: dataRoot})
	t.Cleanup(func() {
		if err := second.Shutdown(ctx); err != nil {
			t.Fatalf("shutdown second server: %v", err)
		}
	})
	secondHandler := second.http.Handler

	workspaceEvents := readEventRecordsForHTTP(t, secondHandler, "/api/v1/workorch/events?workspaceId="+workspace.ID)
	if !eventRecordsContainTopic(workspaceEvents, events.TopicWorkspaceCreated) ||
		!eventRecordsContainTopic(workspaceEvents, events.TopicGoalDefined) ||
		!eventRecordsContainTopic(workspaceEvents, events.TopicDoDDefined) ||
		!eventRecordsContainTopic(workspaceEvents, events.TopicRunCreated) {
		t.Fatalf("workspace events = %+v, want workspace/goal/dod/run persisted events after restart", workspaceEvents)
	}

	runEvents := readEventRecordsForHTTP(t, secondHandler, "/api/v1/workorch/events?runId=run-event-replay")
	if len(runEvents) != 1 || runEvents[0].Topic != events.TopicRunCreated || runEvents[0].WorkspaceID != workspace.ID {
		t.Fatalf("run events = %+v, want replayed run-created event", runEvents)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-event-replay/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	secondHandler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var snapshotBody runSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &snapshotBody); err != nil {
		t.Fatalf("unmarshal run snapshot: %v", err)
	}
	if len(snapshotBody.Snapshot.EventRecords) != 1 || snapshotBody.Snapshot.EventRecords[0].Topic != events.TopicRunCreated {
		t.Fatalf("snapshot eventRecords = %+v, want replayed run-created event", snapshotBody.Snapshot.EventRecords)
	}

	observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-event-replay/observability", nil)
	observabilityRec := httptest.NewRecorder()
	secondHandler.ServeHTTP(observabilityRec, observabilityReq)
	if observabilityRec.Code != http.StatusOK {
		t.Fatalf("observability status = %d, want 200, body=%s", observabilityRec.Code, observabilityRec.Body.String())
	}
	var observabilityBody runObservabilityResponse
	if err := json.Unmarshal(observabilityRec.Body.Bytes(), &observabilityBody); err != nil {
		t.Fatalf("unmarshal run observability: %v", err)
	}
	if !timelineContainsPersistedEvent(observabilityBody.Observability.Timeline, events.TopicRunCreated) {
		t.Fatalf("timeline = %+v, want persisted run event after restart", observabilityBody.Observability.Timeline)
	}
}

func TestEventRecordQueryFromRequestAppliesBoundedDefaultLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/events", nil)
	if got := eventRecordQueryFromRequest(req).Limit; got != defaultEventRecordPageLimit {
		t.Fatalf("default limit = %d, want %d", got, defaultEventRecordPageLimit)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/workorch/events?limit=2", nil)
	if got := eventRecordQueryFromRequest(req).Limit; got != 2 {
		t.Fatalf("explicit limit = %d, want 2", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/workorch/events?limit=999999", nil)
	if got := eventRecordQueryFromRequest(req).Limit; got != maxEventRecordPageLimit {
		t.Fatalf("clamped limit = %d, want %d", got, maxEventRecordPageLimit)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/workorch/events?limit=-1", nil)
	if got := eventRecordQueryFromRequest(req).Limit; got != defaultEventRecordPageLimit {
		t.Fatalf("negative limit = %d, want %d", got, defaultEventRecordPageLimit)
	}
}

func TestRunSnapshotEndpointReturnsPersistedRunSnapshot(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-snapshot-http"

	workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "run snapshot http workspace", runID)

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

	runScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
	if err := runScopedRepos.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "run evidence http",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := runScopedRepos.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := runScopedRepos.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "validation",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := runScopedRepos.ToolCalls.Create(ctx, &runmodel.ToolCall{
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
	if err := runScopedRepos.Patches.Create(ctx, &runmodel.Patch{
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
	if err := runScopedRepos.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-evidence-http"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "go_test",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := runScopedRepos.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
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
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-status-http"

	workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run status http workspace", runID)

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

func TestRunStatusEndpointCanWaitForTargetState(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var calls atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		call := calls.Add(1)
		status := "running"
		if call >= 2 {
			status = "completed"
		}
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-wait-status", State: status}, nil
	}
	handler := newMuxWithState(state)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-wait-status/status?waitState=completed&waitTimeoutMillis=200&waitIntervalMillis=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.State != "completed" || !body.WaitMatched {
		t.Fatalf("status response = %+v, want completed wait match", body)
	}
	if calls.Load() < 2 {
		t.Fatalf("readRunStatus calls = %d, want repeated wait polling", calls.Load())
	}
}

func TestRunStatusWaitUsesStatusChangeNotification(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var completed atomic.Bool
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		status := "running"
		if completed.Load() {
			status = "completed"
		}
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-notified-status", State: status}, nil
	}

	done := make(chan struct{})
	var got appservice.RunStatus
	var matched bool
	var err error
	started := time.Now()
	go func() {
		defer close(done)
		got, matched, err = state.readRunStatusWithWait(context.Background(), "run-notified-status", url.Values{
			"waitState":          []string{"completed"},
			"waitTimeoutMillis":  []string{"1000"},
			"waitIntervalMillis": []string{"900"},
		})
	}()

	time.Sleep(20 * time.Millisecond)
	completed.Store(true)
	state.notifyRunStatusChanged()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("status wait did not wake from notification")
	}
	if err != nil {
		t.Fatalf("readRunStatusWithWait error = %v", err)
	}
	if !matched || got.State != "completed" {
		t.Fatalf("status = %+v matched=%v, want completed match", got, matched)
	}
	if elapsed := time.Since(started); elapsed >= 300*time.Millisecond {
		t.Fatalf("status wait elapsed = %v, want notification wake before poll interval", elapsed)
	}
}

func TestTerminalRunSnapshotCacheReusesSnapshotForReadViews(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var snapshotReads atomic.Int32
	var observabilityReads atomic.Int32
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		snapshotReads.Add(1)
		return appservice.RunSnapshot{
			Workspace: appservice.Workspace{ID: "ws-cache", State: string(common.WorkspaceStateCompleted)},
			Run:       appservice.Run{ID: runID, WorkspaceID: "ws-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	state.readRunObservability = func(context.Context, string, string, string) (appservice.RunObservability, error) {
		observabilityReads.Add(1)
		return appservice.RunObservability{}, nil
	}
	handler := newMuxWithState(state)

	for _, path := range []string{
		"/api/v1/workorch/runs/run-cache/snapshot",
		"/api/v1/workorch/runs/run-cache/status",
		"/api/v1/workorch/runs/run-cache/observability",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200, body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want cached single read", snapshotReads.Load())
	}
	if observabilityReads.Load() != 0 {
		t.Fatalf("observability reads = %d, want cached snapshot projection", observabilityReads.Load())
	}

	state.notifyRunStatusChanged()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-cache/snapshot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot after invalidation status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if snapshotReads.Load() != 2 {
		t.Fatalf("snapshot reads after invalidation = %d, want 2", snapshotReads.Load())
	}
}

func TestRunStatusWaitWarmsTerminalRunSnapshotCache(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var snapshotReads atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-status-warm-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		snapshotReads.Add(1)
		return appservice.RunSnapshot{
			Workspace: appservice.Workspace{ID: "ws-status-warm-cache", State: string(common.WorkspaceStateCompleted)},
			Run:       appservice.Run{ID: runID, WorkspaceID: "ws-status-warm-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	handler := newMuxWithState(state)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-status-warm-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		_, ok := state.cachedRunSnapshot("run-status-warm-cache")
		return ok
	})

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-status-warm-cache/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want one background warm read reused by snapshot endpoint", snapshotReads.Load())
	}
}

func TestRunStatusNotificationForTerminalRunWarmsSnapshotCache(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var snapshotReads atomic.Int32
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		snapshotReads.Add(1)
		return appservice.RunSnapshot{
			Workspace: appservice.Workspace{ID: "ws-notify-warm-cache", State: string(common.WorkspaceStateCompleted)},
			Run:       appservice.Run{ID: runID, WorkspaceID: "ws-notify-warm-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	handler := newMuxWithState(state)

	state.notifyRunStatusChangedForRun(appservice.Run{
		ID:          "run-notify-warm-cache",
		WorkspaceID: "ws-notify-warm-cache",
		State:       string(common.RunStateCompleted),
	})
	waitForCondition(t, time.Second, func() bool {
		_, ok := state.cachedRunSnapshot("run-notify-warm-cache")
		return ok
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-notify-warm-cache/snapshot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want notification warm read reused by snapshot endpoint", snapshotReads.Load())
	}
}

func TestRunSnapshotReadWaitsForTerminalStatusWarmupInflight(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var snapshotReads atomic.Int32
	releaseRead := make(chan struct{})
	readStarted := make(chan struct{})
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-status-inflight-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		snapshotReads.Add(1)
		close(readStarted)
		<-releaseRead
		return appservice.RunSnapshot{
			Workspace: appservice.Workspace{ID: "ws-status-inflight-cache", State: string(common.WorkspaceStateCompleted)},
			Run:       appservice.Run{ID: runID, WorkspaceID: "ws-status-inflight-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	handler := newMuxWithState(state)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-status-inflight-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("background snapshot warmup did not start")
	}

	snapshotDone := make(chan struct{})
	go func() {
		defer close(snapshotDone)
		snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-status-inflight-cache/snapshot", nil)
		snapshotRec := httptest.NewRecorder()
		handler.ServeHTTP(snapshotRec, snapshotReq)
		if snapshotRec.Code != http.StatusOK {
			t.Errorf("snapshot = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
		}
	}()
	time.Sleep(20 * time.Millisecond)
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads before warmup release = %d, want only background read in flight", snapshotReads.Load())
	}
	close(releaseRead)
	select {
	case <-snapshotDone:
	case <-time.After(time.Second):
		t.Fatal("snapshot endpoint did not complete from warmup in-flight read")
	}
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want in-flight warm read reused by snapshot endpoint", snapshotReads.Load())
	}
}

func TestRunObservabilityReadWaitsForTerminalStatusWarmupInflight(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	var snapshotReads atomic.Int32
	var observabilityReads atomic.Int32
	releaseRead := make(chan struct{})
	readStarted := make(chan struct{})
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		return appservice.RunStatus{ID: runID, WorkspaceID: "ws-observability-inflight-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		snapshotReads.Add(1)
		close(readStarted)
		<-releaseRead
		return appservice.RunSnapshot{
			Workspace: appservice.Workspace{ID: "ws-observability-inflight-cache", State: string(common.WorkspaceStateCompleted)},
			Run:       appservice.Run{ID: runID, WorkspaceID: "ws-observability-inflight-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	state.readRunObservability = func(context.Context, string, string, string) (appservice.RunObservability, error) {
		observabilityReads.Add(1)
		return appservice.RunObservability{}, nil
	}
	handler := newMuxWithState(state)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-observability-inflight-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("background snapshot warmup did not start")
	}

	observabilityDone := make(chan struct{})
	go func() {
		defer close(observabilityDone)
		observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-observability-inflight-cache/observability", nil)
		observabilityRec := httptest.NewRecorder()
		handler.ServeHTTP(observabilityRec, observabilityReq)
		if observabilityRec.Code != http.StatusOK {
			t.Errorf("observability = %d, want 200, body=%s", observabilityRec.Code, observabilityRec.Body.String())
		}
	}()
	time.Sleep(20 * time.Millisecond)
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads before warmup release = %d, want only background read in flight", snapshotReads.Load())
	}
	if observabilityReads.Load() != 0 {
		t.Fatalf("observability reads before warmup release = %d, want snapshot inflight reuse", observabilityReads.Load())
	}
	close(releaseRead)
	select {
	case <-observabilityDone:
	case <-time.After(time.Second):
		t.Fatal("observability endpoint did not complete from warmup in-flight read")
	}
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want in-flight warm read reused by observability endpoint", snapshotReads.Load())
	}
	if observabilityReads.Load() != 0 {
		t.Fatalf("observability reads = %d, want cached snapshot projection", observabilityReads.Load())
	}
}

func TestWorkspaceSnapshotEndpointReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	workspace := createWorkspaceForHTTP(t, handler, "workspace snapshot http", "workspaces/workspace-snapshot-http")
	ctx := context.Background()
	runScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
	workspaceRepo := runScopedRepos.Workspaces
	if err := runScopedRepos.Runs.Create(ctx, &runmodel.Run{
		ID:          "run-workspace-snapshot-http-completed",
		WorkspaceID: common.WorkspaceID(workspace.ID),
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	decisionRun := seedBlockedDecisionRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, "run-decision-workspace-snapshot-http")
	approvalRun := seedBlockedApprovalRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, "run-approval-workspace-snapshot-http")
	workspaceItem, err := workspaceRepo.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(approvalRun.ID)
	if err := workspaceRepo.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

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

func TestRunStartEndpointReturnsBlockedProviderDecisionRun(t *testing.T) {
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
	if body.Run.ID != "run-start-http" ||
		body.Run.State != string(common.RunStateBlockedOnDecision) ||
		body.Run.BlockingTicketID == "" ||
		body.Run.FailureReason != "" {
		t.Fatalf("started run = %+v, want run-start-http blocked on provider decision", body.Run)
	}
}

func TestRunStartEndpointReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-http"

	_, seeded := seedBlockedDecisionRunForHTTP(t, dataRoot, "run start blocked http workspace", runID)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal blocked run: %v", err)
	}
	if body.Run.State != "blocked_on_decision" || body.Run.BlockingTicketID != seeded.BlockingTicketID {
		t.Fatalf("run = %+v, want seeded blocked decision", body.Run)
	}
}

func TestRunResumeEndpointReturnsFailedRunAfterDecisionResolvedWithoutContinuation(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-resume-http"

	_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "run resume decision http workspace", runID)
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}

	resolveDecisionForHTTP(t, dataRoot, started.BlockingTicketID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200 with terminal failed run, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if body.Run.State != "failed" || body.Run.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want failed with cleared decision blocker", body.Run)
	}
}

func TestRunResumeEndpointReturnsFailedRunAfterApprovalApprovedWithoutContinuation(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-resume-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume approval http workspace", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200 with terminal failed run, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if body.Run.State != "failed" || body.Run.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want failed with cleared approval blocker", body.Run)
	}
}

func TestRunResumeEndpointReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-rejected-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume approval rejected http workspace", runID)
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
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-pending-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume pending http workspace", runID)
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
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runA := "run-decision-list-a"
	runB := "run-decision-list-b"

	_, startedA := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision list http workspace a", runA)
	workspaceB, startedB := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision list http workspace b", runB)

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
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-decision-resolve-http"

	_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision resolve http workspace", runID)

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

	resumed := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
	if resumed.BlockingTicketID != "" {
		t.Fatalf("resumed run = %+v, want cleared decision blocker", resumed)
	}
}

func TestDecisionResolveEndpointRejectCurrentPathAndRunResumeFailsTerminally(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-decision-reject-http"

	_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision reject http workspace", runID)

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

	resumed := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
	if resumed.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed)
	}
}

func TestDecisionResolveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-decision-conflict-http"

	_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision conflict http workspace", runID)
	rejectDecisionViaHTTP(t, handler, started.BlockingTicketID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusConflict {
		t.Fatalf("decision resolve status = %d, want 409, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
	waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
}

func TestDecisionBatchResolveEndpointResolvesOpenTickets(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	firstRunID := "run-decision-batch-a"
	secondRunID := "run-decision-batch-b"

	_, first := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision batch http workspace a", firstRunID)
	_, second := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision batch http workspace b", secondRunID)

	body := strings.NewReader(`{"ticketIds":["` + second.BlockingTicketID + `","` + first.BlockingTicketID + `"],"resolutionType":"replan","resolutionPayload":{"operator":"http-batch"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/resolve-batch", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch resolve status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resolved decisionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal batch resolved decisions: %v", err)
	}
	if len(resolved.Decisions) != 2 ||
		resolved.Decisions[0].ID != second.BlockingTicketID ||
		resolved.Decisions[1].ID != first.BlockingTicketID {
		t.Fatalf("resolved decisions = %+v, want request order", resolved.Decisions)
	}
	for _, item := range resolved.Decisions {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "http-batch" {
			t.Fatalf("decision = %+v, want resolved replan with payload", item)
		}
	}
	waitRunStateForHTTP(t, dataRoot, firstRunID, string(common.RunStateSuperseded))
	waitRunStateForHTTP(t, dataRoot, secondRunID, string(common.RunStateSuperseded))
}

func TestServerStateSchedulesReplanContinuationForSupersededReplanRun(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	activated := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan",
				WorkspaceID:    "ws-replan-continuation",
				RunID:          "run-superseded-replan",
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.activateWorkspace = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if workspaceID != "ws-replan-continuation" {
			t.Fatalf("workspaceID = %q, want ws-replan-continuation", workspaceID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-replan-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	state.scheduleReplanContinuationIfNeeded(context.Background(), "ws-replan-continuation", "run-superseded-replan", 3)

	select {
	case cmd := <-activated:
		if cmd.Priority != 3 {
			t.Fatalf("activation cmd priority = %d, want 3", cmd.Priority)
		}
	case <-time.After(time.Second):
		t.Fatalf("replan continuation activation was not scheduled")
	}
}

func TestServerStateReplanContinuationRetriesAfterActivationFailure(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	const workspaceID = "ws-replan-retry"
	const runID = "run-replan-retry"
	firstAttempt := make(chan struct{}, 1)
	secondAttempt := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan-retry",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	var calls atomic.Int32
	state.activateWorkspace = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		switch calls.Add(1) {
		case 1:
			firstAttempt <- struct{}{}
			return appservice.WorkspaceExecutionActivation{}, fmt.Errorf("transient activation failure")
		case 2:
			secondAttempt <- cmd
			return appservice.WorkspaceExecutionActivation{
				Run: appservice.Run{ID: "run-replan-retry-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
			}, nil
		default:
			t.Fatalf("unexpected activation retry count")
			return appservice.WorkspaceExecutionActivation{}, nil
		}
	}

	state.scheduleReplanContinuationIfNeeded(context.Background(), workspaceID, runID, 5)
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatalf("first replan continuation activation was not attempted")
	}
	waitForCondition(t, time.Second, func() bool {
		state.replanContinuationMu.Lock()
		defer state.replanContinuationMu.Unlock()
		_, ok := state.replanContinuationStarted[workspaceID+"/"+runID]
		return !ok
	})

	state.scheduleReplanContinuationIfNeeded(context.Background(), workspaceID, runID, 7)
	select {
	case cmd := <-secondAttempt:
		if cmd.Priority != 7 {
			t.Fatalf("retry activation priority = %d, want 7", cmd.Priority)
		}
	case <-time.After(time.Second):
		t.Fatalf("replan continuation activation was not retried after failure")
	}
}

func TestServerStateReplanContinuationWatcherUsesSharedSessionEventHub(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	if state.eventHub == nil {
		t.Fatalf("server state eventHub is nil, want shared session event hub")
	}
	activated := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan-watcher",
				WorkspaceID:    "ws-replan-watcher",
				RunID:          "run-replan-watcher",
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.activateWorkspace = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if workspaceID != "ws-replan-watcher" {
			t.Fatalf("workspaceID = %q, want ws-replan-watcher", workspaceID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-replan-watcher-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       events.TopicRunStateChanged,
		WorkspaceID: "ws-replan-watcher",
		RunID:       "run-replan-watcher",
		SubjectID:   "run-replan-watcher",
		Payload:     events.RunPayload("", "", common.RunStateSuperseded, "", "", ""),
	}); err != nil {
		t.Fatalf("publish superseded run event: %v", err)
	}

	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatalf("replan continuation activation was not scheduled from shared session event hub")
	}
}

func TestServerStateAutoGovernanceResolvedSchedulesResumeAndReplanContinuation(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	if state.eventHub == nil {
		t.Fatalf("server state eventHub is nil, want shared session event hub")
	}
	resumed := make(chan string, 1)
	activated := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.resumeRun = func(_ context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
		if runID != "run-auto-replan" {
			t.Fatalf("runID = %q, want run-auto-replan", runID)
		}
		resumed <- runID
		return appservice.Run{ID: runID, WorkspaceID: "ws-auto-replan", State: string(common.RunStateSuperseded)}, nil
	}
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-auto-replan",
				WorkspaceID:    "ws-auto-replan",
				RunID:          "run-auto-replan",
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.activateWorkspace = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if workspaceID != "ws-auto-replan" {
			t.Fatalf("workspaceID = %q, want ws-auto-replan", workspaceID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-auto-replan-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       events.TopicGovernanceAutoResolved,
		WorkspaceID: "ws-auto-replan",
		RunID:       "run-auto-replan",
		SubjectID:   "ticket-auto-replan/auto-governance",
		Payload:     map[string]any{"status": "resolved"},
	}); err != nil {
		t.Fatalf("publish auto governance resolved event: %v", err)
	}

	select {
	case <-resumed:
	case <-time.After(time.Second):
		t.Fatalf("auto governance resolved event did not schedule run resume")
	}
	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatalf("auto governance replan resume did not schedule continuation")
	}
}

func TestServerStateRecoverReplanContinuationsSchedulesSupersededReplanRuns(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	activated := make(chan string, 2)
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{
			{ID: "ws-replan-recover", State: string(common.WorkspaceStateFailed)},
			{ID: "ws-replan-active", ActiveRunID: "run-active", State: string(common.WorkspaceStateRunning)},
			{ID: "ws-replan-completed", State: string(common.WorkspaceStateCompleted)},
		}, nil
	}
	state.listRuns = func(_ context.Context, _, _, workspaceID string) ([]appservice.Run, error) {
		switch workspaceID {
		case "ws-replan-recover":
			return []appservice.Run{
				{ID: "run-replan-recover", WorkspaceID: workspaceID, State: string(common.RunStateSuperseded), Priority: 4},
				{ID: "run-failed-no-replan", WorkspaceID: workspaceID, State: string(common.RunStateFailed)},
			}, nil
		case "ws-replan-active", "ws-replan-completed":
			t.Fatalf("workspace %s should not be scanned for replan continuation", workspaceID)
		}
		return nil, nil
	}
	state.listDecisions = func(_ context.Context, _, _, workspaceID, runID string) ([]appservice.Decision, error) {
		if workspaceID != "ws-replan-recover" || runID != "run-replan-recover" {
			return nil, nil
		}
		return []appservice.Decision{
			{
				ID:             "ticket-replan-recover",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.activateWorkspace = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if cmd.Priority != 4 {
			t.Fatalf("activation priority = %d, want 4", cmd.Priority)
		}
		activated <- workspaceID
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-replan-recover-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	state.recoverReplanContinuations(context.Background())

	select {
	case workspaceID := <-activated:
		if workspaceID != "ws-replan-recover" {
			t.Fatalf("activated workspace = %q, want ws-replan-recover", workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("replan continuation recovery did not schedule activation")
	}
}

func TestServerStateGovernanceRunResumeCoalescesDuplicateRunID(t *testing.T) {
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	state.resumeRun = func(_ context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
		if runID != "run-duplicate-resume" {
			t.Fatalf("runID = %q, want run-duplicate-resume", runID)
		}
		calls.Add(1)
		started <- struct{}{}
		<-release
		return appservice.Run{ID: runID, State: string(common.RunStateCompleted)}, nil
	}

	state.scheduleGovernanceRunResumes([]string{"run-duplicate-resume"}, 0)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("initial governance resume did not start")
	}
	state.scheduleGovernanceRunResumes([]string{"run-duplicate-resume", "run-duplicate-resume"}, 0)
	time.Sleep(50 * time.Millisecond)
	if got := state.metrics.runQueueWaiting.Load(); got != 0 {
		t.Fatalf("runQueueWaiting = %d, want duplicate governance resume coalesced", got)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want one in-flight call", got)
	}

	close(release)
	waitForCondition(t, time.Second, func() bool {
		state.governanceResumeMu.Lock()
		_, stillMarked := state.governanceResumeStarted["run-duplicate-resume"]
		state.governanceResumeMu.Unlock()
		return calls.Load() == 1 && state.metrics.runQueueWaiting.Load() == 0 && !stillMarked
	})
	state.governanceResumeMu.Lock()
	_, stillMarked := state.governanceResumeStarted["run-duplicate-resume"]
	state.governanceResumeMu.Unlock()
	if stillMarked {
		t.Fatalf("governance resume mark was not cleared after completion")
	}
}

func TestServerStateRecoverReplanContinuationsRetriesAfterActivationFailure(t *testing.T) {
	state := newServerState(normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	const workspaceID = "ws-replan-recover-retry"
	const runID = "run-replan-recover-retry"
	firstAttempt := make(chan struct{}, 1)
	secondAttempt := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{{ID: workspaceID, State: string(common.WorkspaceStateFailed)}}, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateSuperseded), Priority: 9}}, nil
	}
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan-recover-retry",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	var calls atomic.Int32
	state.activateWorkspace = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		switch calls.Add(1) {
		case 1:
			firstAttempt <- struct{}{}
			return appservice.WorkspaceExecutionActivation{}, fmt.Errorf("transient activation failure")
		case 2:
			secondAttempt <- cmd
			return appservice.WorkspaceExecutionActivation{
				Run: appservice.Run{ID: "run-replan-recover-retry-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
			}, nil
		default:
			t.Fatalf("unexpected activation retry count")
			return appservice.WorkspaceExecutionActivation{}, nil
		}
	}

	state.recoverReplanContinuations(context.Background())
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatalf("first recovered replan continuation activation was not attempted")
	}
	waitForCondition(t, time.Second, func() bool {
		state.replanContinuationMu.Lock()
		defer state.replanContinuationMu.Unlock()
		_, ok := state.replanContinuationStarted[workspaceID+"/"+runID]
		return !ok
	})

	state.recoverReplanContinuations(context.Background())
	select {
	case cmd := <-secondAttempt:
		if cmd.Priority != 9 {
			t.Fatalf("retry activation priority = %d, want 9", cmd.Priority)
		}
	case <-time.After(time.Second):
		t.Fatalf("recovered replan continuation activation was not retried after failure")
	}
}

func TestApprovalBatchResolveEndpointApprovesPendingApprovals(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	firstRunID := "run-approval-batch-a"
	secondRunID := "run-approval-batch-b"

	_, first := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval batch http workspace a", firstRunID)
	_, second := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval batch http workspace b", secondRunID)

	body := strings.NewReader(`{"approvalIds":["` + second.BlockingApprovalID + `","` + first.BlockingApprovalID + `"],"action":"approve"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/resolve-batch", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch approval resolve status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resolved approvalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal batch resolved approvals: %v", err)
	}
	if len(resolved.Approvals) != 2 ||
		resolved.Approvals[0].ID != second.BlockingApprovalID ||
		resolved.Approvals[1].ID != first.BlockingApprovalID {
		t.Fatalf("resolved approvals = %+v, want request order", resolved.Approvals)
	}
	for _, item := range resolved.Approvals {
		if item.State != "approved" {
			t.Fatalf("approval = %+v, want approved", item)
		}
	}
	waitRunStateForHTTP(t, dataRoot, firstRunID, string(common.RunStateFailed))
	waitRunStateForHTTP(t, dataRoot, secondRunID, string(common.RunStateFailed))
}

func TestApprovalListAndGetEndpointsReturnPersistedApprovals(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runA := "run-approval-list-a"
	runB := "run-approval-list-b"

	_, startedA := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval list http workspace a", runA)
	workspaceB, startedB := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval list http workspace b", runB)

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
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-approval-approve-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval approve http workspace", runID)

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

	resumed := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
	if resumed.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want cleared approval blocker", resumed)
	}
}

func TestApprovalRejectEndpointRejectsApprovalAndRunResumeFailsTerminally(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-approval-reject-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval reject http workspace", runID)

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

	resumed := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
	if resumed.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed)
	}
}

func TestApprovalApproveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-approval-conflict-http"

	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval conflict http workspace", runID)

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
	waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateFailed))
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

	blockedRunID := "run-decision-metrics"
	_, blockedRun := seedBlockedDecisionRunForHTTP(t, dataRoot, "metrics blocked workspace", blockedRunID)
	resolveDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+blockedRun.BlockingTicketID+"/resolve", nil)
	resolveDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveDecisionRec, resolveDecisionReq)
	if resolveDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200", resolveDecisionRec.Code)
	}
	waitRunStateForHTTP(t, dataRoot, blockedRunID, string(common.RunStateFailed))

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
		`workorch_http_requests_total{endpoint="workspace_create"} 1`,
		`workorch_http_requests_total{endpoint="workspace_list"} 1`,
		`workorch_http_requests_total{endpoint="workspace_get"} 1`,
		`workorch_http_requests_total{endpoint="workspace_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_create"} 1`,
		`workorch_http_requests_total{endpoint="run_list"} 1`,
		`workorch_http_requests_total{endpoint="run_get"} 1`,
		`workorch_http_requests_total{endpoint="run_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_status"} 1`,
		`workorch_http_requests_total{endpoint="run_start"} 1`,
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
		`workorch_run_outcomes_total{status="succeeded"} 0`,
		`workorch_run_outcomes_total{status="failed"} 1`,
		`workorch_run_outcomes_total{status="canceled"} 0`,
		`workorch_server_max_concurrent_runs 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

func TestHTTPServerStartupRecoversFileBackedStuckRunningRunInBackground(t *testing.T) {
	dataRoot := t.TempDir()
	useWorkorchRuntimeTestConfig(t, "file", dataRoot)
	ctx := context.Background()
	first := NewHTTPServer(Config{Backend: "file", DataRoot: dataRoot})
	firstHandler := first.http.Handler
	workspace := createWorkspaceForHTTP(t, firstHandler, "stuck running validation workspace", "workspaces/stuck-running-validation")
	goalID, dodID := defineGoalAndDoDForHTTP(t, firstHandler, workspace.ID)
	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-stuck-validation","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	firstHandler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}

	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, "run-stuck-validation")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateRunning
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	second := NewHTTPServer(Config{Backend: "file", DataRoot: dataRoot})
	if err := second.Validate(ctx); err != nil {
		t.Fatalf("validate second server: %v", err)
	}
	getRun := func() appservice.Run {
		t.Helper()
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-stuck-validation", nil)
		getRec := httptest.NewRecorder()
		second.http.Handler.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
		}
		var body runInstanceResponse
		if err := json.Unmarshal(getRec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal run: %v", err)
		}
		return body.Run
	}
	if run := getRun(); run.State != string(common.RunStateRunning) {
		t.Fatalf("run state after Validate = %s, want running until startup background recovery", run.State)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	second.http.Addr = ln.Addr().String()
	done := make(chan error, 1)
	go func() {
		done <- second.http.Serve(ln)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := second.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown second server: %v", err)
		}
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Fatalf("second server stopped: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("second server did not stop")
		}
	}()
	second.state.scheduleStartupRecovery()

	var recovered appservice.Run
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recovered = getRun()
		if recovered.State == "failed" && recovered.FailureReason != "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-stuck-validation", nil)
	getRec := httptest.NewRecorder()
	second.http.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if body.Run.State != "failed" || body.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed recovered run", body.Run)
	}
}

func createWorkspaceForHTTP(t *testing.T, handler http.Handler, title string, deliverableDirectory string) appservice.Workspace {
	t.Helper()

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"`+title+`","deliverableDirectory":"`+deliverableDirectory+`"}`))
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
	if body.Workspace.ID == "" || body.Workspace.Title != title || body.Workspace.DeliverableDirectory != deliverableDirectory || body.Workspace.State != "active" {
		t.Fatalf("workspace = %+v, want generated/%s/%s/active", body.Workspace, title, deliverableDirectory)
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

func putAndExpect(t *testing.T, handler http.Handler, path string, body string, wantStatus int) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d, body = %s", path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec.Body.String()
}

func deleteAndExpect(t *testing.T, handler http.Handler, path string, wantStatus int) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, path, nil)
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

func seedBlockedDecisionRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appservice.Workspace, appservice.Run) {
	t.Helper()
	workspace, run := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run = seedBlockedDecisionRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID)
	return workspace, run
}

func seedBlockedDecisionRunInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string) appservice.Run {
	t.Helper()
	ticketID := common.TicketID("ticket-" + runID + "-decision")
	stepID := common.StepID(runID + "/decision")
	ctx := context.Background()
	runScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	ticketRepo := runScopedRepos.Tickets
	if existing, err := runRepo.Get(ctx, common.RunID(runID)); err == nil {
		existing.State = common.RunStateBlockedOnDecision
		existing.BlockingTicketID = ticketID
		existing.BlockingApprovalID = ""
		if err := runRepo.Update(ctx, existing); err != nil {
			t.Fatalf("update decision run: %v", err)
		}
	} else {
		if err := runRepo.Create(ctx, &runmodel.Run{
			ID:               common.RunID(runID),
			WorkspaceID:      common.WorkspaceID(workspaceID),
			State:            common.RunStateBlockedOnDecision,
			BlockingTicketID: ticketID,
		}); err != nil {
			t.Fatalf("create run: %v", err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_decision"},
	}); err != nil {
		t.Fatalf("create decision step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  common.WorkspaceID(workspaceID),
		RunID:        common.RunID(runID),
		StepID:       stepID,
		Reason:       "test decision gate",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "provide test input",
		Context:      map[string]any{"requiredFields": []any{"input"}},
		State:        common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create decision ticket: %v", err)
	}
	item, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get decision run: %v", err)
	}
	return appservice.Run{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		State:            string(item.State),
		BlockingTicketID: string(item.BlockingTicketID),
	}
}

func seedBlockedApprovalRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appservice.Workspace, appservice.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedApprovalRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID)
	return workspace, run
}

func seedBlockedApprovalRunInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string) appservice.Run {
	t.Helper()
	approvalID := common.ApprovalID("approval-" + runID + "-approval")
	stepID := common.StepID(runID + "/approval")
	ctx := context.Background()
	runScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	approvalRepo := runScopedRepos.Approvals
	if existing, err := runRepo.Get(ctx, common.RunID(runID)); err == nil {
		existing.State = common.RunStateBlockedOnApproval
		existing.BlockingApprovalID = approvalID
		existing.BlockingTicketID = ""
		if err := runRepo.Update(ctx, existing); err != nil {
			t.Fatalf("update approval run: %v", err)
		}
	} else {
		if err := runRepo.Create(ctx, &runmodel.Run{
			ID:                 common.RunID(runID),
			WorkspaceID:        common.WorkspaceID(workspaceID),
			State:              common.RunStateBlockedOnApproval,
			BlockingApprovalID: approvalID,
		}); err != nil {
			t.Fatalf("create run: %v", err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_approval"},
	}); err != nil {
		t.Fatalf("create approval step: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       common.RunID(runID),
		StepID:      stepID,
		Reason:      "test approval gate",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	item, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get approval run: %v", err)
	}
	return appservice.Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		BlockingApprovalID: string(item.BlockingApprovalID),
	}
}

func seedBlockedRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appservice.Workspace, appservice.Run) {
	t.Helper()
	ctx := context.Background()
	workspace := &workspacemodel.Workspace{
		ID:                   common.WorkspaceID("ws-" + strings.TrimPrefix(runID, "run-")),
		Title:                title,
		DeliverableDirectory: "workspaces/" + runID,
		State:                common.WorkspaceStateActive,
	}
	runScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
	if err := runScopedRepos.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	runRepo := runScopedRepos.Runs
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          common.RunID(runID),
		WorkspaceID: workspace.ID,
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	run, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	return appservice.Workspace{ID: string(workspace.ID), Title: workspace.Title, DeliverableDirectory: workspace.DeliverableDirectory, State: string(workspace.State)}, appservice.Run{ID: string(run.ID), WorkspaceID: string(run.WorkspaceID), State: string(run.State)}
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

func readEventRecordsForHTTP(t *testing.T, handler http.Handler, path string) []appservice.EventRecord {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200, body=%s", path, rec.Code, rec.Body.String())
	}
	var body eventRecordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal event records: %v", err)
	}
	return body.EventRecords
}

func eventRecordsContainTopic(items []appservice.EventRecord, topic string) bool {
	for _, item := range items {
		if item.Topic == topic {
			return true
		}
	}
	return false
}

func timelineContainsPersistedEvent(items []appservice.RuntimeTimelineItem, topic string) bool {
	for _, item := range items {
		if item.Source == "persisted_event" && item.Topic == topic {
			return true
		}
	}
	return false
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

func waitRunStateForHTTP(t *testing.T, dataRoot string, runID string, wantState string) appservice.Run {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var last appservice.Run
	var lastErr error
	for time.Now().Before(deadline) {
		item, err := appservice.ReadRunWithBackend(context.Background(), "file", dataRoot, runID)
		if err == nil {
			last = item
			if item.State == wantState {
				return item
			}
		} else {
			lastErr = err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("run %s did not reach %s: last error %v", runID, wantState, lastErr)
	}
	t.Fatalf("run %s state = %s, want %s; run=%+v", runID, last.State, wantState, last)
	return last
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

func TestRunStartEndpointDispatchesHigherPriorityQueuedRunFirst(t *testing.T) {
	heldStarted := make(chan struct{}, 1)
	highStarted := make(chan struct{}, 1)
	lowStarted := make(chan struct{}, 1)
	releaseHeld := make(chan struct{})
	releaseHigh := make(chan struct{})
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	state.validate = func(context.Context, string, string) error { return nil }
	state.startRun = func(_ context.Context, _, _, runID string) (appservice.Run, error) {
		switch runID {
		case "run-held":
			heldStarted <- struct{}{}
			<-releaseHeld
		case "run-high":
			highStarted <- struct{}{}
			<-releaseHigh
		case "run-low":
			lowStarted <- struct{}{}
		}
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	errs := make(chan error, 3)
	postStart := func(path string) {
		resp, err := http.Post(server.URL+path, "application/json", nil)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errs <- fmt.Errorf("%s status = %d, want 200", path, resp.StatusCode)
			return
		}
		errs <- nil
	}

	go postStart("/api/v1/workorch/runs/run-held/start")
	<-heldStarted
	go postStart("/api/v1/workorch/runs/run-low/start?priority=1")
	go postStart("/api/v1/workorch/runs/run-high/start?priority=10")
	waitForRunQueueWaiting(t, state, 2)

	close(releaseHeld)
	select {
	case <-highStarted:
	case <-lowStarted:
		t.Fatal("low priority run started before high priority run")
	case <-time.After(time.Second):
		t.Fatal("high priority run did not start")
	}
	select {
	case <-lowStarted:
		t.Fatal("low priority run started while high priority run still holds slot")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseHigh)
	select {
	case <-lowStarted:
	case <-time.After(time.Second):
		t.Fatal("low priority run did not start after high priority run released slot")
	}
	for idx := 0; idx < 3; idx++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func waitForRunQueueWaiting(t *testing.T, state *serverState, want int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if state.metrics.runQueueWaiting.Load() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runQueueWaiting = %d, want %d", state.metrics.runQueueWaiting.Load(), want)
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
	state.resumeRun = func(ctx context.Context, backend string, dataRoot string, runID string, _ ...map[string]any) (appservice.Run, error) {
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
