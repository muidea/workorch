package server

import (
	"bufio"
	"context"
	"encoding/base64"
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

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	magictask "github.com/muidea/magicCommon/task"
	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	appserviceruntimecore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
	"github.com/muidea/workorch/web/consoleassets"
)

func TestWorkspaceLLMSelectionRoutesValidateScopeAndPersistCompleteTarget(t *testing.T) {
	hub := magicevent.NewHub(32)
	routine := magictask.NewBackgroundRoutine(16)
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), hub, routine)
	t.Cleanup(func() {
		// Parallel full-suite load can slow hub/background drain; keep a bounded
		// but non-trivial timeout so teardown does not hang package timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = state.close(ctx)
		_ = routine.Shutdown(ctx)
	})
	llmObserver := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	llmObserver.Subscribe(llmevents.TopicLLMCommandValidateInferenceSelection, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.ValidateProviderInferenceSelectionCommand)
		result.Set(llmevents.ValidateProviderInferenceSelectionResult{Selection: command.Selection, CapabilityRevision: "capability-1"}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicLLMCommandValidateInferenceSelection) })
	var ownerCommands []workspaceevents.UpdateWorkspaceLLMSelectionCommand
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workspaceevents.UpdateWorkspaceLLMSelectionCommand)
		ownerCommands = append(ownerCommands, command)
		result.Set(workspaceevents.WorkspaceLLMSelectionResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, Revision: command.ExpectedRevision + 1, State: common.WorkspaceStateActive, DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(command.Selection)}, Changed: true}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection) })

	mux := newMuxWithState(state)
	put := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/ws-model/llm-selection", strings.NewReader(`{"providerId":"ai-proxy","modelId":"model-a","reasoningEffort":"none","expectedRevision":1}`))
	put.Header.Set("Content-Type", "application/json")
	putResponse := httptest.NewRecorder()
	mux.ServeHTTP(putResponse, put)
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", putResponse.Code, putResponse.Body.String())
	}
	var updated workspaceResponse
	if err := json.NewDecoder(putResponse.Body).Decode(&updated); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if updated.Workspace.ID != "ws-model" || updated.Workspace.DefaultLLMSelection == nil || updated.Workspace.DefaultLLMSelection.Target.ProviderID != "ai-proxy" || updated.Workspace.DefaultLLMSelection.Target.ModelID != "model-a" || updated.Workspace.DefaultLLMSelection.ReasoningEffort != "none" {
		t.Fatalf("PUT workspace = %+v", updated.Workspace)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/workorch/workspaces/ws-model/llm-selection", strings.NewReader(`{"expectedRevision":2}`))
	deleteRequest.Header.Set("Content-Type", "application/json")
	deleteResponse := httptest.NewRecorder()
	mux.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if len(ownerCommands) != 2 || ownerCommands[0].WorkspaceID != "ws-model" || ownerCommands[0].Selection == nil || ownerCommands[1].WorkspaceID != "ws-model" || ownerCommands[1].Selection != nil {
		t.Fatalf("owner commands = %+v", ownerCommands)
	}
	if ownerCommands[0].ExpectedRevision != 1 || ownerCommands[0].Selection.ReasoningEffort != "none" || ownerCommands[1].ExpectedRevision != 2 {
		t.Fatalf("owner command CAS/selection = %+v", ownerCommands)
	}

	invalid := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/workspaces/ws-model/llm-selection", strings.NewReader(`{"providerId":"ai-proxy"}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalidResponse := httptest.NewRecorder()
	mux.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
}

func testSessionExports(session *runtimecore.Session) (testkit.Exports, error) {
	return testkit.SessionExports(session)
}

func mustServerGovernancePayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("governance payload: %v", err)
	}
	return payload
}

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

func waitForReconciledWorkspaceProjection(t *testing.T, state *serverState) {
	t.Helper()
	waitForCondition(t, 2*time.Second, func() bool {
		status, err := state.reconciledWorkspaceProjectionStatus(context.Background())
		return err == nil && status.State == workspaceProjectionStateReady
	})
}

type recordingEventHub struct {
	magicevent.Hub
	onSend func(magicevent.Event)
}

func (h recordingEventHub) Send(event magicevent.Event) magicevent.Result {
	if h.onSend != nil {
		h.onSend(event)
	}
	return h.Hub.Send(event)
}

func newTestServerState(t *testing.T, config Config) *serverState {
	t.Helper()
	routine := magictask.NewBackgroundRoutine(128)
	state := newIsolatedTestServerStateWithBackground(config, routine)
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
	if state == nil {
		return
	}
	existingCommandHub := testStateCommandHub(state)
	if session := clearTestRuntimeSession(state); session != nil {
		_ = session.Close(context.Background())
	}
	runtime, err := testkit.TryNewRuntime(
		config.Backend,
		config.DataRoot,
		existingCommandHub)

	if err != nil {
		return
	}
	session := appservice.NewRuntimeSession(runtime)
	session.SetRepositoryConfig(config.Backend, config.DataRoot)
	configureSessionBackedTestOperations(state, session)
	attachTestEventNotifications(state)
}

// directWorkspaceExecutionSubmitter is a test-only adapter. Production HTTP code
// receives the application Biz port instead of directly using EventHub.
type directWorkspaceExecutionSubmitter struct{ state *serverState }

func (p directWorkspaceExecutionSubmitter) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	session := testRuntimeSession(p.state)
	if session == nil || command.WorkspaceID == "" || command.RequestID == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	// This test adapter follows the production boundary exactly: compile an
	// immutable restart bootstrap if needed, then send one Workspace-owner
	// intent. It must not activate, continue, supersede, or select a Run.
	intent := workspaceevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: command.WorkspaceID,
		RequestID:   command.RequestID,
	}
	switch command.Intent {
	case "resume":
		intent.Kind = runtimecontract.RuntimeExecutionIntentResume
	case "restart":
		intent.Kind = runtimecontract.RuntimeExecutionIntentRestart
		bootstrap, err := appservice.CompileWorkspaceExecutionRestartBootstrapWithSession(ctx, session.Borrow(), string(command.WorkspaceID))
		if err != nil {
			return workorchdevents.WorkspaceRunRequestedResult{}, err
		}
		intent.Bootstrap = bootstrap
	default:
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, intent)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(command.WorkspaceID), WorkspaceID: string(command.WorkspaceID)}))
	response := hub.Send(event)
	if response == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, resultErr
	}
	ownerResult, ok := data.(workspaceevents.WorkspaceExecutionIntentResult)
	if !ok {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	return workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: ownerResult.WorkspaceID,
		RunID:       ownerResult.RunID,
		RequestID:   ownerResult.RequestID,
		Kind:        ownerResult.Kind,
		Outcome:     ownerResult.Outcome,
		Reason:      ownerResult.Reason,
	}, nil
}

type blockingWorkspaceExecutionSubmitter struct {
	state   *serverState
	started chan struct{}
	release chan struct{}
}

type failingWorkspaceExecutionSubmitter struct{ err error }

func (p failingWorkspaceExecutionSubmitter) SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	return workorchdevents.WorkspaceRunRequestedResult{}, p.err
}

// acceptedRestartWorkspaceExecutionSubmitter isolates HTTP projection tests from
// restart bootstrap and provider-catalog work. The HTTP adapter only consumes
// the Workspace owner's typed admission fact.
type acceptedRestartWorkspaceExecutionSubmitter struct{}

func (acceptedRestartWorkspaceExecutionSubmitter) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if command.Intent != string(runtimecontract.RuntimeExecutionIntentRestart) {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	return workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-http-restart",
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentRestart,
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}, nil
}

func (p *blockingWorkspaceExecutionSubmitter) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	close(p.started)
	<-p.release
	return workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: command.WorkspaceID, RequestID: command.RequestID, Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil
}

func newOwnerRuntimeSession(t *testing.T, backend string, dataRoot string) *runtimecore.Session {
	t.Helper()
	runtime, err := testkit.TryNewRuntime(backend, dataRoot, nil)
	if err != nil {
		t.Fatalf("create owner-aware runtime: %v", err)
	}
	session := appservice.NewRuntimeSession(runtime)
	session.SetRepositoryConfig(backend, dataRoot)
	return session
}

func newTestMux(t *testing.T, config Config) http.Handler {
	t.Helper()
	return newMuxWithState(newTestServerState(t, normalizeConfig(config)))
}

func newTestHTTPServer(t *testing.T, config Config) *HTTPServer {
	t.Helper()
	routine := magictask.NewBackgroundRoutine(128)
	server := newIsolatedTestHTTPServerWithBackground(config, routine)
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

func publishHTTPTestWorkspaceRunCommand(ctx context.Context, hub magicevent.Hub, command workorchdevents.WorkspaceRunRequestedCommand) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "workorchd/test", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(event)
	if result == nil {
		return common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return err
	}
	data, err := result.Get()
	if err != nil {
		return err
	}
	accepted, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || accepted.WorkspaceID != command.WorkspaceID || accepted.RequestID != command.RequestID {
		return common.ErrConflict
	}
	return nil
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

func readProjectionSSE(t *testing.T, lines <-chan string, readErr <-chan error) (string, workspaceProjectionChangedSSE) {
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
				var dto workspaceProjectionChangedSSE
				if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &dto); err != nil {
					t.Fatalf("decode projection sse data %q: %v", line, err)
				}
				if strings.TrimSpace(dto.WorkspaceID) == "" {
					// Skip non-projection frames if any.
					continue
				}
				if id == "" {
					id = dto.ID
				}
				return id, dto
			}
		case err := <-readErr:
			t.Fatalf("stream closed before projection event: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for projection sse event")
		}
	}
}

func readSSEEnvelope(t *testing.T, lines <-chan string, readErr <-chan error) (string, streamEvent) {
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
				var envelope streamEvent
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

func waitWorkspaceCreatedSSE(t *testing.T, lines <-chan string, readErr <-chan error, title string) streamEvent {
	t.Helper()
	_ = title // Workspace changes now publish the concrete projection DTO, not raw owner payloads.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case line := <-lines:
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event struct {
				WorkspaceID string `json:"workspaceId"`
				ID          string `json:"id"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				t.Fatalf("decode workspace stream data %q: %v", line, err)
			}
			if event.WorkspaceID != "" {
				return streamEvent{ID: event.ID, WorkspaceID: event.WorkspaceID}
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
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
	dependencyClosedWhileHandling := make(chan bool, 1)
	state := newServerStateBase(Config{}, nil, applicationOperations{})
	srv := newHTTPServerWithHandler(Config{}, state, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		select {
		case <-state.backgroundCtx.Done():
			dependencyClosedWhileHandling <- true
		default:
			dependencyClosedWhileHandling <- false
		}
		_, _ = w.Write([]byte("ok"))
	}))
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
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.Shutdown(ctx) }()
	select {
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("http server did not stop after forced close")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown context did not reach its forced-close deadline")
	}
	// Context expiry and handler release may otherwise race inside
	// net/http.Shutdown: wait until the forced Close has reached the client
	// before allowing the handler to return. This proves that Drain observed
	// the deadline instead of accepting either side of that race.
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("client connection was not force-closed after the shutdown deadline")
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before the force-closed handler exited: %v", err)
	default:
	}
	close(releaseHandler)
	select {
	case closed := <-dependencyClosedWhileHandling:
		if closed {
			t.Fatal("handler dependency was closed before the in-flight handler returned")
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight handler did not return after release")
	}
	select {
	case err := <-shutdownDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after the in-flight handler returned")
	}
	select {
	case <-state.backgroundCtx.Done():
	default:
		t.Fatal("handler dependencies were not closed after transport drain")
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

func TestReadyzNativeRouteDoesNotExposeEventHub(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.validate = func(ctx context.Context, _, _ string) error {
		if got := appservice.CommandHubFromContext(ctx); got != nil {
			t.Fatalf("event hub from context = %#v, want no protocol-layer hub exposure", got)
		}
		return nil
	}
	handler := newMuxWithState(state)
	waitForReconciledWorkspaceProjection(t, state)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadyzDoesNotExposeExecutionAdmissionState(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.metrics.executionAdmissionMetrics.recordFailure(executionAdmissionKindWorkspaceRun, executionAdmissionReasonSubmissionFailed)
	handler := newMuxWithState(state)
	waitForReconciledWorkspaceProjection(t, state)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readyz response: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("readyz status = %q, want ready", body["status"])
	}
	if _, found := body["diagnostics"]; found {
		t.Fatalf("readyz exposed process-local execution admission state: %+v", body)
	}
}

func TestServerStateUsesRuntimeSession(t *testing.T) {
	useWorkorchRuntimeTestConfig(t, "memory", "")
	bg := magictask.NewBackgroundRoutine(128)
	ctx := context.Background()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = bg.Shutdown(shutdownCtx)
	})

	state := newIsolatedTestServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), bg)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.close(shutdownCtx)
	})
	if testStateCommandHub(state) == nil {
		t.Fatal("test runtime session infrastructure is unavailable")
	}
	if testRuntimeSession(state) == nil {
		t.Fatal("server state runtime session = nil")
	}
	if err := state.validate(ctx, "", ""); err != nil {
		t.Fatalf("validate runtime session: %v", err)
	}
}

func TestConfigurationEndpointReturnsRedactedAIProxyConfiguration(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	fixture := workorchconfigurationcontract.ServiceConfiguration{
		Server:     workorchconfig.ServerConfig{Addr: ":8080"},
		Repository: workorchconfig.RepositoryConfig{Backend: "file", DataDir: ".workorch-data"},
		Runtime:    workorchconfigurationcontract.RuntimeConfiguration{DefaultSoulID: "soul-system"},
		LLM: workorchconfigurationcontract.LLMConfiguration{
			AIProxy: workorchconfigurationcontract.AIProxyConfigurationView{
				BaseURL:          "http://127.0.0.1:8080",
				APIKeyConfigured: true,
				Timeout:          "180s",
			},
			DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			Budget: workorchconfig.LLMContextBudgetPolicyConfig{
				ContextUtilizationRatio: 0.75,
				EstimatedTokensPerByte:  0.25,
			},
		},
	}
	state.readConfiguration = func() (workorchconfigurationcontract.ServiceConfiguration, error) {
		return fixture, nil
	}
	state.readConfigurationWithCatalog = func(ctx context.Context) (workorchconfigurationcontract.ServiceConfiguration, error) {
		_ = ctx
		return fixture, nil
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
	if body.Configuration.LLM.DefaultChat.ModelID != "gpt-4o" ||
		body.Configuration.Runtime.DefaultSoulID != "soul-system" ||
		!body.Configuration.LLM.AIProxy.APIKeyConfigured ||
		strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("body = %s, want redacted ai-proxy configuration response", rec.Body.String())
	}
}

func TestUpdateLLMConfigurationEndpointReturnsRedactedConfiguration(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.updateLLMConfiguration = func(cmd configcore.UpdateLLMConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
		if cmd.APIKey == nil || *cmd.APIKey != "rotated-key" || cmd.DefaultChatModelID == nil || *cmd.DefaultChatModelID != "gpt-4.1" {
			t.Fatalf("update command = %+v, want narrow llm update command", cmd)
		}
		return workorchconfigurationcontract.ServiceConfiguration{
			LLM: workorchconfigurationcontract.LLMConfiguration{
				AIProxy: workorchconfigurationcontract.AIProxyConfigurationView{
					BaseURL:          "http://127.0.0.1:9090",
					APIKeyConfigured: true,
					Timeout:          "180s",
				},
				DefaultChat:     runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4.1"},
				RestartRequired: true,
			},
		}, nil
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/configuration/llm", strings.NewReader(`{
		"apiKey":"rotated-key",
		"defaultChatModelId":"gpt-4.1"
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
	if strings.Contains(rec.Body.String(), "secret-value") || !body.Configuration.LLM.AIProxy.APIKeyConfigured || !body.Configuration.LLM.RestartRequired {
		t.Fatalf("body = %s, want redacted restart-pending configuration", rec.Body.String())
	}
}

func TestUpdateLLMConfigurationEndpointRejectsUnavailableFields(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	for _, requestBody := range []string{
		`{"aiProxy":{"baseURL":"http://127.0.0.1:9090"}}`,
		`{"clearApiKey":true}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/workorch/configuration/llm", strings.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		newMuxWithState(state).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s, want rejected unsupported LLM configuration field", requestBody, rec.Code, rec.Body.String())
		}
	}
}

func TestUpdateRuntimeConfigurationEndpointPersistsCanonicalPolicySource(t *testing.T) {
	useWorkorchRuntimeTestConfig(t, "file", t.TempDir())
	rawHub := recordingEventHub{Hub: magicevent.NewHub(128)}
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "file", DataRoot: t.TempDir()}), rawHub, magictask.NewBackgroundRoutine(128))
	configureIsolatedTestSession(t, state, state.config)

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
	gotPolicy, err := workorchruntimepolicy.Current()
	if err != nil {
		t.Fatalf("load canonical runtime policy: %v", err)
	}
	if body.Configuration.Runtime.MaxRuntimeRepairAttempts != 3 ||
		body.Configuration.Runtime.ShellCommandTimeout != "30s" ||
		gotPolicy.MaxRuntimeRepairAttempts != 3 {
		t.Fatalf("configuration=%+v policy=%+v, want runtime repair attempts synced to 3", body.Configuration.Runtime, gotPolicy)
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
			"name":"invalid plaintext secret",
			"transport":"stdio",
			"command":"context-helper",
			"environmentRefs":{"TOKEN":"secret-value"},
			"enabled":true
		}`, http.StatusBadRequest)
	postAndExpect(t, mux, "/api/v1/workorch/mcp/servers", `{
			"id":"mcp-local-context",
			"name":"local context",
			"transport":"stdio",
			"command":"context-helper",
			"environmentRefs":{"TOKEN":"env://MCP_HTTP_TOKEN"},
			"enabled":true,
			"allowedTools":["repo.summary","repo.details"],
			"allowedResources":["file://README.md"],
			"allowedPrompts":["delivery.plan"]
		}`, http.StatusCreated)
	putAndExpect(t, mux, "/api/v1/workorch/mcp/servers/mcp-local-context", `{
			"name":"local context updated",
			"transport":"stdio",
			"endpoint":".workorch-data/system/mcp-endpoints/mcp-local-context/context-helper.py",
			"enabled":true,
			"allowedTools":["repo.summary","repo.details"],
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
		{path: "/api/v1/workorch/souls/soul-delete-http", want: `message:not found`},
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
		if strings.Contains(spec.want, "not found") {
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
	serverReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/mcp/servers/mcp-local-context", nil)
	serverRec := httptest.NewRecorder()
	mux.ServeHTTP(serverRec, serverReq)
	if serverRec.Code != http.StatusOK || !strings.Contains(serverRec.Body.String(), `"TOKEN": "env://MCP_HTTP_TOKEN"`) || strings.Contains(serverRec.Body.String(), "secret-value") {
		t.Fatalf("MCP environment reference response = %d/%s", serverRec.Code, serverRec.Body.String())
	}
}

func TestWorkspaceScopedMemoryItemRoutesRejectUnscopedItemPaths(t *testing.T) {
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

	getAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID, http.StatusNotFound)
	getAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID+"?workspaceId="+workspaceB.ID, http.StatusNotFound)
	putAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID+"?workspaceId="+workspaceB.ID, `{}`, http.StatusNotFound)
	deleteAndExpect(t, handler, "/api/v1/workorch/memory/"+memoryID+"?workspaceId="+workspaceB.ID, http.StatusNotFound)

	deleteAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, http.StatusOK)
	getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceA.ID+"/memory/"+memoryID, http.StatusNotFound)
	getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/memory/"+memoryID, http.StatusOK)
}

func useUnavailableLLMProvider(t *testing.T) {
	t.Helper()
	// Keep HTTP tests offline: point ai-proxy at an unreachable local endpoint and use no live catalog.
	// Activation/config paths now read workorchconfig.LLMConfig rather than provider env vars.
	configDir := t.TempDir()
	content := `endpointName = "workorch"

[repository]
backend = "memory"

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "1s"
catalogRequestTimeout = "1s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.aiProxy.modelOverrides.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["responses"]
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current offline llm config: %v", err)
	}
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

[repository]
backend = %q
dataDir = %q


[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "60s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.aiProxy.modelOverrides.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["responses"]

`, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write runtime test config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

func TestAcceptedAPIRoutesAreRegisteredNatively(t *testing.T) {
	router := engine.NewRouteRegistry()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

func TestUnsupportedMutatingAPIRoutesAreNotRegisteredNatively(t *testing.T) {
	router := engine.NewRouteRegistry()
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	registerSystemRoutes(router, state)
	registerConfigurationRoutes(router, state)
	registerReadRoutes(router, state)
	registerBusinessWriteRoutes(router, state)
	registerConsoleRoutes(router)

	for _, route := range unsupportedMutatingAPINativeRouteSpecs() {
		if router.ExistHandler(route.pattern, route.method) {
			t.Fatalf("unsupported native mutating route %s %s must not be registered", route.method, route.pattern)
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
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspace-authoring"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/authoring"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/authoring"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/authoring/compile"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/authoring/confirm"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodDelete, pattern: "/api/v1/workorch/workspaces/:workspaceId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/snapshot"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/memory-recalls"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/soul-bindings"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals/:goalId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods/:dodId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-activations"},
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

func unsupportedMutatingAPINativeRouteSpecs() []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/goals/:goalId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/dods/:dodId"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-restarts"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/template-binding-refresh"},
		{method: http.MethodPut, pattern: "/api/v1/workorch/workspaces/:workspaceId/execution-contract"},
	}
}

func TestMemoryRecallTraceEndpointsReturnFilteredTraces(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := state.close(ctx); err != nil {
			t.Fatalf("close server state: %v", err)
		}
	})
	state.listMemoryRecallTraces = func(_ context.Context, _, _ string, query workorchquerycontract.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
		if query.RunID != "run-recall-http" && query.WorkspaceID != "ws-recall-http" {
			t.Fatalf("memory recall query = %+v, want run or workspace filter", query)
		}
		return []projectioncontract.MemoryRecallTrace{
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-other",
		Payload:     recordpayload.WorkspacePayloadData{Title: "other", DeliverableDirectory: "/tmp/other", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish nonmatching event: %v", err)
	}
	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-stream",
		Payload:     recordpayload.WorkspacePayloadData{Title: "stream workspace", DeliverableDirectory: "/tmp/stream", State: common.WorkspaceStateActive}.EventPayload().Map(),
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
			var envelope streamEvent
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       "/step/state/changed",
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
			// Baseline projection notifications may arrive; raw step payload must never appear.
			if strings.Contains(line, "large runtime payload") || strings.Contains(line, "/step/state/changed") {
				t.Fatalf("projection stream emitted raw step data line: %.120s", line)
			}
		}
	case err := <-readErr:
		t.Fatalf("stream closed before projection event: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	// Explicit typed projection notification must serialize as concrete top-level DTO.
	testStateCommandHub(state).Post(magicevent.NewEventWithContext(
		workorchdevents.TopicWorkspaceProjectionChanged,
		"workorchd/projection",
		workorchdevents.NotificationDestination,
		magicevent.NewHeader(),
		context.Background(),
		workorchdevents.WorkspaceProjectionChangedData{
			StreamGeneration:     "server-default-projection",
			WorkspaceID:          "ws-projection",
			RunID:                "run-1",
			GlobalRevision:       42,
			WorkspaceRevision:    7,
			RuntimeStateRevision: 11,
			EvidenceSequence:     3,
			ActivityRevision:     9,
			ChangedSections: []projectioncontract.WorkspaceProjectionSection{
				projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
				projectioncontract.WorkspaceProjectionSectionActivity,
			},
			ProjectionState: "ready",
			UpdatedAt:       "2026-07-15T15:00:00Z",
		},
	))
	// Allow async delivery into the live stream subscription.
	time.Sleep(30 * time.Millisecond)

	id, dto := readProjectionSSE(t, lines, readErr)
	if id != "projection:server-default-projection:ws-projection:42" {
		t.Fatalf("sse id = %q, want current generation-scoped projection id", id)
	}
	if dto.ID != id || dto.WorkspaceID != "ws-projection" || dto.RunID != "run-1" {
		t.Fatalf("dto = %+v, want concrete projection identity", dto)
	}
	if dto.StreamRevision != 42 || dto.Revision.StreamRevision != 42 ||
		dto.Revision.WorkspaceRevision != 7 || dto.Revision.RuntimeStateRevision != 11 ||
		dto.Revision.EvidenceSequence != 3 || dto.Revision.ActivityRevision != 9 {
		t.Fatalf("dto revisions = %+v, want multi-domain revisions", dto)
	}
	if len(dto.ChangedSections) != 2 {
		t.Fatalf("changedSections = %v, want runtime_progress+activity", dto.ChangedSections)
	}
	if dto.ProjectionState != "ready" {
		t.Fatalf("projectionState = %q, want ready", dto.ProjectionState)
	}
}

func TestEventStreamWritesIDRevisionAndStateSummary(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-summary",
		Payload:     recordpayload.WorkspacePayloadData{Title: "summary", DeliverableDirectory: "/tmp/summary", State: common.WorkspaceStateActive, ActiveRunID: "run-001"}.EventPayload().Map(),
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
	if summary["workspaceState"] != string(common.WorkspaceStateActive) || summary["activeRunId"] != "run-001" || numericTestValue(summary["revision"]) != 1 {
		t.Fatalf("stateSummary = %+v, want workspace state, active run and revision", summary)
	}
}

func TestEventStreamReplaysAfterLastEventID(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

	postServerTestWorkspaceState(context.Background(), testStateCommandHub(state), "ws-stream-replay")
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	postServerTestRunState(context.Background(), testStateCommandHub(state), "ws-stream-replay", "run-replay")
	if _, appended := readSSEEnvelope(t, lines, readErr); appended.Topic != runevents.TopicRunStateChanged || appended.RunID != "run-replay" {
		t.Fatalf("appended replay event = %+v", appended)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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

	postServerTestWorkspaceState(context.Background(), testStateCommandHub(state), "ws-stream-query-replay")
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	postServerTestRunState(context.Background(), testStateCommandHub(state), "ws-stream-query-replay", "run-query-replay")
	if _, appended := readSSEEnvelope(t, lines, readErr); appended.Topic != runevents.TopicRunStateChanged || appended.RunID != "run-query-replay" {
		t.Fatalf("appended query replay event = %+v", appended)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.listEventRecords = func(_ context.Context, _, _ string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
		if query.WorkspaceID != "ws-persist" || query.RunID != "run-persist" || query.CursorAfterID != "evt-old" {
			t.Fatalf("query = %+v, want workspace/run cursor replay query", query)
		}
		if !query.IncludePayload {
			t.Fatalf("query IncludePayload = false, want true for full stream replay")
		}
		return []projectioncontract.EventRecord{{
			ID:          "evt-persisted-step",
			Version:     "workorch.event",
			Topic:       "/step/state/changed",
			WorkspaceID: "ws-persist",
			RunID:       "run-persist",
			StepID:      "run-persist/step-1",
			Revision:    12,
			Payload: mustServerEventPayload(t, map[string]any{
				recordpayload.PayloadKeyState: string(common.StepStateSucceeded),
			}),
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
	if id != "evt-persisted-step" || replayed.Topic != "/step/state/changed" || replayed.StepID != "run-persist/step-1" {
		t.Fatalf("replayed id=%q envelope=%+v, want persisted step state event", id, replayed)
	}
	if replayed.Payload[recordpayload.PayloadKeyState] != string(common.StepStateSucceeded) {
		t.Fatalf("payload = %+v, want completed state", replayed.Payload)
	}
}

func TestEventStreamProjectionReplayUsesConcreteDTOFromInMemoryBuffer(t *testing.T) {
	// Projection live and reconnect replay must share the concrete DTO serializer.
	// Projection notifications are not owner EventRecords; persisted event-record
	// envelopes must not be reconstructed as projection wire frames.
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	listCalled := false
	state.listEventRecords = func(_ context.Context, _, _ string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
		listCalled = true
		t.Fatalf("projection reconnect must not query event records; query = %+v", query)
		return nil, nil
	}
	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-projection-replay", nil)
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

	publishProjection := func(rev int64, activity int64) {
		t.Helper()
		testStateCommandHub(state).Post(magicevent.NewEventWithContext(
			workorchdevents.TopicWorkspaceProjectionChanged,
			"workorchd/projection",
			workorchdevents.NotificationDestination,
			magicevent.NewHeader(),
			context.Background(),
			workorchdevents.WorkspaceProjectionChangedData{
				StreamGeneration:     "server-projection-replay",
				WorkspaceID:          "ws-projection-replay",
				RunID:                "run-replay",
				GlobalRevision:       rev,
				WorkspaceRevision:    5,
				RuntimeStateRevision: 30,
				EvidenceSequence:     4,
				ActivityRevision:     activity,
				ChangedSections: []projectioncontract.WorkspaceProjectionSection{
					projectioncontract.WorkspaceProjectionSectionActivity,
				},
				ProjectionState: "ready",
				UpdatedAt:       "2026-07-15T15:01:00Z",
			},
		))
		time.Sleep(30 * time.Millisecond)
	}

	publishProjection(20, 7)
	firstID, first := readProjectionSSE(t, lines, readErr)
	if firstID != "projection:server-projection-replay:ws-projection-replay:20" || first.StreamRevision != 20 {
		t.Fatalf("first = %q %+v, want rev 20", firstID, first)
	}

	publishProjection(21, 8)
	// Close first connection after second notification is recorded in-memory.
	time.Sleep(30 * time.Millisecond)
	_ = resp.Body.Close()

	replayReq, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/workorch/events/stream?workspaceId=ws-projection-replay", nil)
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

	id, dto := readProjectionSSE(t, replayLines, replayErr)
	if id != "projection:server-projection-replay:ws-projection-replay:21" || dto.ID != id {
		t.Fatalf("replayed id=%q dto=%+v, want concrete projection id rev 21", id, dto)
	}
	if dto.WorkspaceID != "ws-projection-replay" || dto.RunID != "run-replay" {
		t.Fatalf("dto identity = %+v", dto)
	}
	if dto.StreamRevision != 21 || dto.Revision.RuntimeStateRevision != 30 || dto.Revision.ActivityRevision != 8 {
		t.Fatalf("dto revisions = %+v, want multi-domain revisions", dto)
	}
	if len(dto.ChangedSections) != 1 || dto.ChangedSections[0] != string(projectioncontract.WorkspaceProjectionSectionActivity) {
		t.Fatalf("changedSections = %v, want activity", dto.ChangedSections)
	}
	if listCalled {
		t.Fatal("projection reconnect queried event records")
	}
}

func TestWorkspaceCreateEndpointStreamsWorkspaceCreatedEvent(t *testing.T) {
	rawHub := magicevent.NewHub(1024)
	defer rawHub.Terminate(context.Background())
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, magictask.NewBackgroundRoutine(128))
	configureIsolatedTestSession(t, state, state.config)
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

	postAndExpect(t, newMuxWithState(state), "/api/v1/workorch/workspace-templates", `{"id":"stream-template","name":"流式创建模板"}`, http.StatusCreated)
	createResp, err := http.Post(server.URL+"/api/v1/workorch/workspace-authoring", "application/json", strings.NewReader(`{"requestId":"stream-created","templateId":"stream-template","input":{"goalText":"验证创建事件"},"title":"streamed workspace","deliverableDirectory":"workspaces/streamed"}`))
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
	rawHub := magicevent.NewHub(1024)
	defer rawHub.Terminate(context.Background())
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, magictask.NewBackgroundRoutine(128))
	configureIsolatedTestSession(t, state, state.config)
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

	postAndExpect(t, newMuxWithState(state), "/api/v1/workorch/workspace-templates", `{"id":"stream-multi-template","name":"多客户端创建模板"}`, http.StatusCreated)
	createResp, err := http.Post(server.URL+"/api/v1/workorch/workspace-authoring", "application/json", strings.NewReader(`{"requestId":"stream-multi-created","templateId":"stream-multi-template","input":{"goalText":"验证多客户端创建事件"},"title":"streamed multi-client workspace","deliverableDirectory":"workspaces/streamed-multi"}`))
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
				var event struct {
					WorkspaceID string `json:"workspaceId"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
					t.Fatalf("%s decode workspace stream data %q: %v", name, line, err)
				}
				if event.WorkspaceID != "" {
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
	ch := make(chan streamEvent, 1)
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: workspaceevents.TopicWorkspaceCreated, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != workspaceevents.TopicWorkspaceCreated || got.WorkspaceID != "ws-b" {
		t.Fatalf("envelope = %+v, want workspace-created to replace older queued event", got)
	}
}

func TestEnqueueEventStreamEnvelopeDropsNonCriticalWhenChannelFull(t *testing.T) {
	ch := make(chan streamEvent, 1)
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: workspaceevents.TopicWorkspaceCreated, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: "/custom/non-critical", WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != workspaceevents.TopicWorkspaceCreated || got.WorkspaceID != "ws-a" {
		t.Fatalf("envelope = %+v, want existing critical event preserved", got)
	}
}

func TestEnqueueEventStreamEnvelopeKeepsLatestCriticalStateWhenChannelFull(t *testing.T) {
	ch := make(chan streamEvent, 1)
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-a"})
	enqueueEventStreamEnvelope(ch, streamEvent{Topic: workspaceevents.TopicWorkspaceStateChanged, WorkspaceID: "ws-b"})

	got := <-ch
	if got.Topic != workspaceevents.TopicWorkspaceStateChanged || got.WorkspaceID != "ws-b" {
		t.Fatalf("envelope = %+v, want latest critical state event", got)
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
	for _, want := range []string{"创建工作空间", "六类独立审查", "确认规格并准入", "提交执行", "运行摘要", "当前运行"} {
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

func TestWorkspaceCreateEndpointReturnsCreatedWorkspace(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "memory"})

	workspace := createWorkspaceForHTTP(t, handler, "http workspace", "workspaces/ws-http")
	if !strings.HasPrefix(workspace.ID, "ws-") || workspace.Description != "http description" {
		t.Fatalf("owner workspace identity = %q", workspace.ID)
	}
	if body := getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs", http.StatusOK); !strings.Contains(body, `"runs": []`) {
		t.Fatalf("creation implicitly started a Run: %s", body)
	}
}

func TestWorkspaceCreateEndpointDefaultsDeliverableDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})

	body := workspaceResponse{Workspace: createWorkspaceForHTTP(t, handler, "http default directory workspace", "")}
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
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "config"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
	for _, dir := range []string{
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "artifacts"),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "runs"),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "events"),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "llm-calls"),
		filepath.Join(dataRoot, "internal", "workspaces", body.Workspace.ID, "indexes"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("owner-private directory %q stat err = %v, want not created by application", dir, err)
		}
	}
}

func TestArtifactContentEndpointReturnsResultOwnerContent(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-http-artifact")
	runID := common.RunID("run-http-artifact")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	dataRoot := t.TempDir()
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	var err error
	exports, err := testSessionExports(session)
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
	if got := rec.Header().Get("X-Workorch-Content-SHA256"); len(got) != 64 {
		t.Fatalf("artifact content hash = %q, want sha256", got)
	}
	if rec.Body.String() != "HTTP artifact payload\n" {
		t.Fatalf("body = %q, want artifact payload", rec.Body.String())
	}
}

func TestRuntimePayloadEndpointReturnsRunOwnerStepPayload(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-http-raw")
	runID := common.RunID("run-http-raw")
	stepID := common.StepID("run-http-raw/step-1")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "http raw", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "test", State: common.StepStateSucceeded, Input: workorcheventpayload.New(workorcheventpayload.StringField("model", "test"))}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatal(err)
	}

	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	stepToken := base64.RawURLEncoding.EncodeToString([]byte(stepID))
	ref := "workorch://workspaces/" + string(workspaceID) + "/runs/" + string(runID) + "/steps/" + stepToken + "/input.json"
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
	if got := rec.Header().Get("X-Workorch-Content-SHA256"); len(got) != 64 {
		t.Fatalf("runtime content hash = %q, want sha256", got)
	}
	if rec.Body.String() != `{"model":"test"}` {
		t.Fatalf("body = %q, want typed step input payload", rec.Body.String())
	}
}

func TestRuntimePayloadEndpointRejectsUnownedLLMRawRef(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})
	llmToken := base64.RawURLEncoding.EncodeToString([]byte("run-http-raw/llm-1"))
	ref := "workorch://workspaces/ws-http-raw/runs/run-http-raw/llm-calls/" + llmToken + "/request.json"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runtime/payloads?ref="+url.QueryEscape(ref), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDetailContentEndpointsRequireExplicitRefsBeforeReading(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var artifactReads atomic.Int32
	var rawReads atomic.Int32
	state.readArtifactContent = func(context.Context, string, string, string) (content.ArtifactContent, error) {
		artifactReads.Add(1)
		return content.ArtifactContent{}, errors.New("artifact reader should not be called without uri")
	}
	state.readRuntimeRawContent = func(context.Context, string, string, string) (content.RuntimeRawContent, error) {
		rawReads.Add(1)
		return content.RuntimeRawContent{}, errors.New("runtime raw reader should not be called without ref")
	}
	handler := newMuxWithState(state)

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "artifact missing uri", path: "/api/v1/workorch/artifacts/content"},
		{name: "artifact blank uri", path: "/api/v1/workorch/artifacts/content?uri=%20%20"},
		{name: "runtime missing ref", path: "/api/v1/workorch/runtime/payloads"},
		{name: "runtime blank ref", path: "/api/v1/workorch/runtime/payloads?ref=%20%20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	if artifactReads.Load() != 0 || rawReads.Load() != 0 {
		t.Fatalf("reader calls artifact=%d raw=%d, want missing refs rejected before reader", artifactReads.Load(), rawReads.Load())
	}
}

func TestArtifactContentEndpointDoesNotAuthorizeFromTransportCache(t *testing.T) {
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
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
	configureSessionBackedTestOperations(state, session)
	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Run: appmodelcore.Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateCompleted)},
		Artifacts: []projectioncontract.Artifact{{
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

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 because transport cache is not artifact authority, body=%s", rec.Code, rec.Body.String())
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
	if err := server.state.workspaceProjectionWriter.RebuildWorkspace(context.Background(), workspace.ID); err != nil {
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

func TestWorkspaceGetEndpointDoesNotSynthesizeGovernanceIntoWorkspaceOwner(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-get-prompt-owner-state")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.readWorkspace = func(context.Context, string, string, string) (appmodelcore.Workspace, error) {
		return appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-get-prompt-owner-state",
		}, nil
	}
	handler := newMuxWithState(state)
	// The test installs a deliberately stale projection fixture. Wait for the
	// initial asynchronous rebuild to finish first, otherwise it can race and
	// overwrite that fixture before the read route is exercised.
	waitForCondition(t, time.Second, func() bool {
		return state.workspaceProjectionStatus().State == workspaceProjectionStateReady
	})
	state.workspaceProjectionWriter.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-get-prompt-owner-state",
		},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-get-prompt-owner-state",
			WorkspaceID: string(workspaceID),
			RunID:       "run-get-prompt-owner-state",
			State:       string(common.DecisionTicketStateOpen),
		}},
		ViewModel: projectioncontract.WorkspaceViewModel{
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
	if body.Workspace.State != string(common.WorkspaceStateActive) ||
		body.Workspace.ActiveRunID != "run-get-prompt-owner-state" {
		t.Fatalf("workspace = %+v, want canonical owner fields without synthesized governance", body.Workspace)
	}
	if body.Workspace.ViewModel.PromptMode != "decision" {
		t.Fatalf("view model prompt = %q, want separate read-only decision display", body.Workspace.ViewModel.PromptMode)
	}
}

func TestWorkspaceDeleteEndpointDeletesWorkspaceAndSupportsPurge(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})

	workspace := createWorkspaceForHTTP(t, handler, "http delete workspace", "")
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
	if _, err := os.Stat(filepath.Join(dataRoot, "internal", "workspaces", workspace.ID)); err != nil {
		t.Fatalf("Workspace tombstone/journal dir err = %v, want retained", err)
	}
}

func TestWorkspaceCreateEndpointRejectsClientWorkspaceID(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-authoring", strings.NewReader(`{"workspaceId":"ws-client","title":"client workspace","deliverableDirectory":"workspaces/ws-client"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkspaceCreateEndpointRejectsRawRuntimeContractDeclaration(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "memory"})

	for _, body := range []string{
		`{"title":"raw hints","contextHints":{"profileContracts":{}}}`,
		`{"title":"raw policies","evidencePolicies":[{"id":"recipe"}]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-authoring", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("raw workspace contract status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestWorkspaceCreateEndpointReturnsBadRequestForInvalidBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-authoring", strings.NewReader(`{"title":""}`))
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

func TestRetiredGoalAndDoDWritesCannotMaterializeDefinitions(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "file", DataRoot: t.TempDir()})
	workspace := createWorkspaceForHTTP(t, handler, "definition update workspace", "workspaces/definition-update")
	// 未确认草稿不得通过旧 Goal/DoD CRUD 物化正式定义。
	for _, resource := range []string{"goals", "dods"} {
		postAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspace.ID+"/"+resource, `{}`, http.StatusNotFound)
		putAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspace.ID+"/"+resource+"/retired", `{}`, http.StatusNotFound)
		body := getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspace.ID+"/"+resource, http.StatusOK)
		var list map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			t.Fatal(err)
		}
		var items []json.RawMessage
		if err := json.Unmarshal(list[resource], &items); err != nil || len(items) != 0 {
			t.Fatalf("retired writes changed %s: %s / %v", resource, body, err)
		}
	}
}

func TestGoalAndDoDPathScopedEndpointsHandleDuplicateDefinitionIDs(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	workspaceA := createWorkspaceForHTTP(t, handler, "duplicate definition workspace a", "workspaces/dup-definition-a")
	workspaceB := createWorkspaceForHTTP(t, handler, "duplicate definition workspace b", "workspaces/dup-definition-b")
	ctx := context.Background()
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	var err error
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	goalID := common.GoalID("goal-shared-http")
	dodID := common.DoDID("dod-shared-http")
	for _, workspaceID := range []string{workspaceA.ID, workspaceB.ID} {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
		if err := exports.Repositories.Goals.Create(scopedCtx, &workspacemodel.Goal{
			ID:                 goalID,
			WorkspaceID:        common.WorkspaceID(workspaceID),
			Text:               "- goal for " + workspaceID,
			DefinitionRevision: "goal:00000000000000000001:" + strings.Repeat("0", 64),
			LineageState:       "active",
		}); err != nil {
			t.Fatalf("create duplicate goal for %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.DoDs.Create(scopedCtx, &workspacemodel.DoD{
			ID:                 dodID,
			WorkspaceID:        common.WorkspaceID(workspaceID),
			Items:              []string{"dod for " + workspaceID},
			State:              common.DoDStateReady,
			DefinitionRevision: "dod:00000000000000000001:" + strings.Repeat("0", 64),
			LineageState:       "active",
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

	putAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/goals/"+string(goalID), `{"text":"updated duplicate goal b"}`, http.StatusNotFound)
	putAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/dods/"+string(dodID), `{"items":["updated duplicate dod b"]}`, http.StatusNotFound)
	var dodGetBody dodResponse
	if err := json.Unmarshal([]byte(getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/dods/"+string(dodID), http.StatusOK)), &dodGetBody); err != nil {
		t.Fatal(err)
	}
	if dodGetBody.DoD.WorkspaceID != workspaceB.ID || len(dodGetBody.DoD.Items) != 1 || dodGetBody.DoD.Items[0] != "dod for "+workspaceB.ID {
		t.Fatalf("retired write changed scoped DoD: %+v", dodGetBody.DoD)
	}
	var unchangedGoal goalResponse
	if err := json.Unmarshal([]byte(getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+workspaceB.ID+"/goals/"+string(goalID), http.StatusOK)), &unchangedGoal); err != nil || !reflect.DeepEqual(unchangedGoal.Goal, goalGetBody.Goal) {
		t.Fatalf("retired write changed scoped Goal: %+v / %v", unchangedGoal.Goal, err)
	}

	session = newOwnerRuntimeSession(t, "file", dataRoot)
	exports, err = testSessionExports(session)
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

func TestRunSnapshotEndpointPreservesRunScopedSemanticPlanningDiagnostic(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		if runID != "run-semantic-current" {
			t.Fatalf("run snapshot id = %q, want run-semantic-current", runID)
		}
		diagnostic := projectioncontract.RuntimeSemanticDiagnostic{
			SchemaVersion: "diagnostic-v1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OriginalClass: "owner_contract", OriginalCode: "target_stale", OriginalMessage: "owner diagnostic", Signature: "signature-1",
			FieldViolations: []runtimecontract.RuntimeFieldViolation{{
				Path: "$.operations[0]", Actual: "summary", Allowed: []string{"targetRef"},
				Required: []string{"content"}, Forbidden: []string{"payload", "summary"},
			}},
			DetailsRef: "runtime-diagnostic/ws-current/run-current/owner", DetailsHash: "details-hash", DetailsBytes: 9001,
			ContentType: "application/json", OmittedReason: "bounded_payload", Truncated: true, Redacted: true,
		}
		return readmodelcore.RunSnapshotState{
			// ActiveRunID deliberately identifies an older workspace projection.
			// The requested Run remains the only scope allowed to supply semantic planning.
			Workspace: appmodelcore.Workspace{ID: "ws-semantic", ActiveRunID: "run-semantic-stale", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: "run-semantic-current", WorkspaceID: "ws-semantic", State: string(common.RunStateActive)},
			Acceptance: projectioncontract.AcceptanceProjection{SemanticPlanning: &projectioncontract.RuntimeSemanticPlanning{
				WorkspaceID: "ws-semantic", RunID: "run-semantic-current", DefinitionRevision: "definition-current", StateRevision: 41,
				State: "feasibility", Phase: string(runtimecontract.RuntimeSemanticPlanningFeasibilityRequired),
				Items: []projectioncontract.RuntimeSemanticPlanningItem{{
					CoverageWorkItemID: "cwi-current", Status: string(runtimecontract.RuntimeSemanticFeasibilityInfeasible),
					Proofs: []projectioncontract.RuntimeSemanticPlanningProof{{
						OwnerModuleID: runtimecontract.RuntimeModuleFile, Status: string(runtimecontract.RuntimeSemanticFeasibilityInfeasible), Diagnostic: &diagnostic,
					}},
				}},
				Failure: &diagnostic,
			}},
		}, nil
	}
	handler := newMuxWithStateNoStart(state)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-semantic/runs/run-semantic-current/snapshot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal semantic snapshot: %v", err)
	}
	semantic := body.Snapshot.Acceptance.SemanticPlanning
	if semantic == nil || semantic.WorkspaceID != "ws-semantic" || semantic.RunID != "run-semantic-current" ||
		semantic.DefinitionRevision != "definition-current" || semantic.StateRevision != 41 || len(semantic.Items) != 1 ||
		semantic.Items[0].CoverageWorkItemID != "cwi-current" || len(semantic.Items[0].Proofs) != 1 ||
		semantic.Items[0].Proofs[0].Diagnostic == nil || semantic.Failure == nil {
		t.Fatalf("semantic planning snapshot = %+v", semantic)
	}
	diagnostic := semantic.Items[0].Proofs[0].Diagnostic
	if diagnostic.OriginalMessage != "owner diagnostic" || len(diagnostic.FieldViolations) != 1 ||
		diagnostic.FieldViolations[0].Path != "$.operations[0]" || diagnostic.FieldViolations[0].Actual != "summary" ||
		diagnostic.FieldViolations[0].Allowed[0] != "targetRef" || diagnostic.FieldViolations[0].Required[0] != "content" ||
		diagnostic.FieldViolations[0].Forbidden[1] != "summary" || diagnostic.DetailsHash != "details-hash" ||
		diagnostic.DetailsBytes != 9001 || diagnostic.OmittedReason != "bounded_payload" || !diagnostic.Truncated || !diagnostic.Redacted {
		t.Fatalf("semantic planning snapshot diagnostic = %+v", diagnostic)
	}
	if strings.Contains(rec.Body.String(), "run-semantic-stale\",\"definitionRevision") {
		t.Fatalf("stale workspace run supplied semantic planning: %s", rec.Body.String())
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
		State: common.StepStateSucceeded,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := runScopedRepos.ToolCalls.Create(ctx, &runtimeevidencemodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		State:       common.ToolCallStateCompleted,
		Output:      mustServerGovernancePayload(t, map[string]any{"exitCode": 0}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := runScopedRepos.Patches.Create(ctx, &runtimeevidencemodel.Patch{
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
	if err := runScopedRepos.ValidationResults.Create(ctx, &runtimeevidencemodel.ValidationResult{
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var calls atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		call := calls.Add(1)
		status := "running"
		if call >= 2 {
			status = "completed"
		}
		return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-wait-status", State: status}, nil
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

func TestRunStatusEndpointInvalidatesCachedTerminalSnapshotOnTypedOwnerRunStateNotification(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var statusReads atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		statusReads.Add(1)
		return projectioncontract.RunStatus{
			ID:          runID,
			WorkspaceID: "ws-status-owner-event",
			State:       string(common.RunStateActive),
		}, nil
	}
	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-status-owner-event", State: string(common.WorkspaceStateActive)},
		Run: appmodelcore.Run{
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

	state.RecordRunStateChanged(context.Background(), runevents.RunStateChangedData{
		WorkspaceID:            "ws-status-owner-event",
		RunID:                  "run-status-owner-event",
		RunRevision:            1,
		State:                  common.RunStateActive,
		WorkspaceRunGeneration: 1, CausationID: runevents.RunStateChangedCausationID("run-status-owner-event", 1, 1, 0),
	})

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status after owner event = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.State != string(common.RunStateActive) {
		t.Fatalf("status after owner event = %+v, want fresh running status", body.Status)
	}
	if statusReads.Load() == 0 {
		t.Fatal("status endpoint reused stale cached snapshot after owner run state event")
	}
}

func TestRunStatusEndpointDoesNotInvalidateCachedTerminalSnapshotOnEventstreamRecord(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var statusReads atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		statusReads.Add(1)
		return projectioncontract.RunStatus{
			ID:          runID,
			WorkspaceID: "ws-status-eventstream-record",
			State:       string(common.RunStateActive),
		}, nil
	}
	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-status-eventstream-record", State: string(common.WorkspaceStateActive)},
		Run: appmodelcore.Run{
			ID:          "run-status-eventstream-record",
			WorkspaceID: "ws-status-eventstream-record",
			State:       string(common.RunStateCompleted),
		},
	})
	handler := newMuxWithState(state)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-status-eventstream-record/runs/run-status-eventstream-record/status", nil)

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-status-eventstream-record",
		RunID:       "run-status-eventstream-record",
		SubjectID:   "run-status-eventstream-record",
		Payload:     recordpayload.RunPayloadData{State: common.RunStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish eventstream run record: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status after eventstream record = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.State != string(common.RunStateCompleted) || statusReads.Load() != 0 {
		t.Fatalf("eventstream record invalidated terminal cache: status=%+v reads=%d", body.Status, statusReads.Load())
	}
}

func TestRunStatusWaitUsesStatusChangeNotification(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var completed atomic.Bool
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		status := "running"
		if completed.Load() {
			status = "completed"
		}
		return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-notified-status", State: status}, nil
	}

	done := make(chan struct{})
	var got projectioncontract.RunStatus
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var snapshotReads atomic.Int32
	var observabilityReads atomic.Int32
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		snapshotReads.Add(1)
		return readmodelcore.RunSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-cache", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	state.readRunObservability = func(context.Context, string, string, string) (readmodelcore.RunObservabilityState, error) {
		observabilityReads.Add(1)
		return readmodelcore.RunObservabilityState{}, nil
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-cache-unique", State: string(common.WorkspaceStateActive)},
		Run:       appmodelcore.Run{ID: "run-cache-first", WorkspaceID: "ws-cache-unique", State: string(common.RunStateCompleted)},
	})
	if cached, ok := state.cachedRunSnapshotByWorkspace("ws-cache-unique"); !ok || cached.Run.ID != "run-cache-first" {
		t.Fatalf("cached snapshot = %+v ok=%v, want first run cache hit", cached, ok)
	}

	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-cache-unique", State: string(common.WorkspaceStateActive)},
		Run:       appmodelcore.Run{ID: "run-cache-second", WorkspaceID: "ws-cache-unique", State: string(common.RunStateCompleted)},
	})
	if cached, ok := state.cachedRunSnapshotByWorkspace("ws-cache-unique"); ok {
		t.Fatalf("cached snapshot = %+v, want miss when one workspace has multiple cached terminal runs", cached)
	}
}

func TestRunStatusWaitWarmsTerminalRunSnapshotCache(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var snapshotReads atomic.Int32
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-status-warm-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		snapshotReads.Add(1)
		return readmodelcore.RunSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-status-warm-cache", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-status-warm-cache", State: string(common.RunStateCompleted)},
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var snapshotReads atomic.Int32
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		snapshotReads.Add(1)
		return readmodelcore.RunSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-notify-warm-cache", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-notify-warm-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	handler := newMuxWithState(state)

	state.notifyRunStatusChangedForRun(appmodelcore.Run{
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var snapshotReads atomic.Int32
	releaseRead := make(chan struct{})
	readStarted := make(chan struct{})
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-status-inflight-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		snapshotReads.Add(1)
		close(readStarted)
		<-releaseRead
		return readmodelcore.RunSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-status-inflight-cache", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-status-inflight-cache", State: string(common.RunStateCompleted)},
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-inflight-clear")
	runID := "run-inflight-clear"

	if _, duplicate := state.ensureRunSnapshotInflight(ctx, runID); duplicate {
		t.Fatal("first inflight registration reported duplicate")
	}
	state.cacheRunSnapshot(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-inflight-clear", State: string(common.WorkspaceStateActive)},
		Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-inflight-clear", State: string(common.RunStateCompleted)},
	})
	state.clearRunSnapshotCache()
	if _, ok := state.cachedRunSnapshot(ctx, runID); ok {
		t.Fatal("cached snapshot survived invalidation")
	}

	waitDone := make(chan struct{})
	var waited readmodelcore.RunSnapshotState
	var waitErr error
	var waitedInflight bool
	go func() {
		defer close(waitDone)
		waited, waitErr, waitedInflight = state.waitRunSnapshotInflight(ctx, runID)
	}()
	time.Sleep(20 * time.Millisecond)
	state.completeRunSnapshotInflight(ctx, runID, readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-inflight-clear", State: string(common.WorkspaceStateActive)},
		Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-inflight-clear", State: string(common.RunStateCompleted)},
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var snapshotReads atomic.Int32
	var observabilityReads atomic.Int32
	releaseRead := make(chan struct{})
	readStarted := make(chan struct{})
	state.readRunStatus = func(_ context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-observability-inflight-cache", State: string(common.RunStateCompleted)}, nil
	}
	state.readRunSnapshot = func(_ context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		snapshotReads.Add(1)
		close(readStarted)
		<-releaseRead
		return readmodelcore.RunSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-observability-inflight-cache", State: string(common.WorkspaceStateActive)},
			Run:       appmodelcore.Run{ID: runID, WorkspaceID: "ws-observability-inflight-cache", State: string(common.RunStateCompleted)},
		}, nil
	}
	state.readRunObservability = func(context.Context, string, string, string) (readmodelcore.RunObservabilityState, error) {
		observabilityReads.Add(1)
		return readmodelcore.RunObservabilityState{}, nil
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
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
	decisionTicketID := decisionRun.BlockingTicketID
	retiredDecisionRun, err := runScopedRepos.Runs.GetByWorkspaceID(ctx, common.WorkspaceID(workspace.ID), common.RunID(decisionRun.ID))
	if err != nil {
		t.Fatalf("get decision Run for retirement: %v", err)
	}
	retiredDecisionRun.State = common.RunStateCompleted
	retiredDecisionRun.BlockingTicketID = ""
	if err := runScopedRepos.Runs.Update(ctx, retiredDecisionRun); err != nil {
		t.Fatalf("retire decision Run fixture: %v", err)
	}
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
	if body.Snapshot.Workspace.State != string(common.WorkspaceStateActive) || body.Snapshot.Workspace.ActiveRunID != approvalRun.ID {
		t.Fatalf("workspace projection = %+v, want canonical active owner state and active run %s", body.Snapshot.Workspace, approvalRun.ID)
	}
	if body.Snapshot.GovernanceDisplay.ApprovalID != approvalRun.BlockingApprovalID || body.Snapshot.GovernanceDisplay.PromptMode != "approval" {
		t.Fatalf("governance display = %+v, want approval handoff %s", body.Snapshot.GovernanceDisplay, approvalRun.BlockingApprovalID)
	}
	if len(body.Snapshot.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", body.Snapshot.Runs)
	}
	if len(body.Snapshot.Decisions) != 1 || body.Snapshot.Decisions[0].ID != decisionTicketID {
		t.Fatalf("decisions = %+v, want decision %s", body.Snapshot.Decisions, decisionTicketID)
	}
	if len(body.Snapshot.Approvals) != 1 || body.Snapshot.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", body.Snapshot.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestWorkspaceSnapshotEndpointReturnsCompactProjection(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-compact-http")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
	handler := newMuxWithState(state)
	ctx := context.Background()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	if err := repos.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "compact projection",
		DeliverableDirectory: "workspaces/compact-projection",
		State:                common.WorkspaceStateActive,
		ActiveRunID:          "run-00",
		ContextHints:         mustServerGovernancePayload(t, map[string]any{"large": strings.Repeat("x", 128)}),
		EvidencePolicies:     []workorcheventpayload.Payload{mustServerGovernancePayload(t, map[string]any{"large": strings.Repeat("x", 128)})},
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for idx := 0; idx < 12; idx++ {
		runID := common.RunID(fmt.Sprintf("run-%02d", idx))
		state := common.RunStateCompleted
		if idx == 0 {
			state = common.RunStateActive
		}
		if err := repos.Runs.Create(ctx, &runmodel.Run{
			ID: runID, WorkspaceID: workspaceID, State: state,
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
			ID:             common.EventRecordID(fmt.Sprintf("usage-%02d", idx)),
			Version:        recordpayload.WorkorchEventVersion,
			Topic:          "/llm/usage/recorded",
			OccurredAt:     fmt.Sprintf("2026-06-12T00:%02d:00Z", idx),
			SourceModule:   llmcommon.ModuleID,
			SubjectType:    "llm_usage",
			SubjectID:      fmt.Sprintf("request-compact-%02d", idx),
			WorkspaceID:    workspaceID,
			RunID:          runID,
			StepID:         common.StepID(fmt.Sprintf("%s/step-%02d", runID, idx)),
			StreamPosition: int64(idx + 1),
			Revision:       int64(idx + 1),
			Payload: mustServerEventPayload(t, map[string]any{
				recordpayload.PayloadKeyWorkspaceID:          string(workspaceID),
				recordpayload.PayloadKeyRunID:                string(runID),
				recordpayload.PayloadKeyStepID:               fmt.Sprintf("%s/step-%02d", runID, idx),
				recordpayload.PayloadKeyProviderID:           "provider-compact",
				recordpayload.PayloadKeyProvider:             "compatible",
				recordpayload.PayloadKeyModel:                "model-compact",
				recordpayload.PayloadKeyOperation:            "responses",
				recordpayload.PayloadKeyInputTokens:          10 + idx,
				recordpayload.PayloadKeyOutputTokens:         1,
				recordpayload.PayloadKeyTotalTokens:          11 + idx,
				recordpayload.PayloadKeyPhysicalResponseMode: string(llmaudit.PhysicalResponseModeNonStreaming),
				recordpayload.PayloadKeyProviderSubmitted:    true,
				recordpayload.PayloadKeyStatus:               "completed",
			}),
		}); err != nil {
			t.Fatalf("append usage event %d: %v", idx, err)
		}
	}
	// Let the asynchronous baseline finish after the direct repository fixture,
	// then perform the explicit per-Workspace readback. This prevents an older
	// empty baseline from committing after the focused rebuild and erasing its
	// workspace-local revision.
	waitForCondition(t, 2*time.Second, func() bool {
		return state.workspaceProjectionStatus().State == workspaceProjectionStateReady
	})
	state.rebuildProjectedWorkspace(ctx, string(workspaceID))

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
	if body.Snapshot.Run == nil {
		t.Fatalf("run = %+v, want compact active run", body.Snapshot.Run)
	}
	if len(body.Snapshot.Runs) != 11 {
		t.Fatalf("runs = %d, want active run plus latest 10 compact runs", len(body.Snapshot.Runs))
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	state.workspaceProjectionWriter.UpsertWorkspace(appmodelcore.Workspace{
		ID:          string(workspaceID),
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-explicit-compact-detail",
	})
	state.workspaceProjectionWriter.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:                   string(workspaceID),
			State:                string(common.WorkspaceStateActive),
			ActiveRunID:          "run-explicit-compact-detail",
			DeliverableDirectory: "workspaces/explicit-compact-detail",
		},
	})
	var detailReads atomic.Int32
	var fullReads atomic.Int32
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		detailReads.Add(1)
		return readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateActive),
				ActiveRunID:          "run-explicit-compact-detail",
				DeliverableDirectory: "workspaces/explicit-compact-detail",
			},
			Run: &appmodelcore.Run{
				ID:          "run-explicit-compact-detail",
				WorkspaceID: string(workspaceID),
				State:       string(common.RunStateActive),
			},
		}, nil
	}
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		fullReads.Add(1)
		return readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateActive),
				ActiveRunID:          "run-explicit-compact-detail",
				ContextHints:         map[string]any{"large": strings.Repeat("x", 128)},
				EvidencePolicies:     []map[string]any{{"large": strings.Repeat("x", 128)}},
				DeliverableDirectory: "workspaces/explicit-compact-detail",
			},
			Run: &appmodelcore.Run{
				ID:          "run-explicit-compact-detail",
				WorkspaceID: string(workspaceID),
				State:       string(common.RunStateActive),
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
	if body.Snapshot.Run == nil {
		t.Fatalf("run = %+v, want projected compact run", body.Snapshot.Run)
	}
	if len(body.Snapshot.Runs) != 1 || body.Snapshot.Runs[0].ID != "run-explicit-compact-detail" {
		t.Fatalf("runs = %+v, want selected run included in projected detail relations", body.Snapshot.Runs)
	}
}

func TestWorkspaceSnapshotEndpointSummaryDoesNotHydrateDetail(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-summary-no-detail-hydrate")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	state.workspaceProjectionWriter.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-summary-no-detail-hydrate",
		},
	})
	var projectionReads atomic.Int32
	state.readProjectionSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		projectionReads.Add(1)
		return readmodelcore.WorkspaceSnapshotState{}, errors.New("detail projection read should not be used for summary")
	}
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		return readmodelcore.WorkspaceSnapshotState{}, errors.New("full snapshot read should not be used for summary")
	}
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		return readmodelcore.WorkspaceSnapshotState{}, errors.New("detail snapshot read should not be used for summary")
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	state.workspaceProjectionWriter.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-detail-prompt-same-source",
		},
	})
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		return readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{
				ID:          string(workspaceID),
				State:       string(common.WorkspaceStateActive),
				ActiveRunID: "run-detail-prompt-same-source",
			},
			Run: &appmodelcore.Run{
				ID:               "run-detail-prompt-same-source",
				WorkspaceID:      string(workspaceID),
				State:            string(common.RunStateBlockedOnDecision),
				BlockingTicketID: "ticket-detail-prompt-same-source",
			},
			Decisions: []projectioncontract.Decision{{
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
	if body.Snapshot.Workspace.State != string(common.WorkspaceStateActive) ||
		body.Snapshot.Workspace.ActiveRunID != "run-detail-prompt-same-source" ||
		body.Snapshot.Run == nil ||
		body.Snapshot.Run.State != string(common.RunStateBlockedOnDecision) ||
		body.Snapshot.Run.BlockingTicketID != "ticket-detail-prompt-same-source" ||
		body.Snapshot.GovernanceDisplay.DecisionID != "ticket-detail-prompt-same-source" {
		t.Fatalf("snapshot workspace/run = %+v/%+v, want blocked decision owner state", body.Snapshot.Workspace, body.Snapshot.Run)
	}
	if body.Snapshot.ViewModel.PromptMode != "decision" ||
		body.Snapshot.Workspace.ViewModel.PromptMode != "decision" {
		t.Fatalf("snapshot viewModel = %+v workspace viewModel = %+v, want decision from detail owner state", body.Snapshot.ViewModel, body.Snapshot.Workspace.ViewModel)
	}
}

func TestWorkspaceSnapshotEndpointObservabilityUsesExplicitFullRead(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-observability-full-read")
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.workspaceProjectionWriter.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-observability-full-read",
		},
	})
	var fullReads atomic.Int32
	state.readWorkspaceSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		fullReads.Add(1)
		return readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{
				ID:                   string(workspaceID),
				State:                string(common.WorkspaceStateActive),
				ActiveRunID:          "run-observability-full-read",
				ContextHints:         map[string]any{"large": strings.Repeat("x", 128)},
				EvidencePolicies:     []map[string]any{{"large": strings.Repeat("x", 128)}},
				DeliverableDirectory: "workspaces/observability-full-read",
			},
			Run: &appmodelcore.Run{
				ID:          "run-observability-full-read",
				WorkspaceID: string(workspaceID),
				State:       string(common.RunStateActive),
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
	if body.Snapshot.Run == nil {
		t.Fatalf("run = %+v, want observability full run retained", body.Snapshot.Run)
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
	if len(listed.Decisions) != 2 || listed.Decisions[0].ID != startedA.BlockingTicketID || listed.Decisions[1].ID != startedB.BlockingTicketID {
		t.Fatalf("listed decisions = %+v, want owner composite-key order", listed.Decisions)
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

func TestDecisionReadEndpointsProjectTypedRuntimeDefinitionClarification(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newTestMux(t, Config{Backend: "file", DataRoot: dataRoot})
	workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "typed definition clarification read", "run-definition-read")
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(workspace.ID))
	ticket, err := repos.Tickets.Get(ctx, common.TicketID(started.BlockingTicketID))
	if err != nil {
		t.Fatalf("get definition ticket: %v", err)
	}
	ticket.DecisionKind = "runtime_definition_clarification"
	ticket.FailureClass = "runtime_definition_gap"
	ticket.DefinitionClarification = &decisionmodel.DefinitionClarificationBinding{
		Class: decisionmodel.DefinitionClarificationRuntime, OpenBindingHash: "open-binding", RequiredActionKind: decisionevents.DefinitionClarificationActionReviseAndRestart,
		ViolationOrGapCode: string(decisionevents.RuntimeDefinitionGapImplicitOneOf), CanonicalSignatureVersion: "runtime-gap-v1",
		CanonicalSignature: "safe-gap-signature", CanonicalSignatureHash: "signature-hash", OffendingRefs: []string{"dod:item-1", "goal:goal-1"},
		DefinitionRevision: "definition-old", StartupInputHash: "startup-old", AuthoritySnapshotID: "authority-1",
		ClarificationDispatchID: "clarification-dispatch-1", FocusRef: "focus-1", LogicalRequestID: "logical-1", SemanticDocumentHash: "document-1",
	}
	if err := repos.Tickets.Update(ctx, ticket); err != nil {
		t.Fatalf("update definition ticket: %v", err)
	}

	for _, path := range []string{
		"/api/v1/workorch/decisions?workspaceId=" + url.QueryEscape(workspace.ID),
		"/api/v1/workorch/decisions/" + url.PathEscape(started.BlockingTicketID) + "?workspaceId=" + url.QueryEscape(workspace.ID),
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200, body=%s", path, rec.Code, rec.Body.String())
		}
		var projected projectioncontract.Decision
		if strings.Contains(path, "?") && !strings.Contains(path, "/"+url.PathEscape(started.BlockingTicketID)) {
			var response decisionsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Decisions) != 1 {
				t.Fatalf("unmarshal definition list: err=%v decisions=%+v", err, response.Decisions)
			}
			projected = response.Decisions[0]
		} else {
			var response decisionResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("unmarshal definition detail: %v", err)
			}
			projected = response.Decision
		}
		binding := projected.DefinitionClarification
		if binding == nil || binding.Class != string(decisionmodel.DefinitionClarificationRuntime) || binding.BindingState != "open" ||
			binding.RequiredAction != decisionevents.DefinitionClarificationActionReviseAndRestart ||
			binding.ViolationOrGapCode != string(decisionevents.RuntimeDefinitionGapImplicitOneOf) || len(binding.OffendingRefs) != 2 ||
			binding.DefinitionRevision != "definition-old" || binding.AuthoritySnapshotID != "authority-1" ||
			binding.ClarificationDispatchID != "clarification-dispatch-1" || binding.FocusRef != "focus-1" {
			t.Fatalf("GET %s definition clarification = %+v", path, binding)
		}
	}
}

func TestDecisionResolveEndpointResolvesTicketWithoutLegacyRunResume(t *testing.T) {
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

	blocked := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnDecision))
	if blocked.BlockingTicketID != started.BlockingTicketID {
		t.Fatalf("run = %+v, want legacy decision blocker unchanged", blocked)
	}
}

func TestDecisionResolveEndpointReturnsExactOwnerReadbackAfterResponseLoss(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	_, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "decision response loss http workspace", "run-decision-response-loss-http")
	body := `{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"http-idempotent","answer":"same answer"}}`
	resolve := func() decisionResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("decision response-loss status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		var response decisionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("unmarshal decision response-loss response: %v", err)
		}
		return response
	}
	first := resolve()
	second := resolve()
	if first.Decision.ResolutionID == "" || second.Decision.ResolutionID != first.Decision.ResolutionID {
		t.Fatalf("response-loss readbacks = %+v / %+v, want same owner resolution", first.Decision, second.Decision)
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
			readSession := newOwnerRuntimeSession(t, "file", dataRoot)
			ticket, err := appservice.ReadDecisionWithSession(context.Background(), readSession, started.BlockingTicketID)
			_ = readSession.Close(context.Background())
			if err != nil {
				t.Fatalf("read decision: %v", err)
			}
			if ticket.State != string(common.DecisionTicketStateOpen) {
				t.Fatalf("decision state = %q, want open after invalid missing-info payload", ticket.State)
			}
		})
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

func TestDecisionResolveEndpointRejectCurrentPathWithoutLegacyRunResume(t *testing.T) {
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
	operator, _ := rejected.Decision.ResolutionPayload.String("operator")
	if operator != "tester" {
		t.Fatalf("resolution payload = %+v, want operator tester", rejected.Decision.ResolutionPayload)
	}

	blocked := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnDecision))
	if blocked.BlockingTicketID != started.BlockingTicketID {
		t.Fatalf("run = %+v, want legacy decision blocker unchanged", blocked)
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
	waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnDecision))
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
		operator, _ := item.ResolutionPayload.String("operator")
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || operator != "http-batch" {
			t.Fatalf("decision = %+v, want resolved replan with payload", item)
		}
	}
	waitRunStateForHTTP(t, dataRoot, firstRunID, string(common.RunStateBlockedOnDecision))
	waitRunStateForHTTP(t, dataRoot, secondRunID, string(common.RunStateBlockedOnDecision))
}

func TestDecisionMutationEndpointsRejectNonCanonicalOpenTicketWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		batch bool
	}{
		{name: "single"},
		{name: "batch", batch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			handler := newFileMuxWithCleanup(t, dataRoot)
			workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, "non-canonical decision mutation", "run-decision-noncanonical-"+tc.name)
			extraID := common.TicketID("ticket-decision-extra-" + tc.name)
			repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(workspace.ID))
			if err := repos.Tickets.Create(ctx, &decisionmodel.Ticket{
				ID: extraID, WorkspaceID: common.WorkspaceID(workspace.ID), RunID: common.RunID(started.ID),
				StepID: common.StepID(started.ID + "/extra-decision"), State: common.DecisionTicketStateOpen,
			}); err != nil {
				t.Fatalf("create extra Decision: %v", err)
			}

			var req *http.Request
			if tc.batch {
				body := strings.NewReader(`{"ticketIds":["` + started.BlockingTicketID + `","` + string(extraID) + `"],"resolutionType":"replan"}`)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/resolve-batch?workspaceId="+url.QueryEscape(workspace.ID), body)
			} else {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+string(extraID)+"/resolve?workspaceId="+url.QueryEscape(workspace.ID), nil)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("Decision %s status = %d, want 409, body=%s", tc.name, rec.Code, rec.Body.String())
			}
			for _, id := range []common.TicketID{common.TicketID(started.BlockingTicketID), extraID} {
				item, err := repos.Tickets.Get(ctx, id)
				if err != nil {
					t.Fatalf("get Decision %s: %v", id, err)
				}
				if item.State != common.DecisionTicketStateOpen {
					t.Fatalf("Decision %s state = %s, want open after zero-mutation conflict", id, item.State)
				}
			}
		})
	}
}

func TestDecisionMutationEndpointsRejectDefinitionTicketsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		class decisionmodel.DefinitionClarificationClass
	}{
		{name: "workspace definition", class: decisionmodel.DefinitionClarificationWorkspacePreSeal},
		{name: "runtime definition", class: decisionmodel.DefinitionClarificationRuntime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			handler := newFileMuxWithCleanup(t, dataRoot)
			workspace, started := seedBlockedDecisionRunForHTTP(t, dataRoot, tc.name, "run-definition-generic-"+strings.ReplaceAll(tc.name, " ", "-"))
			repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(workspace.ID))
			ticket, err := repos.Tickets.Get(ctx, common.TicketID(started.BlockingTicketID))
			if err != nil {
				t.Fatalf("get definition ticket: %v", err)
			}
			ticket.DefinitionClarification = &decisionmodel.DefinitionClarificationBinding{Class: tc.class}
			if err := repos.Tickets.Update(ctx, ticket); err != nil {
				t.Fatalf("update definition ticket: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve?workspaceId="+url.QueryEscape(workspace.ID), nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("generic definition resolve status = %d, want 409, body=%s", rec.Code, rec.Body.String())
			}
			persisted, err := repos.Tickets.Get(ctx, ticket.ID)
			if err != nil {
				t.Fatalf("read definition ticket after conflict: %v", err)
			}
			if persisted.State != common.DecisionTicketStateOpen {
				t.Fatalf("definition ticket state = %s, want open for dedicated admission", persisted.State)
			}
		})
	}
}

func TestDecisionBatchResolveEndpointRejectsDefinitionTicketBeforeAnyMutation(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	validWorkspace, valid := seedBlockedDecisionRunForHTTP(t, dataRoot, "valid decision batch target", "run-definition-batch-valid")
	definitionWorkspace, definition := seedBlockedDecisionRunForHTTP(t, dataRoot, "definition decision batch target", "run-definition-batch-dedicated")
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	definitionCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(definitionWorkspace.ID))
	ticket, err := repos.Tickets.Get(definitionCtx, common.TicketID(definition.BlockingTicketID))
	if err != nil {
		t.Fatalf("get definition ticket: %v", err)
	}
	ticket.DefinitionClarification = &decisionmodel.DefinitionClarificationBinding{Class: decisionmodel.DefinitionClarificationRuntime}
	if err := repos.Tickets.Update(definitionCtx, ticket); err != nil {
		t.Fatalf("update definition ticket: %v", err)
	}

	body := strings.NewReader(`{"ticketIds":["` + valid.BlockingTicketID + `","` + definition.BlockingTicketID + `"],"resolutionType":"replan"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/resolve-batch", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("definition batch status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	for _, target := range []struct {
		workspace appmodelcore.Workspace
		run       appmodelcore.Run
	}{{validWorkspace, valid}, {definitionWorkspace, definition}} {
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(target.workspace.ID))
		persisted, err := repos.Tickets.Get(ctx, common.TicketID(target.run.BlockingTicketID))
		if err != nil {
			t.Fatalf("read Decision %s after batch conflict: %v", target.run.BlockingTicketID, err)
		}
		if persisted.State != common.DecisionTicketStateOpen {
			t.Fatalf("Decision %s state = %s, want open after zero-mutation batch conflict", persisted.ID, persisted.State)
		}
	}
}

func TestDecisionResolveEndpointScopesDuplicateTicketIDsByWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	ticketID := "ticket-shared-http"
	workspaceA, startedA := seedBlockedDecisionRunWithTicketForHTTP(t, dataRoot, "decision shared http workspace a", "run-decision-shared-a", ticketID)
	workspaceB, startedB := seedBlockedDecisionRunWithTicketForHTTP(t, dataRoot, "decision shared http workspace b", "run-decision-shared-b", ticketID)

	body := strings.NewReader(`{"resolutionType":"replan","resolutionPayload":{"operator":"http-scoped"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+ticketID+"/resolve?workspaceId="+url.QueryEscape(workspaceB.ID), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resolved decisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal resolved decision: %v", err)
	}
	if resolved.Decision.ID != ticketID || resolved.Decision.WorkspaceID != workspaceB.ID || resolved.Decision.RunID != startedB.ID || resolved.Decision.State != "resolved" || resolved.Decision.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("resolved decision = %+v, want only workspace B ticket resolved", resolved.Decision)
	}

	ctx := context.Background()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	ticketA, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceA.ID)), common.TicketID(startedA.BlockingTicketID))
	if err != nil {
		t.Fatalf("get workspace A ticket: %v", err)
	}
	if ticketA.State != common.DecisionTicketStateOpen {
		t.Fatalf("workspace A ticket = %+v, want still open", ticketA)
	}
	ticketB, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceB.ID)), common.TicketID(startedB.BlockingTicketID))
	if err != nil {
		t.Fatalf("get workspace B ticket: %v", err)
	}
	if ticketB.State != common.DecisionTicketStateResolved {
		t.Fatalf("workspace B ticket = %+v, want resolved", ticketB)
	}
}

func TestDecisionBatchResolveEndpointScopesDuplicateTicketIDsByWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	ticketID := "ticket-shared-batch-http"
	workspaceA, startedA := seedBlockedDecisionRunWithTicketForHTTP(t, dataRoot, "decision shared batch http workspace a", "run-decision-shared-batch-a", ticketID)
	workspaceB, startedB := seedBlockedDecisionRunWithTicketForHTTP(t, dataRoot, "decision shared batch http workspace b", "run-decision-shared-batch-b", ticketID)

	body := strings.NewReader(`{"ticketIds":["` + ticketID + `"],"resolutionType":"replan","resolutionPayload":{"operator":"http-batch-scoped"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/resolve-batch?workspaceId="+url.QueryEscape(workspaceB.ID), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch resolve status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resolved decisionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal batch resolved decisions: %v", err)
	}
	if len(resolved.Decisions) != 1 || resolved.Decisions[0].ID != ticketID || resolved.Decisions[0].WorkspaceID != workspaceB.ID || resolved.Decisions[0].RunID != startedB.ID || resolved.Decisions[0].State != "resolved" {
		t.Fatalf("resolved decisions = %+v, want only workspace B ticket resolved", resolved.Decisions)
	}

	ctx := context.Background()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	ticketA, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceA.ID)), common.TicketID(startedA.BlockingTicketID))
	if err != nil {
		t.Fatalf("get workspace A ticket: %v", err)
	}
	if ticketA.State != common.DecisionTicketStateOpen {
		t.Fatalf("workspace A ticket = %+v, want still open", ticketA)
	}
	ticketB, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceB.ID)), common.TicketID(startedB.BlockingTicketID))
	if err != nil {
		t.Fatalf("get workspace B ticket: %v", err)
	}
	if ticketB.State != common.DecisionTicketStateResolved {
		t.Fatalf("workspace B ticket = %+v, want resolved", ticketB)
	}
}

func TestWorkorchDActivationHTTPProjectsRequestID(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation request id", "workspaces/activation-request-id")
	// 当前 Workspace 只有未确认的创作输入，Resume 必须收到 owner 拒绝；不需要旧定义写入。

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-http-activation",
		"reason":"http_test",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("activation status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode activation response: %v", err)
	}
	if body["accepted"] != false ||
		body["state"] != "rejected" ||
		body["intent"] != "resume" ||
		body["requestId"] != "request-http-activation" ||
		body["source"] != "web" ||
		body["requestReason"] != "http_test" ||
		body["outcome"] != "restart_required" ||
		body["reason"] != "workspace_active_run_absent" {
		t.Fatalf("activation response = %+v, want exact owner rejection", body)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "rejected" ||
		snapshot.RunCommand.Outcome != runtimecontract.RuntimeExecutionIntentRestartRequired ||
		snapshot.RunCommand.AdmissionReason != runtimecontract.RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent ||
		snapshot.RunCommand.Diagnostic != "workspace has no active run" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-http-activation" ||
		snapshot.RunCommand.RequestReason != "http_test" ||
		snapshot.RunCommand.Source != "web" ||
		snapshot.RunCommand.AcceptedAt != "" || snapshot.RunCommand.RejectedAt == "" {
		t.Fatalf("owner outcome projection = %+v", snapshot.RunCommand)
	}
	runs, err := state.listRuns(context.Background(), state.config.Backend, state.config.DataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("list runs after resume without active run: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("resume without active run created runs = %+v, want restart-required admission without implicit replacement", runs)
	}
}

func TestWorkorchDActivationHTTPWaitsForOwnerAdmission(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "async activation", "workspaces/async-activation")
	submitter := &blockingWorkspaceExecutionSubmitter{
		state: state, started: make(chan struct{}), release: make(chan struct{}),
	}
	state.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: state, submitter: submitter}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-http-async-activation",
		"reason":"user_requested",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		t.Fatal("workspace owner admission did not start")
	}
	select {
	case <-done:
		t.Fatal("activation HTTP handler returned before Workspace owner admission")
	case <-time.After(50 * time.Millisecond):
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("run command before owner admission = %+v, want no application-local queued state", snapshot.RunCommand)
	}

	close(submitter.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("activation HTTP handler did not return after Workspace owner admission")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("activation status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	accepted, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RunCommand == nil || accepted.RunCommand.State != "accepted" || accepted.RunCommand.AcceptedAt == "" {
		t.Fatalf("accepted run command = %+v, want synchronous owner outcome", accepted.RunCommand)
	}
}

func TestWorkorchDActivationHTTPReturnsOwnerFailureSynchronously(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "failed owner admission", "workspaces/failed-owner-admission")
	state.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: state, submitter: failingWorkspaceExecutionSubmitter{err: errors.New("execution admission submission failed")}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-http-owner-failure",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("activation status = %d, want 500 owner admission failure, body=%s", rec.Code, rec.Body.String())
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("run command = %+v, want no fabricated owner outcome for submission failure", snapshot.RunCommand)
	}
}

func TestWorkorchDRunResumeHTTPRejectsLegacyReplanIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
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
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run resume replan status = %d, want 404 for removed run-scoped route, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkorchDActivationHTTPRejectsRunScopedIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation rejects replan", "workspaces/activation-rejects-replan")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
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

func TestWorkorchDActivationHTTPRejectsScopedLegacyReplan(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation accepts scoped replan", "workspaces/activation-accepts-scoped-replan")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"runId":"run-http-replan",
		"intent":"replan",
		"requestId":"request-http-scoped-replan",
		"reason":"decision_replan_resolved",
		"source":"tui"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("activation scoped replan status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkorchDRunResumeHTTPRejectsRestartIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "run resume rejects restart intent", "workspaces/run-resume-rejects-restart")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-http-restart/resume", strings.NewReader(`{"intent":"restart","requestId":"request-http-restart"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run resume restart status = %d, want 404 for removed run-scoped route, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkorchDRunResumeHTTPRejectsRecoverIntent(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "run resume rejects recover intent", "workspaces/run-resume-rejects-recover")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/runs/run-http-recover/resume", strings.NewReader(`{"intent":"recover","requestId":"request-http-recover"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run resume recover status = %d, want 404 for removed run-scoped route, body=%s", rec.Code, rec.Body.String())
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("projected run command = %+v, want no recover command projection", snapshot.RunCommand)
	}
}

func TestWorkorchDWorkspaceRestartHTTPProjectsAcceptedSemanticPlanningActivation(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "workspace restart intent", "workspaces/workspace-restart-intent")
	// 仅验证 HTTP 对 typed 接受事实的投影；此替身不证明真实 Restart 准入。
	state.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: state, submitter: acceptedRestartWorkspaceExecutionSubmitter{}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"restart",
		"requestId":"request-http-restart",
		"reason":"operator_restart",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("workspace restart status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	var latest readmodelcore.WorkspaceSnapshotState
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
		if err == nil {
			latest = snapshot
			if snapshot.RunCommand != nil && snapshot.RunCommand.State == "accepted" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if latest.RunCommand == nil || latest.RunCommand.State != "accepted" {
		t.Fatalf("workspace restart projection = %+v, want accepted semantic planning activation", latest.RunCommand)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "restart" ||
		snapshot.RunCommand.RunID == "" ||
		snapshot.RunCommand.RequestID != "request-http-restart" ||
		snapshot.RunCommand.RequestReason != "operator_restart" ||
		snapshot.RunCommand.Source != "web" ||
		snapshot.RunCommand.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested ||
		snapshot.RunCommand.AdmissionReason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated ||
		snapshot.RunCommand.Diagnostic != "new Workspace Run activated" ||
		snapshot.RunCommand.AcceptedAt == "" || snapshot.RunCommand.RejectedAt != "" {
		t.Fatalf("projected run command = %+v, want accepted HTTP restart metadata", snapshot.RunCommand)
	}
}

func TestWorkspaceActivationFailureDoesNotFabricateOwnerOutcome(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation diagnostic", "workspaces/activation-diagnostic")
	const requestID = "request-activation-diagnostic"
	state.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: state, submitter: failingWorkspaceExecutionSubmitter{err: errors.New("activation preflight: provider target is not ready")}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"`+requestID+`",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("activation status = %d, want 500 owner admission failure, body=%s", rec.Code, rec.Body.String())
	}

	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.Run != nil {
		t.Fatalf("run = %+v, want no Run when activation preflight fails", snapshot.Run)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("run command = %+v, want no fabricated owner outcome for activation failure", snapshot.RunCommand)
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
	waitRunStateForHTTP(t, dataRoot, firstRunID, string(common.RunStateBlockedOnApproval))
	waitRunStateForHTTP(t, dataRoot, secondRunID, string(common.RunStateBlockedOnApproval))
}

func TestApprovalMutationEndpointsRejectNonCanonicalPendingResourceWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		batch bool
	}{
		{name: "single"},
		{name: "batch", batch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			handler := newFileMuxWithCleanup(t, dataRoot)
			workspace, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "non-canonical approval mutation", "run-approval-noncanonical-"+tc.name)
			extraID := common.ApprovalID("approval-extra-" + tc.name)
			repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), common.WorkspaceID(workspace.ID))
			if err := repos.Approvals.Create(ctx, &approvalmodel.Approval{
				ID: extraID, WorkspaceID: common.WorkspaceID(workspace.ID), RunID: common.RunID(started.ID),
				StepID: common.StepID(started.ID + "/extra-approval"), State: common.ApprovalStatePending,
			}); err != nil {
				t.Fatalf("create extra Approval: %v", err)
			}

			var req *http.Request
			if tc.batch {
				body := strings.NewReader(`{"approvalIds":["` + started.BlockingApprovalID + `","` + string(extraID) + `"],"action":"approve"}`)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/resolve-batch?workspaceId="+url.QueryEscape(workspace.ID), body)
			} else {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+string(extraID)+"/approve?workspaceId="+url.QueryEscape(workspace.ID), nil)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("Approval %s status = %d, want 409, body=%s", tc.name, rec.Code, rec.Body.String())
			}
			for _, id := range []common.ApprovalID{common.ApprovalID(started.BlockingApprovalID), extraID} {
				item, err := repos.Approvals.Get(ctx, id)
				if err != nil {
					t.Fatalf("get Approval %s: %v", id, err)
				}
				if item.State != common.ApprovalStatePending {
					t.Fatalf("Approval %s state = %s, want pending after zero-mutation conflict", id, item.State)
				}
			}
		})
	}
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

func TestApprovalApproveEndpointApprovesApprovalWithoutLegacyRunResume(t *testing.T) {
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
	semantic, ok := approved.Approval.ResolutionContext.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	source, _ := semantic.String("source")
	if !ok || intent != "approve_execution" || source != "web" {
		t.Fatalf("approval resolution semantic = %#v, want approve from web", semantic)
	}

	blocked := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnApproval))
	if blocked.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("run = %+v, want legacy approval blocker unchanged", blocked)
	}
}

func TestApprovalApproveEndpointReturnsExactOwnerReadbackAfterResponseLoss(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newFileMuxWithCleanup(t, dataRoot)
	_, started := seedBlockedApprovalRunForHTTP(t, dataRoot, "approval response loss http workspace", "run-approval-response-loss-http")
	body := `{"payload":{"rawInput":"approve","source":"web"}}`
	approve := func() approvalResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("approval response-loss status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		var response approvalResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("unmarshal approval response-loss response: %v", err)
		}
		return response
	}
	first := approve()
	second := approve()
	if first.Approval.ResolutionID == "" || second.Approval.ResolutionID != first.Approval.ResolutionID {
		t.Fatalf("response-loss readbacks = %+v / %+v, want same owner resolution", first.Approval, second.Approval)
	}
}

func TestApprovalRejectEndpointRejectsApprovalWithoutLegacyRunResume(t *testing.T) {
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

	blocked := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnApproval))
	if blocked.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("run = %+v, want legacy approval blocker unchanged", blocked)
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
	waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnApproval))
}

func TestMetricsEndpointReportsExecutionAdmissionCounters(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.metrics.executionAdmissionMetrics.recordCommand(executionAdmissionKindWorkspaceRun, "accepted")
	state.metrics.executionAdmissionMetrics.recordFailure(executionAdmissionKindWorkspaceRun, executionAdmissionReasonSubmissionFailed)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	newMuxWithState(state).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`workorch_execution_admission_commands_total{kind="workspace_run",result="accepted"} 1`,
		`workorch_execution_admission_failures_total{kind="workspace_run",reason="submission_failed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

var runtimeHTTPFixtureSequence atomic.Uint64

func createWorkspaceForHTTP(t *testing.T, handler http.Handler, title string, deliverableDirectory string) projectioncontract.Workspace {
	t.Helper()

	// 通过正式创建入口持久化 owner draft；模板合同存在不代表完成六审查或获得执行准入。
	templateID := fmt.Sprintf("runtime-http-fixture-%d", runtimeHTTPFixtureSequence.Add(1))
	templateBody := `{"id":"` + templateID + `","name":"Runtime HTTP fixture","contextHints":{"acceptanceSlicePolicy":{"schemaVersion":"workorch.acceptance_slice_policy","maxWorkItems":1,"maxFocuses":8,"maxCandidateContextItems":4,"completionPolicy":"all_required_items_accepted","resultPolicy":"slice_result_allowed","groups":[{"id":"http-fixture-all","title":"HTTP fixture acceptance","scope":"http_fixture","priority":10,"maxWorkItems":1,"match":{"all":true}}]},"profileContracts":{"http-fixture-proof":{"evidenceRequirements":[{"id":"*","scope":"dodItem","requiredEvidenceKinds":["validation"],"acceptedStepKinds":["validation/managed_session"],"targetRole":"http_fixture_target","targetDiscoverySources":["workspace_manifest"],"targetSelectorRef":"file/http_fixture_target","targetSelectorOwnerRef":"http-fixture-proof","targetSelectorSubjectRef":"dodItem","validationRecipeId":"http-fixture-validation","requiredCheckRefs":["http-fixture-check"]}]}}}}}`
	postAndExpect(t, handler, "/api/v1/workorch/workspace-templates", templateBody, http.StatusCreated)

	requestBody, err := json.Marshal(workorchmanagementcontract.DefinitionAuthoringCreate{RequestID: templateID, Title: title, Description: "http description", TemplateID: templateID, DeliverableDirectory: deliverableDirectory, Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "验证 HTTP 资源协议"}})
	if err != nil {
		t.Fatal(err)
	}
	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspace-authoring", strings.NewReader(string(requestBody)))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}
	var created definitionAuthoringResponse
	if err := json.Unmarshal(workspaceRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal authoring create response: %v", err)
	}
	if created.Authoring.State != "draft" || created.Authoring.InputHash == "" || created.Authoring.StartupInputHash != "" || created.Authoring.ActivationState != "" {
		t.Fatalf("creation fabricated admission: %+v", created.Authoring)
	}
	var body workspaceResponse
	if err := json.Unmarshal([]byte(getAndExpect(t, handler, "/api/v1/workorch/workspaces/"+created.Authoring.WorkspaceID, http.StatusOK)), &body); err != nil {
		t.Fatalf("unmarshal workspace read response: %v", err)
	}
	if body.Workspace.ID == "" || body.Workspace.Title != title || deliverableDirectory != "" && body.Workspace.DeliverableDirectory != deliverableDirectory || body.Workspace.State != "active" {
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

func seedBlockedDecisionRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appmodelcore.Workspace, appmodelcore.Run) {
	t.Helper()
	workspace, run := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run = seedBlockedDecisionRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID)
	return workspace, run
}

func seedBlockedDecisionRunInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string) appmodelcore.Run {
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
		State:  common.StepStateSucceeded,
		Output: mustServerEventPayload(t, map[string]any{"stage": "blocked_on_decision"}),
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
		Context:      mustServerGovernancePayload(t, map[string]any{"requiredFields": []any{"input"}}),
		State:        common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create decision ticket: %v", err)
	}
	item, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get decision run: %v", err)
	}
	return appmodelcore.Run{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		State:            string(item.State),
		BlockingTicketID: string(item.BlockingTicketID),
	}
}

func seedBlockedDecisionRunWithTicketForHTTP(t *testing.T, dataRoot string, title string, runID string, ticketID string) (appmodelcore.Workspace, appmodelcore.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedDecisionRunWithTicketInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID, ticketID)
	return workspace, run
}

func seedBlockedDecisionRunWithTicketInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string, ticketID string) appmodelcore.Run {
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
		State:  common.StepStateSucceeded,
		Output: mustServerEventPayload(t, map[string]any{"stage": "blocked_on_decision"}),
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
	return appmodelcore.Run{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		State:            string(item.State),
		BlockingTicketID: string(item.BlockingTicketID),
	}
}

func seedBlockedApprovalRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appmodelcore.Workspace, appmodelcore.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedApprovalRunInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID)
	return workspace, run
}

func seedBlockedApprovalRunInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string) appmodelcore.Run {
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
		State:  common.StepStateSucceeded,
		Output: mustServerEventPayload(t, map[string]any{"stage": "blocked_on_approval"}),
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
	return appmodelcore.Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		BlockingApprovalID: string(item.BlockingApprovalID),
	}
}

func seedBlockedApprovalRunWithApprovalForHTTP(t *testing.T, dataRoot string, title string, runID string, approvalID string) (appmodelcore.Workspace, appmodelcore.Run) {
	t.Helper()
	workspace, _ := seedBlockedRunForHTTP(t, dataRoot, title, runID)
	run := seedBlockedApprovalRunWithApprovalInWorkspaceForHTTP(t, dataRoot, workspace.ID, runID, approvalID)
	return workspace, run
}

func seedBlockedApprovalRunWithApprovalInWorkspaceForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string, approvalID string) appmodelcore.Run {
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
		State:  common.StepStateSucceeded,
		Output: mustServerEventPayload(t, map[string]any{"stage": "blocked_on_approval"}),
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
	return appmodelcore.Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		BlockingApprovalID: string(item.BlockingApprovalID),
	}
}

func seedBlockedRunForHTTP(t *testing.T, dataRoot string, title string, runID string) (appmodelcore.Workspace, appmodelcore.Run) {
	t.Helper()
	ctx := context.Background()
	workspace := &workspacemodel.Workspace{
		ID:                   common.WorkspaceID("ws-" + strings.TrimPrefix(runID, "run-")),
		Title:                title,
		DeliverableDirectory: "workspaces/" + runID,
		State:                common.WorkspaceStateActive,
		ActiveRunID:          common.RunID(runID),
	}
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	if err := runScopedRepos.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	runRepo := runScopedRepos.Runs
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          common.RunID(runID),
		WorkspaceID: workspace.ID,
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	run, err := runRepo.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	return appmodelcore.Workspace{ID: string(workspace.ID), Title: workspace.Title, DeliverableDirectory: workspace.DeliverableDirectory, State: string(workspace.State)}, appmodelcore.Run{ID: string(run.ID), WorkspaceID: string(run.WorkspaceID), State: string(run.State)}
}

func readEventRecordsForHTTP(t *testing.T, handler http.Handler, path string) []projectioncontract.EventRecord {
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

func eventRecordsContainTopic(items []projectioncontract.EventRecord, topic string) bool {
	for _, item := range items {
		if item.Topic == topic {
			return true
		}
	}
	return false
}

func timelineContainsPersistedEvent(items []projectioncontract.RuntimeTimelineItem, topic string) bool {
	for _, item := range items {
		if item.Source == "persisted_event" && item.Topic == topic {
			return true
		}
	}
	return false
}

func startRunForHTTP(t *testing.T, handler http.Handler, workspaceID string, runID string) projectioncontract.Run {
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

func waitRunStateForHTTP(t *testing.T, dataRoot string, runID string, wantState string) appmodelcore.Run {
	t.Helper()
	runs := testkit.OpenRepositories("file", dataRoot).Runs
	deadline := time.Now().Add(time.Second)
	var last appmodelcore.Run
	var lastErr error
	for time.Now().Before(deadline) {
		item, err := runs.Get(context.Background(), common.RunID(runID))
		if err == nil {
			last = appserviceruntimecore.MapRun(item)
			if last.State == wantState {
				return last
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
	commandHub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("command hub: %v", err)
	}
	command := decisionevents.EnsureResolvedCommand{
		ResolutionID: common.ResolutionID(resolutionID),
		TicketID:     common.TicketID(ticketID),
		Type:         string(resolutionType),
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "application/workorchd/appservice", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(decisionevents.Contracts(), decisionevents.TopicDecisionCommandEnsureResolved, eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: ticketID}))
	result := commandHub.Send(event)
	if result == nil {
		t.Fatal("EnsureResolvedTicket: missing command result")
	}
	if err := result.Error(); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("EnsureResolvedTicket result: %v", err)
	}
	if _, ok := data.(decisionevents.EnsureResolvedResult); !ok {
		t.Fatalf("EnsureResolvedTicket result type = %T, want EnsureResolvedResult", data)
	}
}

func rejectDecisionViaHTTP(t *testing.T, handler http.Handler, ticketID string) projectioncontract.Decision {
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

func resolveApprovalForHTTP(t *testing.T, dataRoot string, workspaceID string, runID string, approvalID string, reject bool) {
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

	commandHub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("command hub: %v", err)
	}
	topic := approvalevents.TopicApprovalCommandEnsureApproved
	principal := approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice"}
	var command any = approvalevents.EnsureApprovedCommand{ApprovalID: common.ApprovalID(approvalID), WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), Principal: principal}
	if reject {
		topic = approvalevents.TopicApprovalCommandEnsureRejected
		command = approvalevents.EnsureRejectedCommand{ApprovalID: common.ApprovalID(approvalID), WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), Principal: principal}
	}
	event := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(approvalevents.Contracts(), topic, eventcontract.LaneScope{SubjectType: "approval", SubjectID: approvalID, WorkspaceID: workspaceID, RunID: runID}))
	result := commandHub.Send(event)
	if result == nil {
		t.Fatal("resolve approval: missing command result")
	}
	if err := result.Error(); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("resolve approval result: %v", err)
	}
	if reject {
		if _, ok := data.(approvalevents.EnsureRejectedResult); !ok {
			t.Fatalf("resolve approval result type = %T, want EnsureRejectedResult", data)
		}
	} else if _, ok := data.(approvalevents.EnsureApprovedResult); !ok {
		t.Fatalf("resolve approval result type = %T, want EnsureApprovedResult", data)
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
