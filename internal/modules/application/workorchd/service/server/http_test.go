package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	magictask "github.com/muidea/magicCommon/task"
	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
	"github.com/muidea/workorch/web/consoleassets"
)

func TestMain(m *testing.M) {
	originalWD, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get test working directory: %v\n", err)
		os.Exit(1)
	}
	testWD, err := os.MkdirTemp("", "workorch-server-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated test working directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(testWD); err != nil {
		fmt.Fprintf(os.Stderr, "switch to isolated test working directory: %v\n", err)
		_ = os.RemoveAll(testWD)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.Chdir(originalWD)
	_ = os.RemoveAll(testWD)
	os.Exit(code)
}

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

func skipObsoleteSynchronousExecutionEndpointTest(t *testing.T) {
	t.Helper()
	t.Skip("execution-driving REST endpoints are asynchronous EventHub scheduler commands and return accepted; final run state is verified through scheduler/projection tests")
}

func newTestServerState(t *testing.T, config Config, hub events.Hub) *serverState {
	t.Helper()
	routine := magictask.NewBackgroundRoutine(128)
	state := newServerStateWithBackground(config, hub, routine)
	configureIsolatedTestSession(t, state, config)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(ctx)
		_ = routine.Shutdown(ctx)
	})
	return state
}

func configureIsolatedTestSession(t *testing.T, state *serverState, config Config) {
	t.Helper()
	if state == nil || state.sharedSession != nil {
		return
	}
	session, err := appservice.NewRuntimeSessionWithBackendAndEventHub(config.Backend, config.DataRoot, state.eventHub)
	if err != nil {
		return
	}
	for _, unsubscribe := range state.eventUnsubscribes {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	state.eventUnsubscribes = nil
	state.configureSessionBackedOperations(session)
	state.registerEventReplayRecorder()
	state.registerRunStatusCacheInvalidationWatcher()
	state.registerGovernanceResolvedResumeWatcher()
	state.registerWorkorchDSchedulerCommandWatcher()
}

func newTestMux(t *testing.T, config Config) http.Handler {
	t.Helper()
	return newMuxWithState(newTestServerState(t, normalizeConfig(config), nil))
}

func newTestHTTPServer(t *testing.T, config Config) *HTTPServer {
	t.Helper()
	routine := magictask.NewBackgroundRoutine(128)
	server := NewHTTPServerWithEventHubAndBackground(config, nil, routine)
	configureIsolatedTestSession(t, server.state, normalizeConfig(config))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown test http server: %v", err)
		}
		_ = routine.Shutdown(ctx)
	})
	return server
}

func publishHTTPTestCommand(ctx context.Context, hub events.Hub, envelope events.Envelope) error {
	err := hub.Publish(ctx, envelope)
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	return err
}

func readSSELines(body io.Reader) (<-chan string, <-chan error) {
	lines := make(chan string, 16)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		readErr <- scanner.Err()
	}()
	return lines, readErr
}

func waitSSEConnected(t *testing.T, lines <-chan string, readErr <-chan error) {
	t.Helper()
	select {
	case line := <-lines:
		if !strings.HasPrefix(line, ": connected") {
			t.Fatalf("first stream line = %q, want connected comment", line)
		}
	case err := <-readErr:
		t.Fatalf("stream closed before connected comment: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream connection")
	}
}

func readSSEEnvelope(t *testing.T, lines <-chan string, readErr <-chan error) (string, events.Envelope) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	var id string
	for {
		select {
		case line := <-lines:
			switch {
			case strings.HasPrefix(line, "id:"):
				id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "data:"):
				var envelope events.Envelope
				if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &envelope); err != nil {
					t.Fatalf("decode stream data %q: %v", line, err)
				}
				return id, envelope
			}
		case err := <-readErr:
			t.Fatalf("stream closed before event: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for streamed event")
		}
	}
}

func waitWorkspaceCreatedSSE(t *testing.T, lines <-chan string, readErr <-chan error, title string) events.Envelope {
	t.Helper()
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
			if envelope.Topic == workspaceevents.TopicWorkspaceCreated && envelope.Payload[events.PayloadKeyTitle] == title {
				return envelope
			}
		case err := <-readErr:
			t.Fatalf("stream closed before workspace-created event: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for streamed workspace-created event")
		}
	}
}

func numericTestValue(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func newFileMuxWithCleanup(t *testing.T, dataRoot string) http.Handler {
	t.Helper()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
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

	newTestMux(t, Config{}).ServeHTTP(rec, req)

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

func TestPprofRoutesAreExposed(t *testing.T) {
	handler := newTestMux(t, Config{})
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/goroutine?debug=1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s body is empty", path)
		}
	}
}

func TestHTTPServerShutdownForceClosesAfterDeadline(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	srv := &HTTPServer{
		state: &serverState{},
		http: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(handlerStarted)
				<-releaseHandler
				_, _ = w.Write([]byte("ok"))
			}),
		},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.http.Serve(listener)
	}()
	clientDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if resp != nil {
			_ = resp.Body.Close()
		}
		clientDone <- err
	}()
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = srv.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown elapsed = %v, want bounded return", elapsed)
	}
	select {
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("http server did not stop after forced close")
	}
	close(releaseHandler)
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("client request did not finish after releasing handler")
	}
}

func TestReadyzReturnsServiceUnavailableWhenBackendInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	newTestMux(t, Config{Backend: "unsupported"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestReadyzNativeRouteInjectsEventHub(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	state.validate = func(ctx context.Context, _, _ string) error {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("event hub from context = %#v, want injected hub", got)
		}
		return nil
	}
	if err := state.workspaceProjection.RebuildAll(context.Background()); err != nil {
		t.Fatalf("rebuild workspace projection baseline: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadyzIncludesSchedulerDiagnostics(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), events.NewMemoryHub())
	state.schedulerDiagnostics.recordWatchdogTimerRegistration()
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    "ws-readyz-diagnostics",
		RunID:          "run-readyz-diagnostics",
		Kind:           schedulerKindReplanContinuation,
		State:          schedulerDiagnosticStateFailed,
		Reason:         schedulerReasonOwnerCommandFailed,
		Detail:         "owner command returned an explicit failure",
		Recoverability: schedulerRecoverabilityManualRetry,
	})
	if err := state.workspaceProjection.RebuildAll(context.Background()); err != nil {
		t.Fatalf("rebuild workspace projection baseline: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readyz response: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("readyz status = %q, want ready", body.Status)
	}
	if body.Diagnostics == nil || body.Diagnostics.LatestContinuation == nil {
		t.Fatalf("readyz diagnostics = %+v, want latest scheduler continuation", body.Diagnostics)
	}
	if body.Diagnostics.Watchdog.TimerRegistrations != 1 {
		t.Fatalf("readyz watchdog timer registrations = %d, want 1", body.Diagnostics.Watchdog.TimerRegistrations)
	}
	latest := body.Diagnostics.LatestContinuation
	if latest.WorkspaceID != "ws-readyz-diagnostics" ||
		latest.RunID != "run-readyz-diagnostics" ||
		latest.Kind != schedulerKindReplanContinuation ||
		latest.State != schedulerDiagnosticStateFailed ||
		latest.Reason != schedulerReasonOwnerCommandFailed {
		t.Fatalf("latest diagnostics = %+v, want recorded replan failure", latest)
	}
}

func TestServerStateRequiresFrameworkInitiatorRuntimeSession(t *testing.T) {
	useWorkorchRuntimeTestConfig(t, "memory", "")
	bg := magictask.NewBackgroundRoutine(128)
	ctx := context.Background()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = bg.Shutdown(shutdownCtx)
	})

	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, bg)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(shutdownCtx)
	})
	if state.sharedSession != nil {
		t.Fatal("server state must not create an owned bootstrap runtime when framework initiator is missing")
	}
	err := state.validate(ctx, "", "")
	if err == nil {
		t.Fatal("validate error = nil, want missing framework runtime session error")
	}
	if !strings.Contains(err.Error(), "framework runtime session unavailable") {
		t.Fatalf("validate error = %v, want framework runtime session unavailable", err)
	}
}

func TestConfigurationEndpointReturnsRedactedLLMProviders(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

func TestUpdateRuntimeConfigurationEndpointSyncsOrchestratorPolicy(t *testing.T) {
	useWorkorchRuntimeTestConfig(t, "file", t.TempDir())
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: t.TempDir()}), nil)
	state.eventHub = events.NewMemoryHub()
	var gotPolicy workorchruntimepolicy.RuntimePolicy
	state.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandUpdateRuntimePolicy, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(orchestratorcontracts.UpdateRuntimePolicyCommand)
		if !ok {
			t.Fatalf("runtime policy command payload = %#v, want UpdateRuntimePolicyCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		gotPolicy = cmd.Policy
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/response",
			SourceModule: "test",
			SubjectType:  envelope.SubjectType,
			SubjectID:    envelope.SubjectID,
		})
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/configuration/runtime", strings.NewReader(`{
		"maxRuntimeRepairAttempts":3,
		"shellCommandTimeout":"30s"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var body configurationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal configuration: %v", err)
	}
	if body.Configuration.Runtime.MaxRuntimeRepairAttempts != 3 ||
		body.Configuration.Runtime.ShellCommandTimeout != "30s" ||
		gotPolicy.MaxRuntimeRepairAttempts != 3 {
		t.Fatalf("configuration=%+v policy=%+v, want runtime repair attempts synced to 3", body.Configuration.Runtime, gotPolicy)
	}
}

func TestMemoryBackendPersistsStateAcrossHTTPRequests(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	mux := newTestMux(t, Config{Backend: "memory"})

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

	startRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-memory-http/start", nil)
	startRunRec := httptest.NewRecorder()
	mux.ServeHTTP(startRunRec, startRunReq)
	if startRunRec.Code != http.StatusOK {
		t.Fatalf("start run status = %d, want 200, body = %s", startRunRec.Code, startRunRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-memory-http/status", nil)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("run status code = %d, want 200, body = %s", statusRec.Code, statusRec.Body.String())
	}
	if !strings.Contains(statusRec.Body.String(), `"state": "blocked_on_decision"`) ||
		!strings.Contains(statusRec.Body.String(), `"blockingTicketId": "run-memory-http-step-1-provider-retry-exhausted-decision"`) {
		t.Fatalf("run status body = %s, want provider decision blocker without configured LLM provider", statusRec.Body.String())
	}

	observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-memory-http/observability", nil)
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
	mux := newTestMux(t, Config{Backend: "memory"})

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

func TestWorkspaceScopedMemoryItemRoutesBindWorkspaceScope(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "memory"})
	workspaceA := createWorkspaceForHTTP(t, handler, "memory scope A", "workspaces/ws-memory-scope-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "memory scope B", "workspaces/ws-memory-scope-b")
	memoryID := "memory-http-scope-shared"

	postAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory", `{
		"memoryId":"`+memoryID+`",
		"sourceType":"user",
		"memoryType":"project_context",
		"summary":"workspace A scoped memory"
	}`, http.StatusCreated)
	postAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/memory", `{
		"memoryId":"`+memoryID+`",
		"sourceType":"user",
		"memoryType":"project_context",
		"summary":"workspace B scoped memory"
	}`, http.StatusCreated)

	getA := getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, http.StatusOK)
	if !strings.Contains(getA, `"summary": "workspace A scoped memory"`) || strings.Contains(getA, `"workspace B scoped memory"`) {
		t.Fatalf("workspace A memory response = %s", getA)
	}

	putAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, `{
		"sourceType":"user",
		"memoryType":"project_context",
		"summary":"workspace A updated memory"
	}`, http.StatusOK)

	getB := getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/memory/"+memoryID, http.StatusOK)
	if !strings.Contains(getB, `"summary": "workspace B scoped memory"`) || strings.Contains(getB, `"workspace A updated memory"`) {
		t.Fatalf("workspace B memory response = %s", getB)
	}

	getAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID, http.StatusBadRequest)
	getAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID+"?workspaceId="+workspaceB.ID, http.StatusOK)

	deleteAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, http.StatusOK)
	getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, http.StatusNotFound)
	getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/memory/"+memoryID, http.StatusOK)
}

func TestWorkspaceExecutionActivationEndpointCreatesAndStartsRun(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	useUnavailableLLMProvider(t)
	mux := newTestMux(t, Config{Backend: "memory"})

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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	useUnavailableLLMProvider(t)
	mux := newTestMux(t, Config{Backend: "memory"})

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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	registerSystemRoutes(router, state)
	registerConfigurationRoutes(router, state)
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
		{method: http.MethodGet, pattern: "/debug/pprof/**"},
		{method: http.MethodGet, pattern: "/healthz"},
		{method: http.MethodGet, pattern: "/readyz"},
		{method: http.MethodGet, pattern: "/metrics"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/configuration"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodDelete, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/snapshot"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory-recalls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals/:goalId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals/:goalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods/:dodId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods/:dodId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-activations"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/template-binding-refresh"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/runs"},
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
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId"},
		{method: http.MethodDelete, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/memory"},
		{method: http.MethodGet, pattern: "/favicon.ico"},
		{method: http.MethodGet, pattern: "/console"},
		{method: http.MethodGet, pattern: "/console/"},
		{method: http.MethodGet, pattern: "/console/**"},
	}
}

func TestMemoryRecallTraceEndpointsReturnFilteredTraces(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
		"/api/v1/workorch/workspaces/ws-recall-http/runs/run-recall-http/memory-recalls",
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream&view=full", nil)
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
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-other",
		Payload:     events.WorkspacePayload("other", "", "/tmp/other", "", common.WorkspaceStateActive, ""),
	}); err != nil {
		t.Fatalf("publish nonmatching event: %v", err)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceCreated,
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
			if envelope.WorkspaceID != "ws-stream" || envelope.Topic != workspaceevents.TopicWorkspaceCreated {
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

func TestEventStreamDefaultProjectionViewPublishesOnlyProjectionEvents(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-projection", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	lines, readErr := readSSELines(resp.Body)
	waitSSEConnected(t, lines, readErr)

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       runevents.TopicStepStateChanged,
		WorkspaceID: "ws-projection",
		RunID:       "run-heavy",
		StepID:      "run-heavy/step-001",
		Payload: map[string]any{
			"state": "completed",
			"input": strings.Repeat("large runtime payload", 4096),
		},
	}); err != nil {
		t.Fatalf("publish heavy step event: %v", err)
	}

	select {
	case line := <-lines:
		if strings.HasPrefix(line, "data:") {
			t.Fatalf("projection stream emitted raw step data line: %.120s", line)
		}
	case err := <-readErr:
		t.Fatalf("stream closed before projection event: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-projection",
		Payload: map[string]any{
			"type":              "workspace_projection_changed",
			"workspaceId":       "ws-projection",
			"workspaceRevision": 7,
		},
	}); err != nil {
		t.Fatalf("publish projection event: %v", err)
	}

	_, envelope := readSSEEnvelope(t, lines, readErr)
	if envelope.Topic != workorchdevents.TopicWorkspaceProjectionChanged || envelope.WorkspaceID != "ws-projection" {
		t.Fatalf("projection envelope = %+v, want projection change for ws-projection", envelope)
	}
	if _, exists := envelope.Payload["input"]; exists {
		t.Fatalf("projection payload unexpectedly contains raw step input: %+v", envelope.Payload)
	}
}

func TestEventStreamWritesIDRevisionAndStateSummary(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream-summary&view=full", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	lines, readErr := readSSELines(resp.Body)
	waitSSEConnected(t, lines, readErr)

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-summary",
		Payload:     events.WorkspacePayload("summary", "", "/tmp/summary", "", common.WorkspaceStateRunning, "run-001"),
	}); err != nil {
		t.Fatalf("publish state event: %v", err)
	}

	id, envelope := readSSEEnvelope(t, lines, readErr)
	if id == "" || envelope.ID == "" || id != envelope.ID {
		t.Fatalf("sse id = %q envelope id = %q, want matching non-empty ids", id, envelope.ID)
	}
	if envelope.Revision != 1 {
		t.Fatalf("revision = %d, want 1", envelope.Revision)
	}
	summary, _ := envelope.Payload["stateSummary"].(map[string]any)
	if summary["workspaceState"] != "running" || summary["activeRunId"] != "run-001" || numericTestValue(summary["revision"]) != 1 {
		t.Fatalf("stateSummary = %+v, want workspace state, active run and revision", summary)
	}
}

func TestEventStreamReplaysAfterLastEventID(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream-replay&view=full", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	lines, readErr := readSSELines(resp.Body)
	waitSSEConnected(t, lines, readErr)

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-replay",
		Payload:     events.WorkspacePayload("first", "", "/tmp/first", "", common.WorkspaceStateRunning, ""),
	}); err != nil {
		t.Fatalf("publish first event: %v", err)
	}
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-stream-replay",
		RunID:       "run-replay",
		Payload:     events.RunPayload("", "", common.RunStateRunning, "", "", ""),
	}); err != nil {
		t.Fatalf("publish replay event: %v", err)
	}
	_ = resp.Body.Close()

	replayReq, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream-replay&view=full", nil)
	if err != nil {
		t.Fatalf("new replay request: %v", err)
	}
	replayReq.Header.Set("Accept", "text/event-stream")
	replayReq.Header.Set("Last-Event-ID", firstID)
	replayResp, err := http.DefaultClient.Do(replayReq)
	if err != nil {
		t.Fatalf("replay stream request: %v", err)
	}
	defer replayResp.Body.Close()
	replayLines, replayErr := readSSELines(replayResp.Body)
	waitSSEConnected(t, replayLines, replayErr)
	_, replayed := readSSEEnvelope(t, replayLines, replayErr)
	if replayed.Topic != runevents.TopicRunStateChanged || replayed.RunID != "run-replay" {
		t.Fatalf("replayed envelope = %+v, want run state event after Last-Event-ID", replayed)
	}
}

func TestEventStreamReplaysAfterLastEventIDQueryParam(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-stream-query-replay&view=full", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	lines, readErr := readSSELines(resp.Body)
	waitSSEConnected(t, lines, readErr)

	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-query-replay",
		Payload:     events.WorkspacePayload("first", "", "/tmp/first", "", common.WorkspaceStateRunning, ""),
	}); err != nil {
		t.Fatalf("publish first event: %v", err)
	}
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-stream-query-replay",
		RunID:       "run-query-replay",
		Payload:     events.RunPayload("", "", common.RunStateRunning, "", "", ""),
	}); err != nil {
		t.Fatalf("publish replay event: %v", err)
	}
	_ = resp.Body.Close()

	replayURL := server.URL + "/api/v1/workorch/events/stream?workspaceId=ws-stream-query-replay&view=full&lastEventId=" + url.QueryEscape(firstID)
	replayReq, err := http.NewRequestWithContext(ctx, http.MethodGet, replayURL, nil)
	if err != nil {
		t.Fatalf("new replay request: %v", err)
	}
	replayReq.Header.Set("Accept", "text/event-stream")
	replayResp, err := http.DefaultClient.Do(replayReq)
	if err != nil {
		t.Fatalf("replay stream request: %v", err)
	}
	defer replayResp.Body.Close()
	replayLines, replayErr := readSSELines(replayResp.Body)
	waitSSEConnected(t, replayLines, replayErr)
	_, replayed := readSSEEnvelope(t, replayLines, replayErr)
	if replayed.Topic != runevents.TopicRunStateChanged || replayed.RunID != "run-query-replay" {
		t.Fatalf("replayed envelope = %+v, want run state event after lastEventId query param", replayed)
	}
}

func TestEventStreamReplaysPersistedRecordsAfterLastEventID(t *testing.T) {
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	state.listEventRecords = func(_ context.Context, _, _ string, query appservice.EventRecordQuery) ([]appservice.EventRecord, error) {
		if query.WorkspaceID != "ws-persist" || query.RunID != "run-persist" || query.CursorAfterID != "evt-old" {
			t.Fatalf("query = %+v, want workspace/run cursor replay query", query)
		}
		return []appservice.EventRecord{{
			ID:          "evt-persisted-step",
			Version:     "workorch.event.v1",
			Topic:       runevents.TopicStepStateChanged,
			WorkspaceID: "ws-persist",
			RunID:       "run-persist",
			StepID:      "run-persist/step-1",
			Revision:    12,
			Payload: map[string]any{
				events.PayloadKeyState: string(common.StepStateCompleted),
			},
		}}, nil
	}
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-persist&runId=run-persist&view=full", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", "evt-old")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	lines, readErr := readSSELines(resp.Body)
	waitSSEConnected(t, lines, readErr)

	id, replayed := readSSEEnvelope(t, lines, readErr)
	if id != "evt-persisted-step" || replayed.Topic != runevents.TopicStepStateChanged || replayed.StepID != "run-persist/step-1" {
		t.Fatalf("replayed id=%q envelope=%+v, want persisted step state event", id, replayed)
	}
	if replayed.Payload[events.PayloadKeyState] != string(common.StepStateCompleted) {
		t.Fatalf("payload = %+v, want completed state", replayed.Payload)
	}
}

func TestWorkspaceCreateEndpointStreamsWorkspaceCreatedEvent(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?view=full", nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
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

	lines := make(chan string, 16)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		readErr <- scanner.Err()
	}()

	select {
	case line := <-lines:
		if !strings.HasPrefix(line, ": connected") {
			t.Fatalf("first stream line = %q, want connected comment", line)
		}
	case err := <-readErr:
		t.Fatalf("stream closed before connected comment: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream connection")
	}

	createResp, err := http.Post(server.URL+"/api/v1/workorch/workspaces", "application/json", strings.NewReader(`{"title":"streamed workspace","deliverableDirectory":"workspaces/streamed"}`))
	if err != nil {
		t.Fatalf("post workspace: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createResp.StatusCode)
	}

	envelope := waitWorkspaceCreatedSSE(t, lines, readErr, "streamed workspace")
	if envelope.WorkspaceID == "" {
		t.Fatalf("stream envelope = %+v, want workspace-created event for created workspace", envelope)
	}
}

func TestWorkspaceCreateEndpointStreamsWorkspaceCreatedEventToMultipleClients(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	openStream := func(name string) (<-chan string, <-chan error, func()) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?view=full", nil)
		if err != nil {
			t.Fatalf("%s new stream request: %v", name, err)
		}
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s stream request: %v", name, err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("%s stream status = %d, want 200", name, resp.StatusCode)
		}
		lines := make(chan string, 16)
		readErr := make(chan error, 1)
		go func() {
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
			readErr <- scanner.Err()
		}()
		return lines, readErr, func() { _ = resp.Body.Close() }
	}

	firstLines, firstErr, closeFirst := openStream("first")
	defer closeFirst()
	secondLines, secondErr, closeSecond := openStream("second")
	defer closeSecond()

	waitConnected := func(name string, lines <-chan string, readErr <-chan error) {
		t.Helper()
		select {
		case line := <-lines:
			if !strings.HasPrefix(line, ": connected") {
				t.Fatalf("%s first stream line = %q, want connected comment", name, line)
			}
		case err := <-readErr:
			t.Fatalf("%s stream closed before connected comment: %v", name, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s timed out waiting for stream connection", name)
		}
	}
	waitConnected("first", firstLines, firstErr)
	waitConnected("second", secondLines, secondErr)

	createResp, err := http.Post(server.URL+"/api/v1/workorch/workspaces", "application/json", strings.NewReader(`{"title":"streamed multi-client workspace","deliverableDirectory":"workspaces/streamed-multi"}`))
	if err != nil {
		t.Fatalf("post workspace: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createResp.StatusCode)
	}

	waitWorkspaceCreated := func(name string, lines <-chan string, readErr <-chan error) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case line := <-lines:
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				var envelope events.Envelope
				if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &envelope); err != nil {
					t.Fatalf("%s decode stream data %q: %v", name, line, err)
				}
				if envelope.Topic == workspaceevents.TopicWorkspaceCreated && envelope.Payload[events.PayloadKeyTitle] == "streamed multi-client workspace" {
					return
				}
			case err := <-readErr:
				t.Fatalf("%s stream closed before created event: %v", name, err)
			case <-deadline:
				t.Fatalf("%s timed out waiting for streamed workspace-created event", name)
			}
		}
	}
	waitWorkspaceCreated("first", firstLines, firstErr)
	waitWorkspaceCreated("second", secondLines, secondErr)
}

func TestEnqueueEventStreamEnvelopeKeepsWorkspaceCreatedWhenChannelFull(t *testing.T) {
	ch := make(chan events.Envelope, 1)
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: runevents.TopicStepStateChanged, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: workspaceevents.TopicWorkspaceCreated, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != workspaceevents.TopicWorkspaceCreated || got.WorkspaceID != "ws-b" {
		t.Fatalf("envelope = %+v, want workspace-created to replace older queued event", got)
	}
}

func TestEnqueueEventStreamEnvelopeDropsNonCriticalWhenChannelFull(t *testing.T) {
	ch := make(chan events.Envelope, 1)
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: workspaceevents.TopicWorkspaceCreated, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: "/custom/non-critical", WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != workspaceevents.TopicWorkspaceCreated || got.WorkspaceID != "ws-a" {
		t.Fatalf("envelope = %+v, want existing critical event preserved", got)
	}
}

func TestEnqueueEventStreamEnvelopeKeepsStepStateWhenChannelFull(t *testing.T) {
	ch := make(chan events.Envelope, 1)
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: runevents.TopicStepDispatched, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, events.Envelope{Topic: runevents.TopicStepStateChanged, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != runevents.TopicStepStateChanged || got.WorkspaceID != "ws-b" {
		t.Fatalf("envelope = %+v, want latest critical step state event", got)
	}
}

func TestConsoleRoutesServeEmbeddedAssets(t *testing.T) {
	mux := newTestMux(t, Config{Backend: "memory"})

	faviconReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	faviconRec := httptest.NewRecorder()
	mux.ServeHTTP(faviconRec, faviconReq)
	if faviconRec.Code != http.StatusOK {
		t.Fatalf("favicon status = %d, want 200", faviconRec.Code)
	}
	if got := faviconRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("favicon content type = %q, want image/png", got)
	}
	if !strings.HasPrefix(faviconRec.Body.String(), "\x89PNG") {
		t.Fatalf("favicon body does not contain PNG data")
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
		if !strings.Contains(rec.Body.String(), "/console/workorch.png") {
			t.Fatalf("%s body does not contain console icon reference", uri)
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

	req = httptest.NewRequest(http.MethodGet, "/console/workorch.png", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("console icon status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("console icon content type = %q, want image/png", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/console/assets/workorch-console.js", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("console js status = %d, want 200", rec.Code)
	}
	body := rec.Body.String() + "\n" + readEmbeddedConsoleJSAssets(t, mux)
	for _, want := range []string{"业务总览", "治理队列", "工作空间", "服务状态"} {
		if !strings.Contains(body, want) {
			t.Fatalf("console js body does not contain %q", want)
		}
	}
	for _, want := range []string{"创建工作空间", "定义目标", "定义验收标准", "提交执行", "运行摘要", "当前运行"} {
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

func readEmbeddedConsoleJSAssets(t *testing.T, mux http.Handler) string {
	t.Helper()

	var body strings.Builder
	err := fs.WalkDir(consoleassets.Assets, "dist/assets", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".js") {
			return nil
		}
		uri := "/console/" + strings.TrimPrefix(name, "dist/")
		req := httptest.NewRequest(http.MethodGet, uri, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return fmt.Errorf("%s status = %d, want 200", uri, rec.Code)
		}
		body.WriteString(rec.Body.String())
		body.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read embedded console js assets: %v", err)
	}
	return body.String()
}

func TestNativeBusinessRoutesInjectEventHub(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	hub := events.NewMemoryHub()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), hub)
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
	state.readWorkspaceDetailSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
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
		return appservice.TaskProgress{WorkspaceID: "ws-native-read", RunID: runID, State: "running", Recovery: appservice.RecoveryPlan{Mode: "resume"}}, nil
	}
	state.readRunContextBundle = func(ctx context.Context, _, _, runID string) (appservice.ContextBundle, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("run context bundle event hub = %#v, want injected hub", got)
		}
		if runID != "run-native-read" {
			t.Fatalf("runID = %q, want run-native-read", runID)
		}
		return appservice.ContextBundle{WorkspaceID: "ws-native-read", RunID: runID, Mode: "run_recovery", Recovery: appservice.RecoveryPlan{Mode: "resume"}}, nil
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
	state.refreshWorkspaceTemplate = func(ctx context.Context, _, _, workspaceID string) (appservice.Workspace, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("workspace template refresh event hub = %#v, want injected hub", got)
		}
		if workspaceID != "ws-native-read" {
			t.Fatalf("workspace template refresh id = %q, want ws-native-read", workspaceID)
		}
		return appservice.Workspace{ID: workspaceID, Title: "native refreshed", TemplateID: "template-native", State: "active"}, nil
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
	state.resumeWorkspaceExecution = func(ctx context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
	state.approveApproval = func(ctx context.Context, _, _, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval approve event hub = %#v, want injected hub", got)
		}
		if approvalID != "approval-native-read" {
			t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
		}
		if cmd.Action != "approve" {
			t.Fatalf("approval approve action = %q, want approve", cmd.Action)
		}
		return appservice.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "approved"}, nil
	}
	state.rejectApproval = func(ctx context.Context, _, _, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
		if got := appservice.EventHubFromContext(ctx); got != hub {
			t.Fatalf("approval reject event hub = %#v, want injected hub", got)
		}
		if approvalID != "approval-native-read" {
			t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
		}
		if cmd.Action != "reject" {
			t.Fatalf("approval reject action = %q, want reject", cmd.Action)
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
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/soul-bindings", status: http.StatusOK},
		{method: http.MethodPut, path: "/api/v1/workorch/workspaces/ws-native-read/soul-bindings", body: `{"defaultSoulId":"soul-native","bindings":{"execution":"soul-native-execution"}}`, status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/goal-dod-quality-check", body: `{"goalId":"goal-native","doDId":"dod-native"}`, status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/runs", body: `{"runId":"run-native-write","goalId":"goal-native","doDId":"dod-native"}`, status: http.StatusCreated},
		{method: http.MethodGet, path: "/api/v1/workorch/runs?workspaceId=ws-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/snapshot", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/steps?cursorAfterId=step-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/tool-calls?cursorAfterId=tool-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/runtime-checkpoints?cursorAfterId=checkpoint-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/results?cursorAfterId=result-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/patches?cursorAfterId=patch-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/validation-results?cursorAfterId=validation-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/artifacts?cursorAfterId=artifact-native-1&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/status", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/observability", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/task-progress", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/context-bundle", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/task-decompositions", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/start", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/runs/run-native-read/resume", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/execution-activations", body: `{"goalId":"goal-native","doDId":"dod-native","llmProviderId":"deepseek","soulId":"soul-native","soulBindings":{"validation":"soul-native-validation"},"skillIds":["skill-native"],"mcpServerIds":["mcp-native"],"memoryIds":["memory-native"],"dialogLanguage":"zh-CN","priority":9,"executionContextPatch":{"latestOperatorInput":{"source":"tui","content":"restart from tui"}}}`, status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/template-binding-refresh", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions?workspaceId=ws-native-read&runId=run-native-read&cursorAfterId=ticket-native-cursor&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/decisions/ticket-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/decisions/ticket-native-read/resolve", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/decisions/resolve-batch", body: `{"ticketIds":["ticket-native-read"],"resolutionType":"replan"}`, status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals?workspaceId=ws-native-read&runId=run-native-read&cursorAfterId=approval-native-cursor&limit=2", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/workorch/approvals/approval-native-read", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/approve", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/approval-native-read/reject", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/workorch/approvals/resolve-batch", body: `{"approvalIds":["approval-native-read"],"action":"approve"}`, status: http.StatusOK},
		{method: http.MethodDelete, path: "/api/v1/workorch/workspaces/ws-native-read?purge=true", status: http.StatusNoContent},
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
	handler := newTestMux(t, Config{Backend: "memory"})

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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})

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
	workspaceID := common.WorkspaceID("ws-http-artifact")
	runID := common.RunID("run-http-artifact")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	dataRoot := t.TempDir()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "http artifact", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("Create run: %v", err)
	}
	artifact := &resultmodel.Artifact{
		ID:           common.ArtifactID("run-http-artifact/artifact/result-summary"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
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

	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
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

	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

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

func TestWorkspaceGetEndpointOverlaysProjectedViewModel(t *testing.T) {
	server := newTestHTTPServer(t, Config{Backend: "file", DataRoot: t.TempDir()})
	handler := server.http.Handler

	workspace := createWorkspaceForHTTP(t, handler, "http projected workspace", "workspaces/ws-http-projected")
	if err := server.state.workspaceProjection.RebuildWorkspace(context.Background(), workspace.ID); err != nil {
		t.Fatalf("RebuildWorkspace: %v", err)
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
	if fetched.Workspace.ViewModel.WorkspaceID != workspace.ID ||
		fetched.Workspace.ViewModel.WorkspaceRevision == 0 ||
		fetched.Workspace.ViewModel.PromptMode == "" {
		t.Fatalf("workspace viewModel = %+v, want projected ViewModel overlay", fetched.Workspace.ViewModel)
	}
}

func TestWorkspaceGetEndpointDerivesPromptFromOwnerStateWhenProjectionIsStale(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-get-prompt-owner-state")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	state.readWorkspace = func(context.Context, string, string, string) (appservice.Workspace, error) {
		return appservice.Workspace{
			ID:               string(workspaceID),
			State:            string(common.WorkspaceStateWaitingDecision),
			ActiveRunID:      "run-get-prompt-owner-state",
			BlockingTicketID: "ticket-get-prompt-owner-state",
		}, nil
	}
	state.workspaceProjection.UpsertSnapshot(appservice.WorkspaceSnapshot{
		Workspace: appservice.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-get-prompt-owner-state",
		},
		Decisions: []appservice.Decision{{
			ID:          "ticket-get-prompt-owner-state",
			WorkspaceID: string(workspaceID),
			RunID:       "run-get-prompt-owner-state",
			State:       string(common.DecisionTicketStateOpen),
		}},
		ViewModel: readmodelcore.WorkspaceViewModel{
			WorkspaceID: string(workspaceID),
			PromptMode:  "workspace",
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID), nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace get status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body workspaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace get: %v", err)
	}
	if body.Workspace.State != string(common.WorkspaceStateWaitingDecision) ||
		body.Workspace.BlockingTicketID != "ticket-get-prompt-owner-state" ||
		body.Workspace.ViewModel.PromptMode != "decision" {
		t.Fatalf("workspace = %+v, want decision prompt derived from owner state", body.Workspace)
	}
}

func TestWorkspaceDeleteEndpointDeletesWorkspaceAndSupportsPurge(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})

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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

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

	newTestMux(t, Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkspaceGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-missing", nil)
	rec := httptest.NewRecorder()

	newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoalAndDoDUpdateEndpointsReturnUpdatedDefinitions(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})
	workspace := createWorkspaceForHTTP(t, handler, "definition update workspace", "workspaces/definition-update")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	goalReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/"+workspace.ID+"/goals/"+goalID, strings.NewReader(`{"text":"updated goal text"}`))
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
	if goalBody.Goal.ID != goalID || goalBody.Goal.Text != "- updated goal text" {
		t.Fatalf("goal response = %+v, want normalized updated text", goalBody.Goal)
	}

	dodReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/"+workspace.ID+"/dods/"+dodID, strings.NewReader(`{"items":["updated dod","second dod"]}`))
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

func TestGoalAndDoDPathScopedEndpointsHandleDuplicateDefinitionIDs(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	workspaceA := createWorkspaceForHTTP(t, handler, "duplicate definition workspace a", "workspaces/dup-definition-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "duplicate definition workspace b", "workspaces/dup-definition-b")
	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	goalID := common.GoalID("goal-shared-http")
	dodID := common.DoDID("dod-shared-http")
	for _, workspaceID := range []string{workspaceA.ID, workspaceB.ID} {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
		if err := exports.Repositories.Goals.Create(scopedCtx, &workspacemodel.Goal{
			ID:          goalID,
			WorkspaceID: common.WorkspaceID(workspaceID),
			Text:        "- goal for " + workspaceID,
		}); err != nil {
			t.Fatalf("create duplicate goal for %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.DoDs.Create(scopedCtx, &workspacemodel.DoD{
			ID:          dodID,
			WorkspaceID: common.WorkspaceID(workspaceID),
			Items:       []string{"dod for " + workspaceID},
			State:       common.DoDStateReady,
		}); err != nil {
			t.Fatalf("create duplicate dod for %s: %v", workspaceID, err)
		}
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	goalGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/goals/"+string(goalID), nil)
	goalGetRec := httptest.NewRecorder()
	handler.ServeHTTP(goalGetRec, goalGetReq)
	if goalGetRec.Code != http.StatusOK {
		t.Fatalf("goal get status = %d, want 200, body=%s", goalGetRec.Code, goalGetRec.Body.String())
	}
	var goalGetBody goalResponse
	if err := json.Unmarshal(goalGetRec.Body.Bytes(), &goalGetBody); err != nil {
		t.Fatalf("unmarshal goal get: %v", err)
	}
	if goalGetBody.Goal.WorkspaceID != workspaceB.ID || !strings.Contains(goalGetBody.Goal.Text, workspaceB.ID) {
		t.Fatalf("goal get body = %+v, want workspace B definition", goalGetBody.Goal)
	}

	goalPutReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/goals/"+string(goalID), strings.NewReader(`{"text":"updated duplicate goal b"}`))
	goalPutReq.Header.Set("Content-Type", "application/json")
	goalPutRec := httptest.NewRecorder()
	handler.ServeHTTP(goalPutRec, goalPutReq)
	if goalPutRec.Code != http.StatusOK {
		t.Fatalf("goal update status = %d, want 200, body=%s", goalPutRec.Code, goalPutRec.Body.String())
	}
	var goalPutBody goalResponse
	if err := json.Unmarshal(goalPutRec.Body.Bytes(), &goalPutBody); err != nil {
		t.Fatalf("unmarshal goal update: %v", err)
	}
	if goalPutBody.Goal.WorkspaceID != workspaceB.ID || goalPutBody.Goal.Text != "- updated duplicate goal b" {
		t.Fatalf("goal update body = %+v, want workspace B update", goalPutBody.Goal)
	}

	dodPutReq := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/dods/"+string(dodID), strings.NewReader(`{"items":["updated duplicate dod b"]}`))
	dodPutReq.Header.Set("Content-Type", "application/json")
	dodPutRec := httptest.NewRecorder()
	handler.ServeHTTP(dodPutRec, dodPutReq)
	if dodPutRec.Code != http.StatusOK {
		t.Fatalf("dod update status = %d, want 200, body=%s", dodPutRec.Code, dodPutRec.Body.String())
	}
	var dodPutBody dodResponse
	if err := json.Unmarshal(dodPutRec.Body.Bytes(), &dodPutBody); err != nil {
		t.Fatalf("unmarshal dod update: %v", err)
	}
	if dodPutBody.DoD.WorkspaceID != workspaceB.ID || len(dodPutBody.DoD.Items) != 1 || dodPutBody.DoD.Items[0] != "updated duplicate dod b" {
		t.Fatalf("dod update body = %+v, want workspace B update", dodPutBody.DoD)
	}

	session, err = appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend #2: %v", err)
	}
	exports, err = session.Exports()
	if err != nil {
		t.Fatalf("exports #2: %v", err)
	}
	goalA, err := exports.Repositories.Goals.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceA.ID)), goalID)
	if err != nil {
		t.Fatalf("get workspace A goal: %v", err)
	}
	if goalA.Text != "- goal for "+workspaceA.ID {
		t.Fatalf("workspace A goal = %+v, want unchanged", goalA)
	}
	dodA, err := exports.Repositories.DoDs.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceA.ID)), dodID)
	if err != nil {
		t.Fatalf("get workspace A dod: %v", err)
	}
	if len(dodA.Items) != 1 || dodA.Items[0] != "dod for "+workspaceA.ID {
		t.Fatalf("workspace A dod = %+v, want unchanged", dodA)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session #2: %v", err)
	}
}

func TestRunCreateEndpointReturnsCreatedRun(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

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
	for _, item := range listed.Runs {
		if len(item.ExecutionContext) > 0 {
			t.Fatalf("listed run %s executionContext = %+v, want metadata-only list response", item.ID, item.ExecutionContext)
		}
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
	if len(filtered.Runs[0].ExecutionContext) > 0 {
		t.Fatalf("filtered run executionContext = %+v, want metadata-only list response", filtered.Runs[0].ExecutionContext)
	}

	workspaceScopedReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/runs", nil)
	workspaceScopedRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceScopedRec, workspaceScopedReq)
	if workspaceScopedRec.Code != http.StatusOK {
		t.Fatalf("workspace scoped run list status = %d, want 200, body=%s", workspaceScopedRec.Code, workspaceScopedRec.Body.String())
	}
	var workspaceScoped runsResponse
	if err := json.Unmarshal(workspaceScopedRec.Body.Bytes(), &workspaceScoped); err != nil {
		t.Fatalf("unmarshal workspace scoped runs: %v", err)
	}
	if len(workspaceScoped.Runs) != 1 || workspaceScoped.Runs[0].ID != "run-a" || workspaceScoped.Runs[0].WorkspaceID != workspaceA.ID {
		t.Fatalf("workspace scoped runs = %+v, want run-a for %s", workspaceScoped.Runs, workspaceA.ID)
	}
	if len(workspaceScoped.Runs[0].ExecutionContext) > 0 {
		t.Fatalf("workspace scoped run executionContext = %+v, want metadata-only list response", workspaceScoped.Runs[0].ExecutionContext)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/runs/run-a", nil)
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

func TestUnscopedRunGetReturnsWorkspaceRequiredForDuplicateRunIDs(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceA := createWorkspaceForHTTP(t, handler, "duplicate run workspace a", "workspaces/dup-run-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "duplicate run workspace b", "workspaces/dup-run-b")
	goalA, dodA := defineGoalAndDoDForHTTP(t, handler, workspaceA.ID)
	goalB, dodB := defineGoalAndDoDForHTTP(t, handler, workspaceB.ID)

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/workorch/workspaces/" + workspaceA.ID + "/runs", body: `{"runId":"run-001","goalId":"` + goalA + `","doDId":"` + dodA + `"}`},
		{path: "/api/v1/workorch/workspaces/" + workspaceB.ID + "/runs", body: `{"runId":"run-001","goalId":"` + goalB + `","doDId":"` + dodB + `"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-001", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("unscoped duplicate run get status = %d, want 404, body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "404 page not found") {
		t.Fatalf("unscoped duplicate run get body = %s, want not found", getRec.Body.String())
	}

	scopedReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/runs/run-001", nil)
	scopedRec := httptest.NewRecorder()
	handler.ServeHTTP(scopedRec, scopedReq)
	if scopedRec.Code != http.StatusOK {
		t.Fatalf("workspace scoped duplicate run get status = %d, want 200, body=%s", scopedRec.Code, scopedRec.Body.String())
	}
}

func TestUnscopedRunEndpointsReturnWorkspaceRequiredForDuplicateRunIDs(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceA := createWorkspaceForHTTP(t, handler, "duplicate scoped run workspace a", "workspaces/dup-scoped-run-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "duplicate scoped run workspace b", "workspaces/dup-scoped-run-b")
	goalA, dodA := defineGoalAndDoDForHTTP(t, handler, workspaceA.ID)
	goalB, dodB := defineGoalAndDoDForHTTP(t, handler, workspaceB.ID)

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/workorch/workspaces/" + workspaceA.ID + "/runs", body: `{"runId":"run-001","goalId":"` + goalA + `","doDId":"` + dodA + `"}`},
		{path: "/api/v1/workorch/workspaces/" + workspaceB.ID + "/runs", body: `{"runId":"run-001","goalId":"` + goalB + `","doDId":"` + dodB + `"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "snapshot", method: http.MethodGet, path: "/api/v1/workorch/runs/run-001/snapshot"},
		{name: "status", method: http.MethodGet, path: "/api/v1/workorch/runs/run-001/status"},
		{name: "start", method: http.MethodPost, path: "/api/v1/workorch/runs/run-001/start"},
		{name: "resume", method: http.MethodPost, path: "/api/v1/workorch/runs/run-001/resume"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s duplicate run endpoint status = %d, want 404, body=%s", tc.name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "404 page not found") {
			t.Fatalf("%s duplicate run endpoint body = %s, want not found", tc.name, rec.Body.String())
		}
	}
}

func TestEventStreamEndpointReplaysFileBackedRecordsAfterRestart(t *testing.T) {
	dataRoot := t.TempDir()
	ctx := context.Background()
	first := newTestHTTPServer(t, Config{Backend: "file", DataRoot: dataRoot})
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

	second := newTestHTTPServer(t, Config{Backend: "file", DataRoot: dataRoot})
	t.Cleanup(func() {
		if err := second.Shutdown(ctx); err != nil {
			t.Fatalf("shutdown second server: %v", err)
		}
	})
	secondHandler := second.http.Handler

	workspaceEvents := readEventRecordsForHTTP(t, secondHandler, "/api/v1/workorch/events?workspaceId="+workspace.ID)
	if !eventRecordsContainTopic(workspaceEvents, workspaceevents.TopicWorkspaceCreated) ||
		!eventRecordsContainTopic(workspaceEvents, workspaceevents.TopicGoalDefined) ||
		!eventRecordsContainTopic(workspaceEvents, workspaceevents.TopicDoDDefined) ||
		!eventRecordsContainTopic(workspaceEvents, runevents.TopicRunCreated) {
		t.Fatalf("workspace events = %+v, want workspace/goal/dod/run persisted events after restart", workspaceEvents)
	}

	runEvents := readEventRecordsForHTTP(t, secondHandler, "/api/v1/workorch/events?runId=run-event-replay")
	if len(runEvents) != 1 || runEvents[0].Topic != runevents.TopicRunCreated || runEvents[0].WorkspaceID != workspace.ID {
		t.Fatalf("run events = %+v, want replayed run-created event", runEvents)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-event-replay/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	secondHandler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var snapshotBody runSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &snapshotBody); err != nil {
		t.Fatalf("unmarshal run snapshot: %v", err)
	}
	if len(snapshotBody.Snapshot.EventRecords) != 1 || snapshotBody.Snapshot.EventRecords[0].Topic != runevents.TopicRunCreated {
		t.Fatalf("snapshot eventRecords = %+v, want replayed run-created event", snapshotBody.Snapshot.EventRecords)
	}

	observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-event-replay/observability", nil)
	observabilityRec := httptest.NewRecorder()
	secondHandler.ServeHTTP(observabilityRec, observabilityReq)
	if observabilityRec.Code != http.StatusOK {
		t.Fatalf("observability status = %d, want 200, body=%s", observabilityRec.Code, observabilityRec.Body.String())
	}
	var observabilityBody runObservabilityResponse
	if err := json.Unmarshal(observabilityRec.Body.Bytes(), &observabilityBody); err != nil {
		t.Fatalf("unmarshal run observability: %v", err)
	}
	if !timelineContainsPersistedEvent(observabilityBody.Observability.Timeline, runevents.TopicRunCreated) {
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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-snapshot-http"

	workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "run snapshot http workspace", runID)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/snapshot", nil)
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

	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
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

	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/runs/"+string(runID)+"/snapshot", nil)
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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-status-http"

	workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run status http workspace", runID)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/status", nil)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-wait-status/runs/run-wait-status/status?waitState=completed&waitTimeoutMillis=200&waitIntervalMillis=10", nil)
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

func TestRunStatusEndpointInvalidatesCachedTerminalSnapshotOnOwnerRunStateEvent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	var statusReads atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (appservice.RunStatus, error) {
		statusReads.Add(1)
		return appservice.RunStatus{
			ID:          runID,
			WorkspaceID: "ws-status-owner-event",
			State:       string(common.RunStateRunning),
		}, nil
	}
	state.cacheRunSnapshot(appservice.RunSnapshot{
		Workspace: appservice.Workspace{ID: "ws-status-owner-event", State: string(common.WorkspaceStateCompleted)},
		Run: appservice.Run{
			ID:          "run-status-owner-event",
			WorkspaceID: "ws-status-owner-event",
			State:       string(common.RunStateCompleted),
		},
	})
	handler := newMuxWithState(state)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-owner-event/runs/run-status-owner-event/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cached status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var cachedBody runStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &cachedBody); err != nil {
		t.Fatalf("unmarshal cached run status: %v", err)
	}
	if cachedBody.Status.State != string(common.RunStateCompleted) || statusReads.Load() != 0 {
		t.Fatalf("cached status = %+v reads=%d, want cached completed state before owner event", cachedBody.Status, statusReads.Load())
	}

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-status-owner-event",
		RunID:       "run-status-owner-event",
		SubjectID:   "run-status-owner-event",
		Payload:     events.RunPayload("", "", common.RunStateRunning, "", "", ""),
	}); err != nil {
		t.Fatalf("publish run state changed: %v", err)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status after owner event = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.State != string(common.RunStateRunning) {
		t.Fatalf("status after owner event = %+v, want fresh running status", body.Status)
	}
	if statusReads.Load() == 0 {
		t.Fatal("status endpoint reused stale cached snapshot after owner run state event")
	}
}

func TestRunStatusWaitUsesStatusChangeNotification(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

func TestTerminalRunSnapshotCacheReusesSnapshotForSnapshotAndStatusOnly(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
		"/api/v1/workorch/workspaces/ws-cache/runs/run-cache/snapshot",
		"/api/v1/workorch/workspaces/ws-cache/runs/run-cache/status",
		"/api/v1/workorch/workspaces/ws-cache/runs/run-cache/observability",
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
	if observabilityReads.Load() != 1 {
		t.Fatalf("observability reads = %d, want independent full observability read", observabilityReads.Load())
	}

	state.notifyRunStatusChanged()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-cache/runs/run-cache/snapshot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot after invalidation status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if snapshotReads.Load() != 2 {
		t.Fatalf("snapshot reads after invalidation = %d, want 2", snapshotReads.Load())
	}
}

func TestRunSnapshotCacheByWorkspaceRequiresUniqueCachedRun(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	state.cacheRunSnapshot(appservice.RunSnapshot{
		Workspace: appservice.Workspace{ID: "ws-cache-unique", State: string(common.WorkspaceStateCompleted)},
		Run:       appservice.Run{ID: "run-cache-first", WorkspaceID: "ws-cache-unique", State: string(common.RunStateCompleted)},
	})
	if cached, ok := state.cachedRunSnapshotByWorkspace("ws-cache-unique"); !ok || cached.Run.ID != "run-cache-first" {
		t.Fatalf("cached snapshot = %+v ok=%v, want first run cache hit", cached, ok)
	}

	state.cacheRunSnapshot(appservice.RunSnapshot{
		Workspace: appservice.Workspace{ID: "ws-cache-unique", State: string(common.WorkspaceStateCompleted)},
		Run:       appservice.Run{ID: "run-cache-second", WorkspaceID: "ws-cache-unique", State: string(common.RunStateCompleted)},
	})
	if cached, ok := state.cachedRunSnapshotByWorkspace("ws-cache-unique"); ok {
		t.Fatalf("cached snapshot = %+v, want miss when one workspace has multiple cached terminal runs", cached)
	}
}

func TestRunStatusWaitWarmsTerminalRunSnapshotCache(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-warm-cache/runs/run-status-warm-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	waitForCondition(t, time.Second, func() bool {
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-status-warm-cache")
		_, ok := state.cachedRunSnapshot(ctx, "run-status-warm-cache")
		return ok
	})

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-warm-cache/runs/run-status-warm-cache/snapshot", nil)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-notify-warm-cache")
		_, ok := state.cachedRunSnapshot(ctx, "run-notify-warm-cache")
		return ok
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-notify-warm-cache/runs/run-notify-warm-cache/snapshot", nil)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-inflight-cache/runs/run-status-inflight-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
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
		snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-inflight-cache/runs/run-status-inflight-cache/snapshot", nil)
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

func TestRunSnapshotCacheInvalidationPreservesInflightWarmup(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-inflight-clear")
	runID := "run-inflight-clear"

	if _, duplicate := state.ensureRunSnapshotInflight(ctx, runID); duplicate {
		t.Fatal("first inflight registration reported duplicate")
	}
	state.cacheRunSnapshot(appservice.RunSnapshot{
		Workspace: appservice.Workspace{ID: "ws-inflight-clear", State: string(common.WorkspaceStateCompleted)},
		Run:       appservice.Run{ID: runID, WorkspaceID: "ws-inflight-clear", State: string(common.RunStateCompleted)},
	})
	state.clearRunSnapshotCache()
	if _, ok := state.cachedRunSnapshot(ctx, runID); ok {
		t.Fatal("cached snapshot survived invalidation")
	}

	waitDone := make(chan struct{})
	var waited appservice.RunSnapshot
	var waitErr error
	var waitedInflight bool
	go func() {
		defer close(waitDone)
		waited, waitErr, waitedInflight = state.waitRunSnapshotInflight(ctx, runID)
	}()
	time.Sleep(20 * time.Millisecond)
	state.completeRunSnapshotInflight(ctx, runID, appservice.RunSnapshot{
		Workspace: appservice.Workspace{ID: "ws-inflight-clear", State: string(common.WorkspaceStateCompleted)},
		Run:       appservice.Run{ID: runID, WorkspaceID: "ws-inflight-clear", State: string(common.RunStateCompleted)},
	}, nil)
	state.finishRunSnapshotInflight(ctx, runID)

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("inflight waiter did not complete after cache invalidation")
	}
	if waitErr != nil || !waitedInflight || waited.Run.ID != runID {
		t.Fatalf("waited snapshot/err/inflight = %+v/%v/%v, want completed inflight snapshot", waited, waitErr, waitedInflight)
	}
}

func TestRunObservabilityReadWaitsForTerminalStatusWarmupInflight(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-observability-inflight-cache/runs/run-observability-inflight-cache/status?waitState=completed&waitTimeoutMillis=1000", nil)
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
		observabilityReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-observability-inflight-cache/runs/run-observability-inflight-cache/observability", nil)
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
	select {
	case <-observabilityDone:
	case <-time.After(time.Second):
		t.Fatal("observability endpoint did not complete from independent observability read")
	}
	if observabilityReads.Load() != 1 {
		t.Fatalf("observability reads = %d, want independent full observability read", observabilityReads.Load())
	}
	close(releaseRead)
	if snapshotReads.Load() != 1 {
		t.Fatalf("snapshot reads = %d, want warmup read not reused by observability endpoint", snapshotReads.Load())
	}
}

func TestWorkspaceSnapshotEndpointReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	dataRoot := t.TempDir()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
	handler := newMuxWithState(state)

	workspace := createWorkspaceForHTTP(t, handler, "workspace snapshot http", "workspaces/workspace-snapshot-http")
	ctx := context.Background()
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
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
	state.rebuildProjectedWorkspace(ctx, workspace.ID)

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

func TestWorkspaceSnapshotEndpointReturnsCompactProjection(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-compact-http")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}), nil)
	ctx := context.Background()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	if err := repos.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "compact projection",
		DeliverableDirectory: "workspaces/compact-projection",
		State:                common.WorkspaceStateActive,
		ActiveRunID:          "run-00",
		ContextHints:         map[string]any{"large": strings.Repeat("x", 128)},
		EvidencePolicies:     []map[string]any{{"large": strings.Repeat("x", 128)}},
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for idx := 0; idx < 12; idx++ {
		runID := common.RunID(fmt.Sprintf("run-%02d", idx))
		if err := repos.Runs.Create(ctx, &runmodel.Run{
			ID:               runID,
			WorkspaceID:      workspaceID,
			State:            common.RunStateRunning,
			ExecutionContext: map[string]any{"large": strings.Repeat("x", 128)},
		}); err != nil {
			t.Fatalf("create run %s: %v", runID, err)
		}
	}
	for idx := 0; idx < 24; idx++ {
		runID := common.RunID("run-11")
		if idx == 0 {
			runID = "run-00"
		}
		if err := repos.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
			ID:          common.EventRecordID(fmt.Sprintf("usage-%02d", idx)),
			Version:     events.WorkorchEventVersion,
			Topic:       runevents.TopicRunLLMUsageRecorded,
			OccurredAt:  fmt.Sprintf("2026-06-12T00:%02d:00Z", idx),
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      common.StepID(fmt.Sprintf("%s/step-%02d", runID, idx)),
			Payload: map[string]any{
				events.PayloadKeyWorkspaceID:  string(workspaceID),
				events.PayloadKeyRunID:        string(runID),
				events.PayloadKeyStepID:       fmt.Sprintf("%s/step-%02d", runID, idx),
				events.PayloadKeyProviderID:   "provider-compact",
				events.PayloadKeyProvider:     "compatible",
				events.PayloadKeyModel:        "model-compact",
				events.PayloadKeyOperation:    "chat_completions",
				events.PayloadKeyInputTokens:  10 + idx,
				events.PayloadKeyOutputTokens: 1,
				events.PayloadKeyTotalTokens:  11 + idx,
				events.PayloadKeyStatus:       "completed",
			},
		}); err != nil {
			t.Fatalf("append usage event %d: %v", idx, err)
		}
	}
	state.rebuildProjectedWorkspace(ctx, string(workspaceID))
	handler := newMuxWithState(state)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Projection.WorkspaceRevision <= 0 {
		t.Fatalf("projection workspace revision = %d, want positive workspace-scoped revision", body.Projection.WorkspaceRevision)
	}
	if body.Snapshot.Workspace.ContextHints != nil || body.Snapshot.Workspace.EvidencePolicies != nil {
		t.Fatalf("workspace heavy fields = %+v, want compact projection", body.Snapshot.Workspace)
	}
	if body.Snapshot.Run == nil || body.Snapshot.Run.ExecutionContext != nil {
		t.Fatalf("run = %+v, want compact active run", body.Snapshot.Run)
	}
	if len(body.Snapshot.Runs) != 11 {
		t.Fatalf("runs = %d, want active run plus latest 10 compact runs", len(body.Snapshot.Runs))
	}
	for _, item := range body.Snapshot.Runs {
		if item.ExecutionContext != nil {
			t.Fatalf("run %s execution context = %+v, want compact run", item.ID, item.ExecutionContext)
		}
	}
	if len(body.Snapshot.LLMUsageTraces) >= 24 {
		t.Fatalf("llm usage traces = %d, want bounded detail rather than full workspace usage", len(body.Snapshot.LLMUsageTraces))
	}
	if body.Snapshot.LLMUsageSummary.EntryTotal >= 24 {
		t.Fatalf("summary entry total = %d, want default detail to avoid full workspace usage", body.Snapshot.LLMUsageSummary.EntryTotal)
	}

	fullRec := httptest.NewRecorder()
	fullReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot?view=observability", nil)
	handler.ServeHTTP(fullRec, fullReq)
	if fullRec.Code != http.StatusOK {
		t.Fatalf("observability workspace snapshot status = %d, want 200, body=%s", fullRec.Code, fullRec.Body.String())
	}
	var fullBody workspaceSnapshotResponse
	if err := json.Unmarshal(fullRec.Body.Bytes(), &fullBody); err != nil {
		t.Fatalf("unmarshal observability workspace snapshot: %v", err)
	}
	if len(fullBody.Snapshot.LLMUsageTraces) != 24 {
		t.Fatalf("observability llm usage traces = %d, want full workspace usage", len(fullBody.Snapshot.LLMUsageTraces))
	}
	if fullBody.Snapshot.LLMUsageTraces[0].ID != "usage-00" {
		t.Fatalf("observability first usage trace = %q, want full trace history retained", fullBody.Snapshot.LLMUsageTraces[0].ID)
	}
	if fullBody.Snapshot.LLMUsageSummary.EntryTotal != 24 {
		t.Fatalf("observability summary entry total = %d, want full total", fullBody.Snapshot.LLMUsageSummary.EntryTotal)
	}
}

func TestWorkspaceSnapshotEndpointDefaultUsesExplicitCompactDetail(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-explicit-compact-detail")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	state.workspaceProjection.UpsertWorkspace(appservice.Workspace{
		ID:          string(workspaceID),
		State:       string(common.WorkspaceStateRunning),
		ActiveRunID: "run-explicit-compact-detail",
	})
	state.workspaceProjection.UpsertSnapshot(appservice.WorkspaceSnapshot{
		Workspace: appservice.Workspace{
			ID:                   string(workspaceID),
			State:                string(common.WorkspaceStateRunning),
			ActiveRunID:          "run-explicit-compact-detail",
			DeliverableDirectory: "workspaces/explicit-compact-detail",
		},
	})
	var detailReads atomic.Int32
	var fullReads atomic.Int32
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		detailReads.Add(1)
		return appservice.WorkspaceSnapshot{
			Workspace: appservice.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateRunning),
				ActiveRunID:          "run-explicit-compact-detail",
				DeliverableDirectory: "workspaces/explicit-compact-detail",
			},
			Run: &appservice.Run{
				ID:          "run-explicit-compact-detail",
				WorkspaceID: string(workspaceID),
				State:       string(common.RunStateRunning),
			},
		}, nil
	}
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		fullReads.Add(1)
		return appservice.WorkspaceSnapshot{
			Workspace: appservice.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateRunning),
				ActiveRunID:          "run-explicit-compact-detail",
				ContextHints:         map[string]any{"large": strings.Repeat("x", 128)},
				EvidencePolicies:     []map[string]any{{"large": strings.Repeat("x", 128)}},
				DeliverableDirectory: "workspaces/explicit-compact-detail",
			},
			Run: &appservice.Run{
				ID:               "run-explicit-compact-detail",
				WorkspaceID:      string(workspaceID),
				State:            string(common.RunStateRunning),
				ExecutionContext: map[string]any{"large": strings.Repeat("x", 128)},
			},
		}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if detailReads.Load() != 1 {
		t.Fatalf("detail snapshot reads = %d, want explicit compact detail read", detailReads.Load())
	}
	if fullReads.Load() != 0 {
		t.Fatalf("full snapshot reads = %d, want default detail to avoid full read", fullReads.Load())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ContextHints != nil || body.Snapshot.Workspace.EvidencePolicies != nil {
		t.Fatalf("workspace heavy fields = %+v, want projected compact detail", body.Snapshot.Workspace)
	}
	if body.Snapshot.Run == nil || body.Snapshot.Run.ExecutionContext != nil {
		t.Fatalf("run = %+v, want projected compact run", body.Snapshot.Run)
	}
	if len(body.Snapshot.Runs) != 1 || body.Snapshot.Runs[0].ID != "run-explicit-compact-detail" {
		t.Fatalf("runs = %+v, want selected run included in projected detail relations", body.Snapshot.Runs)
	}
}

func TestWorkspaceSnapshotEndpointSummaryDoesNotHydrateDetail(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-summary-no-detail-hydrate")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	state.workspaceProjection.UpsertWorkspace(appservice.Workspace{
		ID:          string(workspaceID),
		State:       string(common.WorkspaceStateRunning),
		ActiveRunID: "run-summary-no-detail-hydrate",
	})
	var projectionReads atomic.Int32
	state.readProjectionSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		projectionReads.Add(1)
		return appservice.WorkspaceSnapshot{}, errors.New("detail projection read should not be used for summary")
	}
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		return appservice.WorkspaceSnapshot{}, errors.New("full snapshot read should not be used for summary")
	}
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		return appservice.WorkspaceSnapshot{}, errors.New("detail snapshot read should not be used for summary")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot?view=summary", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace summary status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if projectionReads.Load() != 0 {
		t.Fatalf("projection detail reads = %d, want summary from projection overview only", projectionReads.Load())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace summary: %v", err)
	}
	if body.Snapshot.Workspace.ID != string(workspaceID) || body.Snapshot.Workspace.ActiveRunID != "run-summary-no-detail-hydrate" {
		t.Fatalf("workspace summary = %+v, want projected overview", body.Snapshot.Workspace)
	}
	if body.Snapshot.Run != nil || len(body.Snapshot.Runs) != 0 || len(body.Snapshot.Results) != 0 || len(body.Snapshot.EventRecords) != 0 {
		t.Fatalf("summary snapshot includes detail fields: run=%+v runs=%d results=%d events=%d", body.Snapshot.Run, len(body.Snapshot.Runs), len(body.Snapshot.Results), len(body.Snapshot.EventRecords))
	}
	if body.Snapshot.ViewModel.WorkspaceID != string(workspaceID) || body.Snapshot.ViewModel.PromptMode == "" {
		t.Fatalf("viewModel = %+v, want summary projection view model", body.Snapshot.ViewModel)
	}
}

func TestWorkspaceSnapshotEndpointDoesNotOverlayStalePromptMode(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-detail-prompt-same-source")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	state.workspaceProjection.UpsertSnapshot(appservice.WorkspaceSnapshot{
		Workspace: appservice.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-detail-prompt-same-source",
		},
	})
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		return appservice.WorkspaceSnapshot{
			Workspace: appservice.Workspace{
				ID:               string(workspaceID),
				State:            string(common.WorkspaceStateWaitingDecision),
				ActiveRunID:      "run-detail-prompt-same-source",
				BlockingTicketID: "ticket-detail-prompt-same-source",
			},
			Run: &appservice.Run{
				ID:               "run-detail-prompt-same-source",
				WorkspaceID:      string(workspaceID),
				State:            string(common.RunStateBlockedOnDecision),
				BlockingTicketID: "ticket-detail-prompt-same-source",
			},
			Decisions: []appservice.Decision{{
				ID:          "ticket-detail-prompt-same-source",
				WorkspaceID: string(workspaceID),
				RunID:       "run-detail-prompt-same-source",
				State:       string(common.DecisionTicketStateOpen),
			}},
		}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Snapshot.Workspace.State != string(common.WorkspaceStateWaitingDecision) ||
		body.Snapshot.Workspace.BlockingTicketID != "ticket-detail-prompt-same-source" ||
		body.Snapshot.Run == nil ||
		body.Snapshot.Run.State != string(common.RunStateBlockedOnDecision) {
		t.Fatalf("snapshot workspace/run = %+v/%+v, want blocked decision owner state", body.Snapshot.Workspace, body.Snapshot.Run)
	}
	if body.Snapshot.ViewModel.PromptMode != "decision" ||
		body.Snapshot.Workspace.ViewModel.PromptMode != "decision" {
		t.Fatalf("snapshot viewModel = %+v workspace viewModel = %+v, want decision from detail owner state", body.Snapshot.ViewModel, body.Snapshot.Workspace.ViewModel)
	}
}

func TestWorkspaceSnapshotEndpointObservabilityUsesExplicitFullRead(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-observability-full-read")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	state.workspaceProjection.UpsertSnapshot(appservice.WorkspaceSnapshot{
		Workspace: appservice.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateRunning),
			ActiveRunID: "run-observability-full-read",
		},
	})
	var fullReads atomic.Int32
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error) {
		fullReads.Add(1)
		return appservice.WorkspaceSnapshot{
			Workspace: appservice.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateRunning),
				ActiveRunID:          "run-observability-full-read",
				ContextHints:         map[string]any{"large": strings.Repeat("x", 128)},
				EvidencePolicies:     []map[string]any{{"large": strings.Repeat("x", 128)}},
				DeliverableDirectory: "workspaces/observability-full-read",
			},
			Run: &appservice.Run{
				ID:               "run-observability-full-read",
				WorkspaceID:      string(workspaceID),
				State:            string(common.RunStateRunning),
				ExecutionContext: map[string]any{"large": strings.Repeat("x", 128)},
			},
		}, nil
	}
	handler := newMuxWithState(state)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+string(workspaceID)+"/snapshot?view=observability", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if fullReads.Load() != 1 {
		t.Fatalf("full snapshot reads = %d, want explicit observability read", fullReads.Load())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ContextHints == nil || body.Snapshot.Workspace.EvidencePolicies == nil {
		t.Fatalf("workspace = %+v, want observability heavy fields retained", body.Snapshot.Workspace)
	}
	if body.Snapshot.Run == nil || body.Snapshot.Run.ExecutionContext == nil {
		t.Fatalf("run = %+v, want observability full run retained", body.Snapshot.Run)
	}
}

func TestRunCreateEndpointReturnsConflictForDuplicateID(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

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

	newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-run-missing/runs/run-missing", nil)
	rec := httptest.NewRecorder()

	newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunStartEndpointReturnsBlockedProviderDecisionRun(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

	workspace := createWorkspaceForHTTP(t, handler, "run start http workspace", "workspaces/run-start-http")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-start-http","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-start-http/start", nil)
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
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-http"

	workspace, seeded := seedBlockedDecisionRunForHTTP(t, dataRoot, "run start blocked http workspace", runID)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID, nil)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-decision-resume-http"

	workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "run resume decision http workspace", runID)
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}

	resolveDecisionForHTTP(t, dataRoot, started.BlockingTicketID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/resume", nil)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-resume-http"

	workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume approval http workspace", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/resume", nil)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-rejected-http"

	workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume approval rejected http workspace", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, true)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/resume", nil)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-approval-pending-http"

	workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "run resume pending http workspace", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestRunResumeEndpointReturnsConflictForCreatedRun(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})
	workspace := createWorkspaceAndRunForHTTP(t, handler, "run resume created http workspace", "run-resume-created-http")

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-resume-created-http/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestDecisionListAndGetEndpointsReturnPersistedTickets(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
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
	if len(listed.Decisions) != 2 || listed.Decisions[0].ID != startedB.BlockingTicketID || listed.Decisions[1].ID != startedA.BlockingTicketID {
		t.Fatalf("listed decisions = %+v, want newest tickets first", listed.Decisions)
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

func TestDecisionResolveEndpointRejectsInvalidMissingInfoAndKeepsTicketOpen(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "empty explicit answer", body: `{"resolutionType":"provide_missing_info","resolutionPayload":{"answer":""}}`},
		{name: "whitespace explicit answer", body: `{"resolutionType":"provide_missing_info","resolutionPayload":{"answer":"   "}}`},
		{name: "slash only", body: `{"resolutionType":"provide_missing_info","resolutionPayload":{"answer":"/"}}`},
		{name: "command prefix only", body: `{"resolutionType":"provide_missing_info","resolutionPayload":{"answer":"/decision answer"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			handler := newFileMuxWithCleanup(t, dataRoot)
			runID := "run-decision-invalid-missing-info-http"
			_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision invalid missing info http workspace", runID)

			resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", strings.NewReader(tc.body))
			resolveReq.Header.Set("Content-Type", "application/json")
			resolveRec := httptest.NewRecorder()
			handler.ServeHTTP(resolveRec, resolveReq)
			if resolveRec.Code != http.StatusBadRequest {
				t.Fatalf("decision resolve status = %d, want 400, body=%s", resolveRec.Code, resolveRec.Body.String())
			}
			if files := resolutionFilesForHTTP(t, dataRoot); len(files) != 0 {
				t.Fatalf("resolution files = %v, want none for invalid missing-info payload", files)
			}
			ticket, err := appservice.ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
			if err != nil {
				t.Fatalf("read decision: %v", err)
			}
			if ticket.State != string(common.DecisionTicketStateOpen) {
				t.Fatalf("decision state = %q, want open after invalid missing-info payload", ticket.State)
			}
		})
	}
}

func TestDecisionResolveEndpointProjectsReplanRunCommand(t *testing.T) {
	dataRoot := t.TempDir()
	routine := &recordingBackgroundRoutine{}
	config := normalizeConfig(Config{Backend: "file", DataRoot: dataRoot})
	state := newServerStateWithBackground(config, nil, routine)
	configureIsolatedTestSession(t, state, config)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(ctx)
	})
	handler := newMuxWithState(state)
	runID := "run-decision-replan-command-http"

	workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision replan command http workspace", runID)
	state.rebuildProjectedWorkspace(context.Background(), workspace.ID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve?priority=3", strings.NewReader(`{"resolutionType":"replan","resolutionPayload":{"operator":"http-replan"}}`))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("decision replan resolve status = %d, want 200, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
	if !routine.asyncFunctionCalled {
		t.Fatal("decision replan resolve did not submit a governance continuation work item")
	}
	expectedRef := governanceRunResumeRef{
		WorkspaceID: workspace.ID,
		RunID:       runID,
		Intent:      "replan",
		Reason:      "decision_replan_resolved",
		RequestID:   governanceRunResumeRequestID("decision", started.BlockingTicketID, "replan"),
		DecisionID:  started.BlockingTicketID,
	}
	expectedKey := governanceRunResumeKey(expectedRef)
	state.runContinuations.governanceMu.Lock()
	pendingRef, governancePending := state.runContinuations.governancePending[expectedKey]
	_, governanceStarted := state.runContinuations.governanceStarted[expectedKey]
	state.runContinuations.governanceMu.Unlock()
	if !governancePending || !governanceStarted {
		t.Fatalf("decision replan resolve continuation pending=%v started=%v for key %q, want queued background work", governancePending, governanceStarted, expectedKey)
	}
	if pendingRef != expectedRef {
		t.Fatalf("pending governance continuation = %+v, want %+v", pendingRef, expectedRef)
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
	if body.Snapshot.RunCommand == nil ||
		body.Snapshot.RunCommand.Intent != "replan" ||
		body.Snapshot.RunCommand.Reason != "decision_replan_resolved" ||
		body.Snapshot.RunCommand.DecisionID != started.BlockingTicketID ||
		body.Snapshot.RunCommand.Source != "governance" ||
		body.Snapshot.RunCommand.RequestID == "" {
		t.Fatalf("projected run command = %+v, want decision-scoped replan command", body.Snapshot.RunCommand)
	}
}

func TestDecisionResolveEndpointStoresLongResolutionIDWithSafeFileName(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	runID := "run-decision-long-resolution-http"
	ticketID := "run-001-repair-001-of-llm-after-approval-" + strings.Repeat("context-gate-retry-", 18) + "context-integrity-decision"

	workspace, started := seedBlockedDecisionRunWithTicketForHTTP(t, dataRoot, "decision long resolution http workspace", runID, ticketID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+url.PathEscape(started.BlockingTicketID)+"/resolve?workspaceId="+url.QueryEscape(workspace.ID), strings.NewReader(`{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"http-long-id"}}`))
	resolveReq.Header.Set("Content-Type", "application/json")
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
	if resolved.Decision.ResolutionID == "" || !strings.Contains(resolved.Decision.ResolutionID, started.BlockingTicketID) {
		t.Fatalf("resolutionID = %q, want workspace-scoped long ticket id", resolved.Decision.ResolutionID)
	}

	files := resolutionFilesForHTTP(t, dataRoot)
	if len(files) != 1 {
		t.Fatalf("resolution file count = %d, want 1: %v", len(files), files)
	}
	name := strings.TrimSuffix(filepath.Base(files[0]), filepath.Ext(files[0]))
	if len(name) > 160 {
		t.Fatalf("resolution filename segment len = %d, want <= 160, file=%s", len(name), files[0])
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return nil, nil
	}
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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

func TestServerStateSkipsReplanContinuationWhenRuntimeSuccessorExists(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	const workspaceID = "ws-replan-successor"
	const runID = "run-replan-successor"
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan-successor",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{
			{
				ID:          runID + "-replan-001",
				WorkspaceID: workspaceID,
				State:       string(common.RunStateRunning),
				ExecutionContext: map[string]any{
					"replanContinuation": map[string]any{"predecessorRunId": runID},
				},
			},
		}, nil
	}
	state.resumeWorkspaceExecution = func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		t.Fatalf("resumeWorkspaceExecution should not be called when runtime successor exists")
		return appservice.WorkspaceExecutionActivation{}, nil
	}

	state.scheduleReplanContinuationIfNeeded(context.Background(), workspaceID, runID, 3)
	time.Sleep(50 * time.Millisecond)
}

func TestServerStateReplanContinuationIgnoresPrefixOnlySuccessor(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	const workspaceID = "ws-replan-prefix-successor"
	const runID = "run-006-replan-001"
	activated := make(chan string, 1)
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{
			{
				ID:             "ticket-replan-prefix-successor",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{
			{
				ID:          runID + "-replan-001",
				WorkspaceID: workspaceID,
				State:       string(common.RunStateFailed),
			},
		}, nil
	}
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, gotWorkspaceID string, _ appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		activated <- gotWorkspaceID
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: runID + "-replan-002", WorkspaceID: gotWorkspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	state.scheduleReplanContinuationIfNeeded(context.Background(), workspaceID, runID, 3)

	select {
	case gotWorkspaceID := <-activated:
		if gotWorkspaceID != workspaceID {
			t.Fatalf("activated workspace = %q, want %s", gotWorkspaceID, workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("prefix-only replan successor incorrectly skipped continuation")
	}
}

func TestServerStateReplanContinuationRetriesAfterActivationFailure(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return nil, nil
	}
	var calls atomic.Int32
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
		state.runContinuations.replanMu.Lock()
		defer state.runContinuations.replanMu.Unlock()
		_, ok := state.runContinuations.replanStarted[workspaceID+"/"+runID]
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

func TestServerStateReplanContinuationWatcherDoesNotDuplicateRuntimeSuccessor(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	activated := make(chan struct{}, 1)
	state.resumeWorkspaceExecution = func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		activated <- struct{}{}
		return appservice.WorkspaceExecutionActivation{}, nil
	}

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-replan-watcher",
		RunID:       "run-replan-watcher",
		SubjectID:   "run-replan-watcher",
		Payload:     events.RunPayload("", "", common.RunStateSuperseded, "", "", ""),
	}); err != nil {
		t.Fatalf("publish superseded run event: %v", err)
	}

	select {
	case <-activated:
		t.Fatalf("run state watcher scheduled duplicate replan continuation")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServerStateAutoGovernanceResolvedSchedulesResumeAndReplanContinuation(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return nil, nil
	}
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if workspaceID != "ws-auto-replan" {
			t.Fatalf("workspaceID = %q, want ws-auto-replan", workspaceID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-auto-replan-continuation", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       governanceevents.TopicGovernanceAutoResolved,
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

func TestServerStateOwnerResolvedEventsScheduleGovernanceResume(t *testing.T) {
	for _, tc := range []struct {
		name  string
		topic string
	}{
		{name: "decision ticket resolved", topic: decisionevents.TopicTicketResolved},
		{name: "approval resolved", topic: approvalevents.TopicApprovalResolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
			if state.eventHub == nil {
				t.Fatalf("server state eventHub is nil, want shared session event hub")
			}
			resumed := make(chan string, 1)
			state.resumeRun = func(_ context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
				if runID != "run-owner-resolved" {
					t.Fatalf("runID = %q, want run-owner-resolved", runID)
				}
				resumed <- runID
				return appservice.Run{ID: runID, WorkspaceID: "ws-owner-resolved", State: string(common.RunStateRunning)}, nil
			}

			if err := state.eventHub.Publish(context.Background(), events.Envelope{
				Topic:       tc.topic,
				WorkspaceID: "ws-owner-resolved",
				RunID:       "run-owner-resolved",
				SubjectID:   "governance-owner-resolved",
				Payload:     map[string]any{"status": "resolved"},
			}); err != nil {
				t.Fatalf("publish owner resolved event: %v", err)
			}

			select {
			case <-resumed:
			case <-time.After(time.Second):
				t.Fatalf("%s event did not schedule run resume", tc.topic)
			}
		})
	}
}

func TestServerStateDecisionReplanResolvedEventProjectsReplanCommand(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	const workspaceID = "ws-owner-replan-resolved"
	const runID = "run-owner-replan-resolved"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "waiting_decision", ActiveRunID: runID},
	})

	if err := state.eventHub.Publish(context.Background(), events.Envelope{
		Topic:       decisionevents.TopicTicketResolved,
		WorkspaceID: workspaceID,
		RunID:       runID,
		SubjectID:   "ticket-owner-replan-resolved",
		Payload: map[string]any{
			events.PayloadKeyType: decisioncommon.ResolutionTypeReplan,
		},
	}); err != nil {
		t.Fatalf("publish replan decision resolved event: %v", err)
	}

	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "replan" ||
		snapshot.RunCommand.Reason != "decision_replan_resolved" ||
		snapshot.RunCommand.RequestID == "" ||
		snapshot.RunCommand.Source != "governance" ||
		snapshot.RunCommand.DecisionID != "ticket-owner-replan-resolved" {
		t.Fatalf("projected run command = %+v, want accepted governance replan command with decision scope", snapshot.RunCommand)
	}
}

func TestServerStateRecoverReplanContinuationsSchedulesSupersededReplanRuns(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
	state := newTestServerState(t, normalizeConfig(Config{
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

	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: "ws-duplicate-resume",
		RunID:       "run-duplicate-resume",
	}}, 0)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("initial governance resume did not start")
	}
	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: "ws-duplicate-resume",
		RunID:       "run-duplicate-resume",
	}, {
		WorkspaceID: "ws-duplicate-resume",
		RunID:       "run-duplicate-resume",
	}}, 0)
	time.Sleep(50 * time.Millisecond)
	if got := state.metrics.runQueueWaiting.Load(); got != 0 {
		t.Fatalf("runQueueWaiting = %d, want duplicate governance resume coalesced", got)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want one in-flight call", got)
	}

	close(release)
	waitForCondition(t, time.Second, func() bool {
		state.runContinuations.governanceMu.Lock()
		_, stillMarked := state.runContinuations.governanceStarted[governanceRunResumeKey(governanceRunResumeRef{WorkspaceID: "ws-duplicate-resume", RunID: "run-duplicate-resume"})]
		state.runContinuations.governanceMu.Unlock()
		return calls.Load() == 1 && state.metrics.runQueueWaiting.Load() == 0 && !stillMarked
	})
	state.runContinuations.governanceMu.Lock()
	_, stillMarked := state.runContinuations.governanceStarted[governanceRunResumeKey(governanceRunResumeRef{WorkspaceID: "ws-duplicate-resume", RunID: "run-duplicate-resume"})]
	state.runContinuations.governanceMu.Unlock()
	if stillMarked {
		t.Fatalf("governance resume mark was not cleared after completion")
	}
}

func TestServerStateGovernanceRunResumeAllowsSameRunIDAcrossWorkspaces(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 2,
	}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	resumed := make(chan string, 2)
	errs := make(chan error, 2)
	state.resumeRun = func(ctx context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
		if runID != "run-001" {
			err := fmt.Errorf("runID = %q, want run-001", runID)
			errs <- err
			return appservice.Run{}, err
		}
		workspaceID := string(common.RuntimeWorkspaceIDFromContext(ctx))
		if workspaceID == "" {
			err := fmt.Errorf("workspace scope is empty")
			errs <- err
			return appservice.Run{}, err
		}
		resumed <- workspaceID
		return appservice.Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateCompleted)}, nil
	}

	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: "ws-governance-a",
		RunID:       "run-001",
	}, {
		WorkspaceID: "ws-governance-b",
		RunID:       "run-001",
	}}, 0)

	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case err := <-errs:
			t.Fatal(err)
		case workspaceID := <-resumed:
			got[workspaceID] = true
		case <-time.After(time.Second):
			t.Fatalf("governance resumes = %#v, want both workspaces", got)
		}
	}
	if !got["ws-governance-a"] || !got["ws-governance-b"] {
		t.Fatalf("governance resumes = %#v, want both workspaces", got)
	}
}

func TestServerStateGovernanceRunResumeUsesWorkspaceContext(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{
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
	resumed := make(chan common.WorkspaceID, 1)
	state.resumeRun = func(ctx context.Context, _, _, runID string, _ ...map[string]any) (appservice.Run, error) {
		if runID != "run-duplicate-resume" {
			t.Fatalf("runID = %q, want run-duplicate-resume", runID)
		}
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		resumed <- workspaceID
		return appservice.Run{ID: runID, WorkspaceID: string(workspaceID), State: string(common.RunStateCompleted)}, nil
	}

	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: "ws-governance-resume",
		RunID:       "run-duplicate-resume",
	}}, 0)
	select {
	case workspaceID := <-resumed:
		if workspaceID != "ws-governance-resume" {
			t.Fatalf("resume workspace context = %q, want ws-governance-resume", workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("governance resume did not run")
	}
}

func TestServerStateGovernanceRunResumeRejectsMissingWorkspaceScope(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateWithBackground(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil, routine)

	if key := governanceRunResumeKey(governanceRunResumeRef{RunID: "run-missing-scope"}); key != "" {
		t.Fatalf("governance resume key = %q, want empty for missing workspace scope", key)
	}
	if !state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		RunID: "run-missing-scope",
	}}, 0) {
		t.Fatal("scheduleGovernanceRunResumeRefs returned false for invalid no-op ref")
	}
	state.runContinuations.governanceMu.Lock()
	pendingCount := len(state.runContinuations.governancePending)
	state.runContinuations.governanceMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("governance pending count = %d, want no pending entry without workspace scope", pendingCount)
	}
	if routine.asyncFunctionCalls != 0 {
		t.Fatalf("background submissions = %d, want no work without workspace scope", routine.asyncFunctionCalls)
	}
}

func TestServerStateRecoversResolvedDecisionBlockedRun(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	const workspaceID = "ws-resolved-decision-recovery"
	const runID = "run-resolved-decision-recovery"
	const ticketID = "ticket-resolved-decision-recovery"
	resumed := make(chan common.WorkspaceID, 1)
	currentRun := appservice.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            string(common.RunStateBlockedOnDecision),
		BlockingTicketID: ticketID,
	}
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{{ID: workspaceID, State: string(common.WorkspaceStateBlocked), ActiveRunID: runID}}, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{currentRun}, nil
	}
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.DecisionTicketStateResolved),
		}}, nil
	}
	state.resumeRun = func(ctx context.Context, _, _, id string, _ ...map[string]any) (appservice.Run, error) {
		if id != runID {
			t.Fatalf("runID = %q, want %s", id, runID)
		}
		workspaceScope := common.RuntimeWorkspaceIDFromContext(ctx)
		resumed <- workspaceScope
		currentRun = appservice.Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateRunning)}
		return currentRun, nil
	}

	if err := state.runContinuations.recoverResolvedGovernanceBlockedRuns(context.Background()); err != nil {
		t.Fatalf("recoverResolvedGovernanceBlockedRuns: %v", err)
	}
	select {
	case workspaceScope := <-resumed:
		if workspaceScope != workspaceID {
			t.Fatalf("resume workspace scope = %q, want %s", workspaceScope, workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("resolved governance blocked run was not resumed")
	}
}

func TestServerStateRecoversResolvedDecisionBlockedRunWithWorkspaceScope(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	const runID = "run-001"
	const workspaceA = "ws-resolved-decision-scope-a"
	const workspaceB = "ws-resolved-decision-scope-b"
	const ticketA = "ticket-resolved-decision-scope-a"
	const ticketB = "ticket-resolved-decision-scope-b"
	resumed := make(chan common.WorkspaceID, 1)
	runsByWorkspace := map[string]appservice.Run{
		workspaceA: {
			ID:               runID,
			WorkspaceID:      workspaceA,
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: ticketA,
		},
		workspaceB: {
			ID:               runID,
			WorkspaceID:      workspaceB,
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: ticketB,
		},
	}
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{
			{ID: workspaceA, State: string(common.WorkspaceStateBlocked), ActiveRunID: runID},
			{ID: workspaceB, State: string(common.WorkspaceStateBlocked), ActiveRunID: runID},
		}, nil
	}
	state.listRuns = func(_ context.Context, _, _, workspaceID string) ([]appservice.Run, error) {
		run, ok := runsByWorkspace[workspaceID]
		if !ok {
			return nil, common.ErrNotFound
		}
		return []appservice.Run{run}, nil
	}
	state.listDecisions = func(_ context.Context, _, _, workspaceID, gotRunID string) ([]appservice.Decision, error) {
		if gotRunID != runID {
			t.Fatalf("runID = %q, want %s", gotRunID, runID)
		}
		switch workspaceID {
		case workspaceA:
			return []appservice.Decision{{
				ID:          ticketA,
				WorkspaceID: workspaceA,
				RunID:       runID,
				State:       string(common.DecisionTicketStateOpen),
			}}, nil
		case workspaceB:
			return []appservice.Decision{{
				ID:          ticketB,
				WorkspaceID: workspaceB,
				RunID:       runID,
				State:       string(common.DecisionTicketStateResolved),
			}}, nil
		default:
			return nil, common.ErrNotFound
		}
	}
	state.resumeRun = func(ctx context.Context, _, _, id string, _ ...map[string]any) (appservice.Run, error) {
		if id != runID {
			t.Fatalf("runID = %q, want %s", id, runID)
		}
		workspaceScope := common.RuntimeWorkspaceIDFromContext(ctx)
		resumed <- workspaceScope
		run := appservice.Run{ID: runID, WorkspaceID: string(workspaceScope), State: string(common.RunStateRunning)}
		runsByWorkspace[string(workspaceScope)] = run
		return run, nil
	}

	if err := state.runContinuations.recoverResolvedGovernanceBlockedRuns(context.Background()); err != nil {
		t.Fatalf("recoverResolvedGovernanceBlockedRuns: %v", err)
	}
	select {
	case workspaceScope := <-resumed:
		if workspaceScope != workspaceB {
			t.Fatalf("resume workspace scope = %q, want %s", workspaceScope, workspaceB)
		}
	case <-time.After(time.Second):
		t.Fatalf("resolved governance blocked run was not resumed")
	}
	select {
	case workspaceScope := <-resumed:
		t.Fatalf("unexpected second workspace resume: %s", workspaceScope)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServerStateGovernanceResumeKeepsPendingAfterBackgroundSubmitFailure(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-governance-resume-pending"
	const runID = "run-governance-resume-pending"

	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: workspaceID,
		RunID:       runID,
	}}, 0)
	pendingKey := governanceRunResumeKey(governanceRunResumeRef{WorkspaceID: workspaceID, RunID: runID})
	state.runContinuations.governanceMu.Lock()
	_, pending := state.runContinuations.governancePending[pendingKey]
	state.runContinuations.governanceMu.Unlock()
	if !pending {
		t.Fatalf("governance resume pending entry was dropped after background submit failure")
	}

	routine.err = nil
	resumed := make(chan common.WorkspaceID, 1)
	currentRun := appservice.Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateRunning)}
	state.resumeRun = func(ctx context.Context, _, _, id string, _ ...map[string]any) (appservice.Run, error) {
		if id != runID {
			t.Fatalf("runID = %q, want %s", id, runID)
		}
		workspaceScope := common.RuntimeWorkspaceIDFromContext(ctx)
		resumed <- workspaceScope
		currentRun = appservice.Run{ID: runID, WorkspaceID: string(workspaceScope), State: string(common.RunStateRunning)}
		return currentRun, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{currentRun}, nil
	}
	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: workspaceID,
		RunID:       runID,
	}}, 0)
	if routine.function == nil {
		t.Fatal("governance resume was not rescheduled after background submit recovery")
	}
	routine.function()
	select {
	case workspaceScope := <-resumed:
		if workspaceScope != workspaceID {
			t.Fatalf("resume workspace scope = %q, want %s", workspaceScope, workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("governance resume pending entry did not execute after reschedule")
	}
	state.runContinuations.governanceMu.Lock()
	_, pending = state.runContinuations.governancePending[pendingKey]
	state.runContinuations.governanceMu.Unlock()
	if pending {
		t.Fatalf("governance resume pending entry was not cleared after successful resume")
	}
}

func TestServerStateGovernanceResumeDoesNotCompleteWhenReadbackStillBlockedOnResolvedDecision(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-governance-readback"
	const runID = "run-governance-readback"
	const ticketID = "ticket-governance-readback"
	staleRun := appservice.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            string(common.RunStateBlockedOnDecision),
		BlockingTicketID: ticketID,
	}
	state.resumeRun = func(context.Context, string, string, string, ...map[string]any) (appservice.Run, error) {
		return staleRun, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{staleRun}, nil
	}
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.DecisionTicketStateResolved),
		}}, nil
	}

	state.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
		WorkspaceID: workspaceID,
		RunID:       runID,
	}}, 0)
	if routine.function == nil {
		t.Fatal("governance resume background function was not queued")
	}
	routine.function()

	pendingKey := governanceRunResumeKey(governanceRunResumeRef{WorkspaceID: workspaceID, RunID: runID})
	state.runContinuations.governanceMu.Lock()
	_, pending := state.runContinuations.governancePending[pendingKey]
	state.runContinuations.governanceMu.Unlock()
	if !pending {
		t.Fatalf("governance resume pending entry was cleared despite unresolved owner readback")
	}
	diagnostics := state.schedulerDiagnostics.snapshot(workspaceID, runID)
	if diagnostics == nil || diagnostics.LatestContinuation == nil {
		t.Fatalf("scheduler diagnostics = %+v, want owner readback failure", diagnostics)
	}
	latest := diagnostics.LatestContinuation
	if latest.Kind != schedulerKindGovernanceContinuation ||
		latest.State != schedulerDiagnosticStateFailed ||
		latest.Reason != schedulerReasonOwnerReadbackNotAdvanced ||
		latest.Readback == nil ||
		latest.Readback.Reason != "blocked_on_resolved_decision" {
		t.Fatalf("latest continuation = %+v, want governance readback failure", latest)
	}
}

func TestServerStateWorkspaceExecutionKeepsPendingAfterBackgroundSubmitFailure(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-workspace-execution-pending"
	req := workspaceRestartRequest{
		WorkspaceID: workspaceID,
		Priority:    4,
		Command: appservice.ActivateWorkspaceExecutionCommand{
			Restart: true,
		},
	}

	if state.runContinuations.scheduleWorkspaceRestart(req) {
		t.Fatalf("workspace execution scheduling returned success after background submit failure")
	}
	state.runContinuations.restartMu.Lock()
	_, pending := state.runContinuations.restartPending["workspace_restart/"+workspaceID]
	state.runContinuations.restartMu.Unlock()
	if !pending {
		t.Fatalf("workspace execution pending entry was dropped after background submit failure")
	}

	routine.err = nil
	activated := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-workspace-execution-pending", WorkspaceID: workspaceID, State: string(common.RunStateRunning)},
		}, nil
	}
	if !state.runContinuations.scheduleWorkspaceRestart(req) {
		t.Fatalf("workspace execution scheduling failed after background routine recovery")
	}
	if routine.function == nil {
		t.Fatal("workspace execution was not rescheduled after background submit recovery")
	}
	routine.function()
	select {
	case cmd := <-activated:
		if !cmd.Restart || cmd.Priority != 4 {
			t.Fatalf("activation command = %#v, want restart priority 4", cmd)
		}
	case <-time.After(time.Second):
		t.Fatalf("workspace execution pending entry did not execute after reschedule")
	}
	state.runContinuations.restartMu.Lock()
	_, pending = state.runContinuations.restartPending["workspace_restart/"+workspaceID]
	state.runContinuations.restartMu.Unlock()
	if pending {
		t.Fatalf("workspace execution pending entry was not cleared after successful activation")
	}
}

func TestServerStateRunContinuationKeepsPendingAfterBackgroundSubmitFailure(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-run-continuation-pending"
	const runID = "run-continuation-pending"
	req := runContinuationRequest{
		WorkspaceID: workspaceID,
		RunID:       runID,
		Action:      "start",
		Priority:    2,
	}

	if state.runContinuations.scheduleRunContinuation(req) {
		t.Fatalf("run continuation scheduling returned success after background submit failure")
	}
	state.runContinuations.runMu.Lock()
	_, pending := state.runContinuations.runPending[workspaceID+"/"+runID+"/start"]
	state.runContinuations.runMu.Unlock()
	if !pending {
		t.Fatalf("run continuation pending entry was dropped after background submit failure")
	}

	routine.err = nil
	started := make(chan common.WorkspaceID, 1)
	state.startRun = func(ctx context.Context, _, _, gotRunID string) (appservice.Run, error) {
		if gotRunID != runID {
			t.Fatalf("runID = %q, want %s", gotRunID, runID)
		}
		workspaceScope := common.RuntimeWorkspaceIDFromContext(ctx)
		started <- workspaceScope
		return appservice.Run{ID: runID, WorkspaceID: string(workspaceScope), State: string(common.RunStateRunning)}, nil
	}
	state.listRuns = func(_ context.Context, _, _, gotWorkspaceID string) ([]appservice.Run, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		return []appservice.Run{{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateRunning)}}, nil
	}
	if !state.runContinuations.scheduleRunContinuation(req) {
		t.Fatalf("run continuation scheduling failed after background routine recovery")
	}
	if routine.function == nil {
		t.Fatal("run continuation was not rescheduled after background submit recovery")
	}
	routine.function()
	select {
	case workspaceScope := <-started:
		if workspaceScope != workspaceID {
			t.Fatalf("start workspace scope = %q, want %s", workspaceScope, workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("run continuation pending entry did not execute after reschedule")
	}
	state.runContinuations.runMu.Lock()
	_, pending = state.runContinuations.runPending[workspaceID+"/"+runID+"/start"]
	state.runContinuations.runMu.Unlock()
	if pending {
		t.Fatalf("run continuation pending entry was not cleared after successful start")
	}
}

func TestServerStateRunContinuationDoesNotCompleteWhenReadbackStillBlockedOnResolvedDecision(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-run-continuation-readback"
	const runID = "run-continuation-readback"
	const ticketID = "ticket-run-continuation-readback"
	req := runContinuationRequest{
		WorkspaceID: workspaceID,
		RunID:       runID,
		Action:      "resume",
		Priority:    2,
	}
	staleRun := appservice.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            string(common.RunStateBlockedOnDecision),
		BlockingTicketID: ticketID,
	}
	state.resumeRun = func(ctx context.Context, _, _, gotRunID string, _ ...map[string]any) (appservice.Run, error) {
		if gotRunID != runID {
			t.Fatalf("runID = %q, want %s", gotRunID, runID)
		}
		if got := common.RuntimeWorkspaceIDFromContext(ctx); got != workspaceID {
			t.Fatalf("workspace scope = %q, want %s", got, workspaceID)
		}
		return staleRun, nil
	}
	state.listRuns = func(_ context.Context, _, _, gotWorkspaceID string) ([]appservice.Run, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		return []appservice.Run{staleRun}, nil
	}
	state.listDecisions = func(_ context.Context, _, _, gotWorkspaceID, gotRunID string) ([]appservice.Decision, error) {
		if gotWorkspaceID != workspaceID || gotRunID != runID {
			t.Fatalf("decision lookup scope = %s/%s, want %s/%s", gotWorkspaceID, gotRunID, workspaceID, runID)
		}
		return []appservice.Decision{{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.DecisionTicketStateResolved),
		}}, nil
	}

	if !state.runContinuations.scheduleRunContinuation(req) {
		t.Fatalf("run continuation scheduling failed")
	}
	if routine.function == nil {
		t.Fatal("run continuation background function was not queued")
	}
	routine.function()

	state.runContinuations.runMu.Lock()
	_, pending := state.runContinuations.runPending[workspaceID+"/"+runID+"/resume"]
	state.runContinuations.runMu.Unlock()
	if !pending {
		t.Fatalf("run continuation pending entry was cleared despite unresolved owner readback")
	}
	diagnostics := state.schedulerDiagnostics.snapshot(workspaceID, runID)
	if diagnostics == nil || diagnostics.LatestContinuation == nil {
		t.Fatalf("scheduler diagnostics = %+v, want owner readback failure", diagnostics)
	}
	latest := diagnostics.LatestContinuation
	if latest.Kind != schedulerKindRunContinuation ||
		latest.State != schedulerDiagnosticStateFailed ||
		latest.Reason != schedulerReasonOwnerReadbackNotAdvanced ||
		latest.Recoverability != schedulerRecoverabilityInspect {
		t.Fatalf("latest continuation = %+v, want owner readback failure", latest)
	}
}

func TestWorkorchDSchedulerWorkspaceRunCommandEnqueuesRunContinuationByWorkspaceScope(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-run-command"
	const runID = "run-001"

	if err := publishHTTPTestCommand(context.Background(), hub, events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyRunID:       runID,
			events.PayloadKeyIntent:      "resume",
			events.PayloadKeyAction:      "resume",
			events.PayloadKeyPriority:    5,
		},
	}); err != nil {
		t.Fatalf("publish scheduler command: %v", err)
	}

	state.runContinuations.runMu.Lock()
	req, pending := state.runContinuations.runPending[workspaceID+"/"+runID+"/resume"]
	state.runContinuations.runMu.Unlock()
	if !pending {
		t.Fatalf("run continuation command did not create workspace-scoped pending entry")
	}
	if req.WorkspaceID != workspaceID || req.RunID != runID || req.Action != "resume" || req.Priority != 5 {
		t.Fatalf("run continuation pending request = %+v, want scoped resume priority 5", req)
	}
}

func TestWorkorchDWorkspaceRunCommandResumeEnqueuesRunContinuation(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-workspace-run-resume"
	const runID = "run-001"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	if err := publishHTTPTestCommand(context.Background(), hub, events.Envelope{
		Topic:        workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		SourceModule: "workorchd/test",
		WorkspaceID:  workspaceID,
		RunID:        runID,
		Revision:     17,
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyRunID:       runID,
			events.PayloadKeyIntent:      "resume",
			events.PayloadKeyRequestID:   "request-resume",
			events.PayloadKeySource:      "tui",
			events.PayloadKeyPriority:    7,
		},
	}); err != nil {
		t.Fatalf("publish workspace run command: %v", err)
	}

	state.runContinuations.runMu.Lock()
	req, pending := state.runContinuations.runPending[workspaceID+"/"+runID+"/resume"]
	state.runContinuations.runMu.Unlock()
	if !pending {
		t.Fatalf("workspace run resume command did not create run continuation pending entry")
	}
	if req.WorkspaceID != workspaceID || req.RunID != runID || req.Action != "resume" || req.Priority != 7 {
		t.Fatalf("workspace run resume pending request = %+v, want scoped resume priority 7", req)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-resume" ||
		snapshot.RunCommand.Source != "tui" ||
		snapshot.RunCommand.Revision <= 0 {
		t.Fatalf("projected run command = %+v, want accepted resume command", snapshot.RunCommand)
	}
}

func TestWorkorchDWorkspaceRunCommandGeneratesRequestIDWhenMissing(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-generated-request"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	if err := state.publishWorkorchDSchedulerCommand(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
		events.PayloadKeyIntent:      "resume",
		events.PayloadKeyReason:      "test_generated_request",
	}); err != nil {
		t.Fatalf("publish workspace run command: %v", err)
	}

	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID == "" ||
		snapshot.RunCommand.Source != "http" ||
		!strings.Contains(snapshot.RunCommand.RequestID, "workspace_run") {
		t.Fatalf("projected run command = %+v, want generated resume request id and default source", snapshot.RunCommand)
	}
}

func TestWorkorchDActivationHTTPProjectsRequestID(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation request id", "workspaces/activation-request-id")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"goalId":"`+goalID+`",
		"doDId":"`+dodID+`",
		"intent":"resume",
		"requestId":"request-http-activation",
		"reason":"http_test",
		"source":"web",
		"definitionRevision":"rev-http-activation",
		"decisionId":"ticket-http-activation"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("activation status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode activation response: %v", err)
	}
	if body["accepted"] != true ||
		body["intent"] != "resume" ||
		body["requestId"] != "request-http-activation" ||
		body["source"] != "web" ||
		body["reason"] != "http_test" ||
		body["definitionRevision"] != "rev-http-activation" ||
		body["decisionId"] != "ticket-http-activation" {
		t.Fatalf("activation response = %+v, want accepted command metadata", body)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-http-activation" ||
		snapshot.RunCommand.Reason != "http_test" ||
		snapshot.RunCommand.Source != "web" ||
		snapshot.RunCommand.DefinitionRevision != "rev-http-activation" ||
		snapshot.RunCommand.DecisionID != "ticket-http-activation" {
		t.Fatalf("projected run command = %+v, want HTTP request id, reason, source, definition revision, and decision id", snapshot.RunCommand)
	}
}

func TestWorkorchDRunResumeHTTPProjectsReplanIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "run resume replan intent", "workspaces/run-resume-replan-intent")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-http-replan/resume", strings.NewReader(`{
		"intent":"replan",
		"requestId":"request-http-replan",
		"reason":"decision_replan_resolved",
		"source":"web",
		"definitionRevision":"rev-http-replan",
		"decisionId":"ticket-http-replan"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run resume replan status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode run resume response: %v", err)
	}
	if body["accepted"] != true ||
		body["intent"] != "replan" ||
		body["requestId"] != "request-http-replan" ||
		body["source"] != "web" ||
		body["reason"] != "decision_replan_resolved" ||
		body["definitionRevision"] != "rev-http-replan" ||
		body["decisionId"] != "ticket-http-replan" ||
		body["runId"] != "run-http-replan" {
		t.Fatalf("run resume response = %+v, want accepted replan metadata", body)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "replan" ||
		snapshot.RunCommand.RunID != "run-http-replan" ||
		snapshot.RunCommand.RequestID != "request-http-replan" ||
		snapshot.RunCommand.IdempotencyKey != "replan/"+workspace.ID+"/run-http-replan/ticket-http-replan" ||
		snapshot.RunCommand.Reason != "decision_replan_resolved" ||
		snapshot.RunCommand.Source != "web" ||
		snapshot.RunCommand.DefinitionRevision != "rev-http-replan" ||
		snapshot.RunCommand.DecisionID != "ticket-http-replan" {
		t.Fatalf("projected run command = %+v, want HTTP replan command metadata", snapshot.RunCommand)
	}
}

func TestWorkorchDWorkspaceRunCommandReplanUsesScopedIdempotencyKey(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-replan-idempotency"
	const runID = "run-scheduler-replan-idempotency"
	const decisionID = "ticket-scheduler-replan-idempotency"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	envelope := events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Revision:    21,
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyRunID:       runID,
			events.PayloadKeyIntent:      "replan",
			events.PayloadKeyDecisionID:  decisionID,
			events.PayloadKeyRequestID:   "request-replan-1",
			events.PayloadKeySource:      "tui",
		},
	}
	if err := publishHTTPTestCommand(context.Background(), hub, envelope); err != nil {
		t.Fatalf("publish first replan command: %v", err)
	}
	envelope.Revision = 22
	envelope.Payload[events.PayloadKeyRequestID] = "request-replan-2"
	if err := publishHTTPTestCommand(context.Background(), hub, envelope); err != nil {
		t.Fatalf("publish duplicate replan command: %v", err)
	}

	expectedKey := "replan/" + workspaceID + "/" + runID + "/" + decisionID
	state.runContinuations.governanceMu.Lock()
	pendingCount := len(state.runContinuations.governancePending)
	startedCount := len(state.runContinuations.governanceStarted)
	_, pending := state.runContinuations.governancePending[expectedKey]
	_, started := state.runContinuations.governanceStarted[expectedKey]
	state.runContinuations.governanceMu.Unlock()
	if pendingCount != 1 || startedCount != 1 || !pending || !started {
		t.Fatalf("governance queue pending=%d started=%d key pending/started=%v/%v, want one scoped replan key %s", pendingCount, startedCount, pending, started, expectedKey)
	}
	if routine.asyncFunctionCalls != 1 {
		t.Fatalf("background submissions = %d, want duplicate replan coalesced", routine.asyncFunctionCalls)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "replan" ||
		snapshot.RunCommand.IdempotencyKey != expectedKey ||
		snapshot.RunCommand.RequestID != "request-replan-2" {
		t.Fatalf("projected run command = %+v, want scoped replan idempotency key", snapshot.RunCommand)
	}
}

func TestWorkorchDRunResumeHTTPReturnsGeneratedRequestID(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "run resume generated request id", "workspaces/run-resume-generated-request-id")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-http-generated/resume", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run resume status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode run resume response: %v", err)
	}
	requestID, _ := body["requestId"].(string)
	if body["accepted"] != true ||
		body["intent"] != "resume" ||
		body["source"] != "http" ||
		body["runId"] != "run-http-generated" ||
		requestID == "" ||
		!strings.Contains(requestID, "workspace_run") {
		t.Fatalf("run resume response = %+v, want generated request id and default command metadata", body)
	}
}

func TestWorkorchDActivationHTTPRejectsRunScopedIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation rejects replan", "workspaces/activation-rejects-replan")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"goalId":"`+goalID+`",
		"doDId":"`+dodID+`",
		"intent":"replan",
		"requestId":"request-http-replan-without-run"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("activation replan status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkorchDRunResumeHTTPRejectsRestartIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "run resume rejects restart intent", "workspaces/run-resume-rejects-restart")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-http-restart/resume", strings.NewReader(`{"intent":"restart","requestId":"request-http-restart"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("run resume restart status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkorchDWorkspaceRestartHTTPProjectsRestartIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "workspace restart intent", "workspaces/workspace-restart-intent")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-restarts", strings.NewReader(`{
		"goalId":"`+goalID+`",
		"doDId":"`+dodID+`",
		"requestId":"request-http-restart",
		"reason":"operator_restart",
		"source":"web",
		"definitionRevision":"rev-http-restart",
		"decisionId":"ticket-http-restart"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("workspace restart status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "restart" ||
		snapshot.RunCommand.RunID != "" ||
		snapshot.RunCommand.RequestID != "request-http-restart" ||
		snapshot.RunCommand.IdempotencyKey != "restart/"+workspace.ID ||
		snapshot.RunCommand.Reason != "operator_restart" ||
		snapshot.RunCommand.Source != "web" ||
		snapshot.RunCommand.DefinitionRevision != "rev-http-restart" ||
		snapshot.RunCommand.DecisionID != "ticket-http-restart" {
		t.Fatalf("projected run command = %+v, want HTTP restart command metadata", snapshot.RunCommand)
	}
}

func TestWorkorchDSchedulerWorkspaceRunCommandEnqueuesWorkspaceRestartByWorkspaceScope(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-restart-command"

	if err := publishHTTPTestCommand(context.Background(), hub, events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: workspaceID,
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:   workspaceID,
			events.PayloadKeyIntent:        "restart",
			events.PayloadKeyGoalID:        "goal-restart",
			events.PayloadKeyDoDID:         "dod-restart",
			events.PayloadKeyLLMProviderID: "llm-restart",
			events.PayloadKeyPriority:      6,
		},
	}); err != nil {
		t.Fatalf("publish scheduler command: %v", err)
	}

	state.runContinuations.restartMu.Lock()
	req, pending := state.runContinuations.restartPending["workspace_restart/"+workspaceID]
	state.runContinuations.restartMu.Unlock()
	if !pending {
		t.Fatalf("workspace restart command did not create workspace-scoped pending entry")
	}
	if req.WorkspaceID != workspaceID || req.Priority != 6 || !req.Command.Restart || req.Command.GoalID != "goal-restart" || req.Command.DoDID != "dod-restart" || req.Command.LLMProviderID != "llm-restart" {
		t.Fatalf("workspace restart pending request = %+v, want scoped restart command", req)
	}
	state.runContinuations.runMu.Lock()
	runPendingCount := len(state.runContinuations.runPending)
	state.runContinuations.runMu.Unlock()
	state.runContinuations.governanceMu.Lock()
	governancePendingCount := len(state.runContinuations.governancePending)
	state.runContinuations.governanceMu.Unlock()
	if runPendingCount != 0 || governancePendingCount != 0 {
		t.Fatalf("restart queued runPending=%d governancePending=%d, want only workspace_restart queue", runPendingCount, governancePendingCount)
	}
}

func TestWorkorchDWorkspaceRunCommandRestartEnqueuesWorkspaceRestart(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-workspace-run-restart"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	envelope := events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: workspaceID,
		Revision:    31,
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:   workspaceID,
			events.PayloadKeyIntent:        "restart",
			events.PayloadKeyRequestID:     "request-restart-command",
			events.PayloadKeySource:        "tui",
			events.PayloadKeyGoalID:        "goal-restart",
			events.PayloadKeyDoDID:         "dod-restart",
			events.PayloadKeyLLMProviderID: "llm-restart",
			events.PayloadKeyPriority:      8,
		},
	}
	if err := publishHTTPTestCommand(context.Background(), hub, envelope); err != nil {
		t.Fatalf("publish workspace run restart command: %v", err)
	}
	envelope.Revision = 32
	envelope.Payload[events.PayloadKeyRequestID] = "request-restart-command-duplicate"
	if err := publishHTTPTestCommand(context.Background(), hub, envelope); err != nil {
		t.Fatalf("publish duplicate workspace run restart command: %v", err)
	}

	state.runContinuations.restartMu.Lock()
	req, pending := state.runContinuations.restartPending["workspace_restart/"+workspaceID]
	restartPendingCount := len(state.runContinuations.restartPending)
	restartStartedCount := len(state.runContinuations.restartStarted)
	state.runContinuations.restartMu.Unlock()
	if !pending {
		t.Fatalf("workspace run restart command did not create workspace restart pending entry")
	}
	if restartPendingCount != 1 || restartStartedCount != 1 || routine.asyncFunctionCalls != 1 {
		t.Fatalf("restart pending=%d started=%d background submissions=%d, want duplicate restart coalesced", restartPendingCount, restartStartedCount, routine.asyncFunctionCalls)
	}
	if req.WorkspaceID != workspaceID || req.Priority != 8 || !req.Command.Restart || req.Command.GoalID != "goal-restart" || req.Command.DoDID != "dod-restart" || req.Command.LLMProviderID != "llm-restart" {
		t.Fatalf("workspace run restart pending request = %+v, want scoped restart command", req)
	}
	state.runContinuations.runMu.Lock()
	runPendingCount := len(state.runContinuations.runPending)
	state.runContinuations.runMu.Unlock()
	state.runContinuations.governanceMu.Lock()
	governancePendingCount := len(state.runContinuations.governancePending)
	state.runContinuations.governanceMu.Unlock()
	if runPendingCount != 0 || governancePendingCount != 0 {
		t.Fatalf("workspace run restart queued runPending=%d governancePending=%d, want only workspace_restart queue", runPendingCount, governancePendingCount)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "restart" ||
		snapshot.RunCommand.RequestID != "request-restart-command-duplicate" ||
		snapshot.RunCommand.IdempotencyKey != "restart/"+workspaceID ||
		snapshot.RunCommand.Source != "tui" {
		t.Fatalf("projected run command = %+v, want scoped restart command projection", snapshot.RunCommand)
	}
}

func TestWorkorchDSchedulerCommandRejectsWorkspaceScopeMismatch(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-scheduler-envelope", State: "active"},
	})
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-scheduler-payload", State: "active"},
	})

	err := publishHTTPTestCommand(context.Background(), hub, events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: "ws-scheduler-envelope",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-scheduler-payload",
			events.PayloadKeyIntent:      "restart",
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("workspace scope mismatch err = %v, want conflict", err)
	}

	state.runContinuations.restartMu.Lock()
	pendingCount := len(state.runContinuations.restartPending)
	state.runContinuations.restartMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("restart pending count = %d, want no queued command after scope mismatch", pendingCount)
	}
	envelopeSnapshot, _, err := state.readProjectedWorkspaceSnapshot("ws-scheduler-envelope")
	if err != nil {
		t.Fatalf("read envelope workspace projection: %v", err)
	}
	if envelopeSnapshot.RunCommand != nil {
		t.Fatalf("envelope workspace run command = %+v, want no projection for malformed scope", envelopeSnapshot.RunCommand)
	}
	payloadSnapshot, _, err := state.readProjectedWorkspaceSnapshot("ws-scheduler-payload")
	if err != nil {
		t.Fatalf("read payload workspace projection: %v", err)
	}
	if payloadSnapshot.RunCommand != nil {
		t.Fatalf("payload workspace run command = %+v, want no cross-workspace projection", payloadSnapshot.RunCommand)
	}
}

func TestWorkorchDSchedulerCommandRejectsRunScopeMismatch(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)

	err := publishHTTPTestCommand(context.Background(), hub, events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		WorkspaceID: "ws-scheduler-run-scope",
		RunID:       "run-scheduler-envelope",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-scheduler-run-scope",
			events.PayloadKeyRunID:       "run-scheduler-payload",
			events.PayloadKeyIntent:      "resume",
			events.PayloadKeyAction:      "resume",
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("run scope mismatch err = %v, want conflict", err)
	}

	state.runContinuations.runMu.Lock()
	pendingCount := len(state.runContinuations.runPending)
	state.runContinuations.runMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("run pending count = %d, want no queued command after scope mismatch", pendingCount)
	}
}

func TestWorkorchDSchedulerCommandReturnsErrorWhenRunContinuationQueueFails(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	hub := events.NewMemoryHub()
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	const workspaceID = "ws-scheduler-run-command-fail"
	const runID = "run-scheduler-command-fail"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	err := state.publishWorkorchDSchedulerCommand(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workspaceID, runID, map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
		events.PayloadKeyRunID:       runID,
		events.PayloadKeyIntent:      "resume",
		events.PayloadKeyAction:      "resume",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("publish scheduler command error = %v, want conflict", err)
	}

	state.runContinuations.runMu.Lock()
	_, pending := state.runContinuations.runPending[workspaceID+"/"+runID+"/resume"]
	state.runContinuations.runMu.Unlock()
	if !pending {
		t.Fatalf("run continuation pending entry was dropped after queue failure")
	}
	diagnostics := state.schedulerDiagnostics.snapshot(workspaceID, runID)
	if diagnostics == nil || diagnostics.LatestContinuation == nil {
		t.Fatalf("scheduler diagnostics = %+v, want enqueue failure diagnostic", diagnostics)
	}
	latest := diagnostics.LatestContinuation
	if latest.WorkspaceID != workspaceID ||
		latest.RunID != runID ||
		latest.Kind != schedulerKindRunContinuation ||
		latest.State != schedulerDiagnosticStateFailed ||
		latest.Reason != schedulerReasonEnqueueFailed ||
		latest.Recoverability != schedulerRecoverabilityManualRetry {
		t.Fatalf("latest continuation = %+v, want run continuation enqueue failure", latest)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "rejected" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RunID != runID ||
		snapshot.RunCommand.Error == "" {
		t.Fatalf("projected run command = %+v, want rejected scheduler command", snapshot.RunCommand)
	}
}

func TestWorkorchDSchedulerCommandRequiresAcceptedResponse(t *testing.T) {
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), events.NewMemoryHub(), &recordingBackgroundRoutine{})
	for _, unsubscribe := range state.eventUnsubscribes {
		unsubscribe()
	}
	state.eventUnsubscribes = nil

	err := state.publishWorkorchDSchedulerCommand(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "ws-missing-scheduler", "", map[string]any{
		events.PayloadKeyWorkspaceID: "ws-missing-scheduler",
		events.PayloadKeyIntent:      "resume",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("publish scheduler command error = %v, want conflict for missing accepted response", err)
	}
}

func TestWorkorchDRESTCommandsReturnConflictWhenSchedulerQueueFails(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	handler := newMuxWithState(state)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(ctx)
	})

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "workspace activation", method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-rest-scheduler-fail/execution-activations"},
		{name: "workspace restart", method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-rest-scheduler-fail/execution-restarts"},
		{name: "run start", method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-rest-scheduler-fail/runs/run-rest-scheduler-fail/start"},
		{name: "run resume", method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-rest-scheduler-fail/runs/run-rest-scheduler-fail/resume"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s status = %d, want 409, body=%s", tc.name, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"accepted":true`) {
				t.Fatalf("%s returned accepted body after scheduler queue failure: %s", tc.name, rec.Body.String())
			}
		})
	}
}

func TestWorkorchDGovernanceResolutionReturnsConflictWhenSchedulerQueueFails(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background closed")}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	const workspaceID = "ws-governance-scheduler-fail"
	const runID = "run-governance-scheduler-fail"
	state.resolveDecision = func(context.Context, string, string, string, appservice.ResolveDecisionCommand) (appservice.Decision, error) {
		return appservice.Decision{ID: "ticket-scheduler-fail", WorkspaceID: workspaceID, RunID: runID, State: string(common.DecisionTicketStateResolved)}, nil
	}
	state.approveApproval = func(context.Context, string, string, string, appservice.ResolveApprovalCommand) (appservice.Approval, error) {
		return appservice.Approval{ID: "approval-scheduler-fail", WorkspaceID: workspaceID, RunID: runID, State: string(common.ApprovalStateApproved)}, nil
	}
	handler := newMuxWithState(state)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(ctx)
	})

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "decision resolve", path: "/api/v1/workorch/decisions/ticket-scheduler-fail/resolve"},
		{name: "approval approve", path: "/api/v1/workorch/approvals/approval-scheduler-fail/approve"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s status = %d, want 409, body=%s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestServerStateDoesNotRecoverOpenDecisionBlockedRun(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}), nil)
	const workspaceID = "ws-open-decision-recovery"
	const runID = "run-open-decision-recovery"
	const ticketID = "ticket-open-decision-recovery"
	resumed := make(chan struct{}, 1)
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{{ID: workspaceID, State: string(common.WorkspaceStateBlocked), ActiveRunID: runID}}, nil
	}
	state.listRuns = func(context.Context, string, string, string) ([]appservice.Run, error) {
		return []appservice.Run{{
			ID:               runID,
			WorkspaceID:      workspaceID,
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: ticketID,
		}}, nil
	}
	state.listDecisions = func(context.Context, string, string, string, string) ([]appservice.Decision, error) {
		return []appservice.Decision{{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.DecisionTicketStateOpen),
		}}, nil
	}
	state.resumeRun = func(context.Context, string, string, string, ...map[string]any) (appservice.Run, error) {
		resumed <- struct{}{}
		return appservice.Run{}, nil
	}

	if err := state.runContinuations.recoverResolvedGovernanceBlockedRuns(context.Background()); err != nil {
		t.Fatalf("recoverResolvedGovernanceBlockedRuns: %v", err)
	}
	select {
	case <-resumed:
		t.Fatalf("open decision blocked run should not be resumed")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServerStateRecoverReplanContinuationsSchedulesWaitingDecisionActiveSupersededRun(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	const workspaceID = "ws-replan-waiting-decision"
	const activeRunID = "run-replan-waiting-decision"
	activated := make(chan string, 1)
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		return []appservice.Workspace{
			{ID: workspaceID, State: string(common.WorkspaceStateWaitingDecision), ActiveRunID: activeRunID},
		}, nil
	}
	state.listRuns = func(_ context.Context, _, _, gotWorkspaceID string) ([]appservice.Run, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		return []appservice.Run{
			{ID: "run-replan-older", WorkspaceID: workspaceID, State: string(common.RunStateSuperseded), Priority: 2},
			{ID: activeRunID, WorkspaceID: workspaceID, State: string(common.RunStateSuperseded), Priority: 6},
		}, nil
	}
	state.listDecisions = func(_ context.Context, _, _, gotWorkspaceID, runID string) ([]appservice.Decision, error) {
		if gotWorkspaceID != workspaceID || runID != activeRunID {
			return nil, nil
		}
		return []appservice.Decision{
			{
				ID:             "ticket-replan-waiting-decision",
				WorkspaceID:    workspaceID,
				RunID:          runID,
				State:          string(common.DecisionTicketStateResolved),
				ResolutionType: decisioncommon.ResolutionTypeReplan,
			},
		}, nil
	}
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if gotWorkspaceID != workspaceID {
			t.Fatalf("workspaceID = %q, want %s", gotWorkspaceID, workspaceID)
		}
		if cmd.Priority != 6 {
			t.Fatalf("activation priority = %d, want 6", cmd.Priority)
		}
		activated <- gotWorkspaceID
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: activeRunID + "-replan-001", WorkspaceID: gotWorkspaceID, State: string(common.RunStateRunning)},
		}, nil
	}

	state.recoverReplanContinuations(context.Background())

	select {
	case gotWorkspaceID := <-activated:
		if gotWorkspaceID != workspaceID {
			t.Fatalf("activated workspace = %q, want %s", gotWorkspaceID, workspaceID)
		}
	case <-time.After(time.Second):
		t.Fatalf("waiting decision replan continuation recovery did not schedule activation")
	}
}

func TestServerStateWatchdogDoesNotRecoverReplanContinuations(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), nil, routine)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})

	var workspaceScan atomic.Bool
	state.listWorkspaces = func(context.Context, string, string) ([]appservice.Workspace, error) {
		workspaceScan.Store(true)
		return []appservice.Workspace{
			{ID: "ws-replan-watchdog", State: string(common.WorkspaceStateWaitingDecision), ActiveRunID: "run-replan-watchdog"},
		}, nil
	}
	var activationCount atomic.Int32
	state.resumeWorkspaceExecution = func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		activationCount.Add(1)
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-replan-watchdog-replan-001", WorkspaceID: "ws-replan-watchdog", State: string(common.RunStateRunning)},
		}, nil
	}

	state.scheduleWatchdog()
	if routine.timerTask == nil {
		t.Fatal("watchdog timer was not registered")
	}
	routine.timerTask.Run()

	if workspaceScan.Load() {
		t.Fatal("watchdog scanned workspaces for replan continuation recovery")
	}
	if got := activationCount.Load(); got != 0 {
		t.Fatalf("watchdog activated %d replan continuations, want none", got)
	}
	if got := state.schedulerDiagnostics.snapshot("", "").Watchdog.LatestTickAt; got == "" {
		t.Fatal("watchdog latest tick was not recorded")
	}
}

func TestServerStateRecoverReplanContinuationsRetriesAfterActivationFailure(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), nil)
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
	state.resumeWorkspaceExecution = func(_ context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
		state.runContinuations.replanMu.Lock()
		defer state.runContinuations.replanMu.Unlock()
		_, ok := state.runContinuations.replanStarted[workspaceID+"/"+runID]
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

func TestApprovalBatchResolveEndpointScopesDuplicateApprovalIDsByWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	approvalID := "approval-shared-http"
	workspaceA, startedA := seedBlockedApprovalRunWithApprovalForHTTP(t, dataRoot, "approval shared http workspace a", "run-approval-shared-a", approvalID)
	workspaceB, startedB := seedBlockedApprovalRunWithApprovalForHTTP(t, dataRoot, "approval shared http workspace b", "run-approval-shared-b", approvalID)

	body := strings.NewReader(`{"approvalIds":["` + approvalID + `"],"action":"approve"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/resolve-batch?workspaceId="+url.QueryEscape(workspaceB.ID), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch approval resolve status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resolved approvalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal batch resolved approvals: %v", err)
	}
	if len(resolved.Approvals) != 1 || resolved.Approvals[0].ID != approvalID || resolved.Approvals[0].WorkspaceID != workspaceB.ID || resolved.Approvals[0].RunID != startedB.ID || resolved.Approvals[0].State != "approved" {
		t.Fatalf("resolved approvals = %+v, want only workspace B approval approved", resolved.Approvals)
	}

	ctx := context.Background()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	approvalA, err := repos.Approvals.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceA.ID)), common.ApprovalID(startedA.BlockingApprovalID))
	if err != nil {
		t.Fatalf("get workspace A approval: %v", err)
	}
	if approvalA.State != common.ApprovalStatePending {
		t.Fatalf("workspace A approval = %+v, want still pending", approvalA)
	}
	approvalB, err := repos.Approvals.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceB.ID)), common.ApprovalID(startedB.BlockingApprovalID))
	if err != nil {
		t.Fatalf("get workspace B approval: %v", err)
	}
	if approvalB.State != common.ApprovalStateApproved {
		t.Fatalf("workspace B approval = %+v, want approved", approvalB)
	}
}

func TestApprovalListAndGetEndpointsReturnPersistedApprovals(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
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

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", strings.NewReader(`{"payload":{"rawInput":"同意继续","source":"web"}}`))
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
	semantic, ok := approved.Approval.Context["governanceSemantic"].(map[string]any)
	if !ok || semantic["intent"] != "approve_execution" || semantic["source"] != "web" {
		t.Fatalf("approval governance semantic = %#v, want approve from web", approved.Approval.Context["governanceSemantic"])
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})

	workspace := createWorkspaceForHTTP(t, handler, "metrics workspace", "workspaces/metrics")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)
	runCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"run-metrics","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runCreateReq.Header.Set("Content-Type", "application/json")
	runCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(runCreateRec, runCreateReq)
	if runCreateRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201", runCreateRec.Code)
	}
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-metrics/start", nil)
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

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/workorch/workspaces", "/api/v1/workorch/workspaces/" + workspace.ID, "/api/v1/workorch/runs", "/api/v1/workorch/workspaces/" + workspace.ID + "/runs/run-metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}

	for _, path := range []string{
		"/api/v1/workorch/workspaces/" + workspace.ID + "/snapshot",
		"/api/v1/workorch/workspaces/" + workspace.ID + "/runs/run-metrics/snapshot",
		"/api/v1/workorch/workspaces/" + workspace.ID + "/runs/run-metrics/status",
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
		`workorch_event_record_metadata_scans_total`,
		`workorch_event_record_metadata_records_scanned_total`,
		`workorch_event_record_payload_hydrates_total`,
		`workorch_event_record_payload_hydrate_bytes_total`,
		`workorch_runtime_event_publish_failures_total{category="accepted"}`,
		`workorch_runtime_event_publish_failures_total{category="terminal"}`,
		`workorch_runtime_event_publish_failures_total{category="progress"}`,
		`workorch_runtime_event_publish_failures_total{category="worker_lifecycle"}`,
		`workorch_runtime_event_publish_failures_total{category="worker_lost"}`,
		`workorch_runtime_heartbeat_timer_start_failures_total`,
		`workorch_workspace_projection_rebuild_total{scope="full"}`,
		`workorch_workspace_projection_pending_rebuilds`,
		`workorch_workspace_projection_debounced_rebuild_total`,
		`workorch_workspace_projection_active_run_heartbeats`,
		`workorch_workspace_projection_active_run_heartbeat_age_seconds_max`,
		`workorch_workspace_projection_stalled_runs`,
		`workorch_workspace_projection_ineffective_retry_runs`,
		`workorch_convergence_active_slices`,
		`workorch_convergence_open_focuses`,
		`workorch_convergence_repair_attempts_total`,
		`workorch_convergence_no_delta_rounds_total`,
		`workorch_convergence_budget_escalations_total`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

func TestMetricsEndpointReportsSchedulerDiagnostics(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}), events.NewMemoryHub())
	state.schedulerDiagnostics.recordCommand(schedulerKindRunContinuation, "accepted")
	state.schedulerDiagnostics.recordCommandEnqueueFailure(schedulerKindRunContinuation, schedulerReasonEnqueueFailed)
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: "ws-metrics-diagnostics",
		RunID:       "run-metrics-diagnostics",
		Kind:        schedulerKindReplanContinuation,
		State:       schedulerDiagnosticStateFailed,
		Reason:      schedulerReasonOwnerCommandFailed,
		Detail:      "owner command failed after accepted scheduler event",
	})
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: "ws-metrics-diagnostics",
		RunID:       "run-metrics-diagnostics",
		Kind:        schedulerKindReplanContinuation,
		State:       schedulerDiagnosticStateNoActionNeeded,
		Reason:      schedulerReasonReplanSuccessorExists,
		Detail:      "successor already exists",
	})
	state.schedulerDiagnostics.recordWatchdogTimerRegistration()
	state.schedulerDiagnostics.recordWatchdogTick(time.Now().UTC(), 25*time.Millisecond, errors.New("watchdog failed"))
	state.schedulerDiagnostics.recordStaleRuntimeGapRecovery("stale_dispatched", "runtime_recovery_policy", 2)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`workorch_scheduler_commands_total{kind="run_continuation",result="accepted"} 1`,
		`workorch_scheduler_command_enqueue_failures_total{kind="run_continuation",reason="scheduler_enqueue_failed"} 1`,
		`workorch_scheduler_continuation_failures_total{kind="replan_continuation",reason="owner_command_failed"} 1`,
		`workorch_scheduler_continuation_no_action_total{kind="replan_continuation",reason="replan_successor_exists"} 1`,
		`workorch_watchdog_timer_registrations_total 1`,
		`workorch_watchdog_interval_seconds 30`,
		`workorch_watchdog_ticks_total 1`,
		`workorch_watchdog_last_error_timestamp_seconds`,
		`workorch_watchdog_stale_runtime_gap_recoveries_total{gap="stale_dispatched",safety="runtime_recovery_policy"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

func TestHTTPServerStartupLeavesFileBackedRunningRunForManualRecovery(t *testing.T) {
	dataRoot := t.TempDir()
	useWorkorchRuntimeTestConfig(t, "file", dataRoot)
	ctx := context.Background()
	first := newTestHTTPServer(t, Config{Backend: "file", DataRoot: dataRoot})
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

	routine := &recordingBackgroundRoutine{}
	second := NewHTTPServerWithEventHubAndBackground(Config{Backend: "file", DataRoot: dataRoot}, nil, routine)
	configureIsolatedTestSession(t, second.state, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := second.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown second server: %v", err)
		}
	})
	if err := second.Validate(ctx); err != nil {
		t.Fatalf("validate second server: %v", err)
	}
	getRun := func() appservice.Run {
		t.Helper()
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-stuck-validation", nil)
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
		t.Fatalf("run state after Validate = %s, want running until explicit recovery", run.State)
	}
	second.state.scheduleWatchdog()
	if routine.timerTask == nil {
		t.Fatal("startup scheduling did not register periodic watchdog timer")
	}
	if got := second.state.schedulerDiagnostics.snapshot("", "").Watchdog.TimerRegistrations; got != 1 {
		t.Fatalf("watchdog timer registrations = %d, want 1", got)
	}
	routine.timerTask.Run()

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-stuck-validation", nil)
	getRec := httptest.NewRecorder()
	second.http.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if body.Run.State != string(common.RunStateRunning) || body.Run.FailureReason != "" {
		t.Fatalf("run = %+v, want running run preserved for manual recovery", body.Run)
	}
}

func TestServiceRunRecoveredWorkspaceResumeEnqueuesWorkspaceActivation(t *testing.T) {
	dataRoot := t.TempDir()
	useWorkorchRuntimeTestConfig(t, "file", dataRoot)
	ctx := context.Background()
	first := newTestHTTPServer(t, Config{Backend: "file", DataRoot: dataRoot})
	handler := first.http.Handler
	workspace := createWorkspaceForHTTP(t, handler, "startup recovered resume workspace", "workspaces/startup-recovered-resume")
	goalID, dodID := defineGoalAndDoDForHTTP(t, handler, workspace.ID)
	runID := "run-startup-recovered-resume"
	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", strings.NewReader(`{"runId":"`+runID+`","goalId":"`+goalID+`","doDId":"`+dodID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}

	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runItem, err := repos.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get seeded run: %v", err)
	}
	runItem.State = common.RunStateRunning
	if err := repos.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("mark seeded run running: %v", err)
	}
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get seeded workspace: %v", err)
	}
	workspaceItem.State = common.WorkspaceStateRunning
	workspaceItem.ActiveRunID = common.RunID(runID)
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("mark seeded workspace running: %v", err)
	}

	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http:              NewHTTPServerWithEventHubAndBackground(Config{Addr: "127.0.0.1:0", Backend: "file", DataRoot: dataRoot}, nil, routine),
		backgroundRoutine: routine,
		done:              make(chan error, 1),
	}
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	recoveredWorkspace, err := appservice.ReadWorkspaceWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if recoveredWorkspace.State != string(common.WorkspaceStateActive) || recoveredWorkspace.ActiveRunID != "" {
		t.Fatalf("workspace after startup recovery = %+v, want active without active run", recoveredWorkspace)
	}

	activated := make(chan appservice.ActivateWorkspaceExecutionCommand, 1)
	svc.http.state.resumeWorkspaceExecution = func(runCtx context.Context, _, _, gotWorkspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if gotWorkspaceID != workspace.ID {
			t.Fatalf("activation workspaceID = %q, want %s", gotWorkspaceID, workspace.ID)
		}
		if scope := common.RuntimeWorkspaceIDFromContext(runCtx); string(scope) != workspace.ID {
			t.Fatalf("activation workspace scope = %q, want %s", scope, workspace.ID)
		}
		activated <- cmd
		return appservice.WorkspaceExecutionActivation{
			Workspace: appservice.Workspace{ID: workspace.ID, State: string(common.WorkspaceStateActive)},
			Run:       appservice.Run{ID: "run-startup-recovered-resume-new", WorkspaceID: workspace.ID, State: string(common.RunStateRunning)},
		}, nil
	}
	if err := svc.http.state.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workspace.ID, "", map[string]any{
		events.PayloadKeyWorkspaceID: workspace.ID,
		events.PayloadKeyIntent:      "resume",
		events.PayloadKeyRequestID:   "request-startup-recovered-resume",
		events.PayloadKeySource:      "tui",
		events.PayloadKeyReason:      "service_recovery_resume",
		events.PayloadKeyGoalID:      goalID,
		events.PayloadKeyDoDID:       dodID,
	}); err != nil {
		t.Fatalf("publish resume command: %v", err)
	}
	if routine.function == nil {
		t.Fatal("resume command did not enqueue workspace activation")
	}
	routine.function()
	select {
	case cmd := <-activated:
		if cmd.Restart {
			t.Fatalf("activation command Restart = true, want recovered resume to activate without restart")
		}
		if cmd.GoalID != goalID || cmd.DoDID != dodID {
			t.Fatalf("activation command definitions = %+v, want goal %s dod %s", cmd, goalID, dodID)
		}
	case <-time.After(time.Second):
		t.Fatal("resume command did not execute workspace activation task")
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

func getAndExpect(t *testing.T, handler http.Handler, path string, wantStatus int) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d, body = %s", path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec.Body.String()
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
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
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

func seedBlockedDecisionRunWithTicketForHTTP(t *testing.T, dataRoot string, title string, runID string, ticketID string) (appservice.Workspace, appservice.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedDecisionRunWithTicketInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID, ticketID)
	return workspace, run
}

func seedBlockedDecisionRunWithTicketInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string, ticketID string) appservice.Run {
	t.Helper()
	stepID := common.StepID(runID + "/decision")
	ctx := context.Background()
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	ticketRepo := runScopedRepos.Tickets
	item, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	item.State = common.RunStateBlockedOnDecision
	item.BlockingTicketID = common.TicketID(ticketID)
	item.BlockingApprovalID = ""
	if err := runRepo.Update(ctx, item); err != nil {
		t.Fatalf("update decision run: %v", err)
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
		ID:           common.TicketID(ticketID),
		WorkspaceID:  common.WorkspaceID(workspaceID),
		RunID:        common.RunID(runID),
		StepID:       stepID,
		Reason:       "test long decision gate",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "provide test input",
		State:        common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create decision ticket: %v", err)
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
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
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

func seedBlockedApprovalRunWithApprovalForHTTP(t *testing.T, dataRoot string, title string, runID string, approvalID string) (appservice.Workspace, appservice.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedApprovalRunWithApprovalInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID, approvalID)
	return workspace, run
}

func seedBlockedApprovalRunWithApprovalInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string, approvalID string) appservice.Run {
	t.Helper()
	stepID := common.StepID(runID + "/approval")
	ctx := context.Background()
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	approvalRepo := runScopedRepos.Approvals
	item, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	item.State = common.RunStateBlockedOnApproval
	item.BlockingApprovalID = common.ApprovalID(approvalID)
	item.BlockingTicketID = ""
	if err := runRepo.Update(ctx, item); err != nil {
		t.Fatalf("update approval run: %v", err)
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
		ID:          common.ApprovalID(approvalID),
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       common.RunID(runID),
		StepID:      stepID,
		Reason:      "test shared approval gate",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
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
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
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

func startRunForHTTP(t *testing.T, handler http.Handler, workspaceID string, runID string) appservice.Run {
	t.Helper()

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+runID+"/start", nil)
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

func resolutionFilesForHTTP(t *testing.T, dataRoot string) []string {
	t.Helper()
	var ret []string
	if err := filepath.WalkDir(dataRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/decisions/resolutions/") {
			ret = append(ret, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk resolution files: %v", err)
	}
	return ret
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
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	if err := publishHTTPTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        decisionevents.TopicDecisionCommandEnsureResolved,
		SourceModule: "workorchd/server/http-test",
		SubjectType:  "decision_ticket",
		SubjectID:    ticketID,
		Payload: map[string]any{
			events.PayloadKeyResolutionID:     resolutionID,
			events.PayloadKeyBlockingTicketID: ticketID,
			events.PayloadKeyType:             resolutionType,
		},
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

	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	topic := approvalevents.TopicApprovalCommandEnsureApproved
	if reject {
		topic = approvalevents.TopicApprovalCommandEnsureRejected
	}
	if err := publishHTTPTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        topic,
		SourceModule: "workorchd/server/http-test",
		SubjectType:  "approval",
		SubjectID:    approvalID,
		Payload: map[string]any{
			events.PayloadKeyApprovalID: approvalID,
		},
	}); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
}

func TestRunStartEndpointWaitsWhenConcurrentRunLimitReached(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newTestServerState(t, normalizeConfig(Config{
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
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/workspaces/ws-run-held/runs/run-held/start", nil)
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
		secondResp, err := http.Post(server.URL+"/api/v1/workorch/workspaces/ws-run-second/runs/run-second/start", "application/json", nil)
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

	metrics := state.metrics.render(state.config, state.workspaceProjection)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newTestServerState(t, normalizeConfig(Config{
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
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/workspaces/ws-run-1/runs/run-1/start", nil)
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
		resp, err := http.Post(server.URL+"/api/v1/workorch/workspaces/ws-run-2/runs/run-2/start", "application/json", nil)
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

	metrics := state.metrics.render(state.config, state.workspaceProjection)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	heldStarted := make(chan struct{}, 1)
	highStarted := make(chan struct{}, 1)
	lowStarted := make(chan struct{}, 1)
	releaseHeld := make(chan struct{})
	releaseHigh := make(chan struct{})
	state := newTestServerState(t, normalizeConfig(Config{
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

	go postStart("/api/v1/workorch/workspaces/ws-run-held/runs/run-held/start")
	<-heldStarted
	go postStart("/api/v1/workorch/workspaces/ws-run-low/runs/run-low/start?priority=1")
	go postStart("/api/v1/workorch/workspaces/ws-run-high/runs/run-high/start?priority=10")
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	state := newTestServerState(t, normalizeConfig(Config{
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
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-held/runs/run-held/resume", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-queued/runs/run-queued/resume", nil).WithContext(ctx)
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

	metrics := state.metrics.render(state.config, state.workspaceProjection)
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

	_, err = ConfigFromValues(map[string]string{
		configServerStaleDispatchedStepMinAge: "also-not-a-duration",
	})
	if err == nil {
		t.Fatal("expected invalid stale dispatched min age duration error")
	}
	if !strings.Contains(err.Error(), configServerStaleDispatchedStepMinAge) {
		t.Fatalf("error = %v, want config key %s", err, configServerStaleDispatchedStepMinAge)
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
