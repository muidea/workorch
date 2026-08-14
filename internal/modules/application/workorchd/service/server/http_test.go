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

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, Revision: command.ExpectedRevision + 1, State: common.WorkspaceStateActive, DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(command.Selection)}}, nil)
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

func skipObsoleteSynchronousExecutionEndpointTest(t *testing.T) {
	t.Helper()
	t.Skip("execution-driving REST endpoints are asynchronous EventHub scheduler commands and return accepted; final run state is verified through scheduler/projection tests")
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
	if state.sharedSession != nil {
		_ = state.sharedSession.Close(context.Background())
		state.sharedSession = nil
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
	state.configureSessionBackedOperations(session)
	attachTestEventNotifications(state)
	state.workspaceRunScheduler = directWorkspaceRunScheduler{state: state}
}

// directWorkspaceRunScheduler is a test-only adapter. Production HTTP code
// receives the application Biz port instead of directly using EventHub.
type directWorkspaceRunScheduler struct{ state *serverState }

func (p directWorkspaceRunScheduler) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if p.state == nil || p.state.sharedSession == nil || command.WorkspaceID == "" || command.RequestID == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	// This test adapter follows the production boundary exactly: compile an
	// immutable restart bootstrap if needed, then send one Workspace-owner
	// intent. It must not activate, continue, supersede, or select a Run.
	intent := workspaceevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: command.WorkspaceID,
		RequestID:   command.RequestID,
		Priority:    command.Priority,
	}
	switch command.Intent {
	case "resume":
		intent.Kind = runtimecontract.RuntimeExecutionIntentResume
	case "restart":
		intent.Kind = runtimecontract.RuntimeExecutionIntentRestart
		bootstrap, err := appservice.CompileWorkspaceExecutionRestartBootstrapWithSession(ctx, p.state.sharedSession.Borrow(), string(command.WorkspaceID), appmodelcore.ActivateWorkspaceExecutionCommand{})
		if err != nil {
			p.state.projectRejectedSchedulerCommand(command, err)
			return workorchdevents.WorkspaceRunRequestedResult{}, err
		}
		intent.Bootstrap = bootstrap
	default:
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	hub, err := p.state.sharedSession.RequiredCommandHub()
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
		p.state.projectRejectedSchedulerCommand(command, err)
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		p.state.projectRejectedSchedulerCommand(command, resultErr)
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

type blockingWorkspaceRunScheduler struct {
	state   *serverState
	started chan struct{}
	release chan struct{}
}

type failingWorkspaceRunScheduler struct{ err error }

func (p failingWorkspaceRunScheduler) SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	return workorchdevents.WorkspaceRunRequestedResult{}, p.err
}

// acceptedRestartWorkspaceRunScheduler isolates HTTP projection tests from
// restart bootstrap and provider-catalog work. The HTTP adapter only consumes
// the Workspace owner's typed admission fact.
type acceptedRestartWorkspaceRunScheduler struct{}

func (acceptedRestartWorkspaceRunScheduler) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
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

func (p *blockingWorkspaceRunScheduler) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
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

func TestReadyzIncludesSchedulerDiagnostics(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.schedulerDiagnostics.recordWatchdogTimerRegistration()
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    "ws-readyz-diagnostics",
		RunID:          "run-readyz-diagnostics",
		Kind:           schedulerKindRunContinuation,
		State:          schedulerDiagnosticStateFailed,
		Reason:         schedulerReasonOwnerCommandFailed,
		Detail:         "owner command returned an explicit failure",
		Recoverability: schedulerRecoverabilityManualRetry,
	})
	handler := newMuxWithState(state)
	waitForReconciledWorkspaceProjection(t, state)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

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
		latest.Kind != schedulerKindRunContinuation ||
		latest.State != schedulerDiagnosticStateFailed ||
		latest.Reason != schedulerReasonOwnerCommandFailed {
		t.Fatalf("latest diagnostics = %+v, want recorded run continuation failure", latest)
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
	if state.sharedSession == nil {
		t.Fatal("server state runtime session = nil")
	}
	if err := state.validate(ctx, "", ""); err != nil {
		t.Fatalf("validate runtime session: %v", err)
	}
}

func TestConfigurationEndpointReturnsRedactedAIProxyConfiguration(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	fixture := workorchconfigurationcontract.ServiceConfiguration{
		Server:     workorchconfig.ServerConfig{Addr: ":8080", MaxConcurrentRuns: 1},
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
maxConcurrentRuns = 1

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
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId"},
		{method: http.MethodGet, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action"},
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
		{method: http.MethodPost, pattern: "/api/v1/workorch/workspaces/:workspaceId/runs"},
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
	if id != "projection:ws-projection:42" {
		t.Fatalf("sse id = %q, want projection:ws-projection:42", id)
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

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-replay",
		Payload:     recordpayload.WorkspacePayloadData{Title: "first", DeliverableDirectory: "/tmp/first", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish first event: %v", err)
	}
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-stream-replay",
		RunID:       "run-replay",
		Payload:     recordpayload.RunPayloadData{State: common.RunStateActive}.EventPayload().Map(),
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

	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-stream-query-replay",
		Payload:     recordpayload.WorkspacePayloadData{Title: "first", DeliverableDirectory: "/tmp/first", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish first event: %v", err)
	}
	firstID, first := readSSEEnvelope(t, lines, readErr)
	if firstID == "" || first.Topic != workspaceevents.TopicWorkspaceStateChanged {
		t.Fatalf("first replay baseline = %q %+v", firstID, first)
	}
	if err := publishServerTestEventRecord(context.Background(), testStateCommandHub(state), streamEvent{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-stream-query-replay",
		RunID:       "run-query-replay",
		Payload:     recordpayload.RunPayloadData{State: common.RunStateActive}.EventPayload().Map(),
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
	if firstID != "projection:ws-projection-replay:20" || first.StreamRevision != 20 {
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
	if id != "projection:ws-projection-replay:21" || dto.ID != id {
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

func TestNativeBusinessRoutesDoNotExposeEventHub(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
	/* Legacy synchronous aggregate fixture retained for historical route
	coverage only. Runtime route contracts are covered by focused tests. */
	/*
		state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
		// HTTP handlers must invoke the session-backed use cases without exposing
		// the application EventHub through their request context. The fixture's
		// existing assertions compare received context values to this nil sentinel.
		state.commandHub = nil
		state.validate = func(context.Context, string, string) error { return nil }
		state.listWorkspaces = func(ctx context.Context, _, _ string) ([]appmodelcore.Workspace, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace list event hub = %#v, want injected hub", got)
			}
			return []appmodelcore.Workspace{{ID: "ws-native-read", Title: "native read"}}, nil
		}
		state.listRuns = func(ctx context.Context, _, _, workspaceID string) ([]appmodelcore.Run, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run list event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" {
				t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
			}
			return []appmodelcore.Run{{ID: "run-native-read", WorkspaceID: workspaceID, State: "created"}}, nil
		}
		state.readWorkspace = func(ctx context.Context, _, _, workspaceID string) (appmodelcore.Workspace, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace get event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" {
				t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
			}
			return appmodelcore.Workspace{ID: workspaceID, Title: "native read", State: "active"}, nil
		}
		state.readWorkspaceDetailSnapshot = func(ctx context.Context, _, _, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace snapshot event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" {
				t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
			}
			return readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: workspaceID}}, nil
		}
		state.readWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace soul bindings read event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" {
				t.Fatalf("workspaceID = %q, want ws-native-read", workspaceID)
			}
			return workorchmanagementcontract.WorkspaceSoulBindings{
				WorkspaceID:   workspaceID,
				DefaultSoulID: "soul-native",
				Bindings: map[string]string{
					"planning": "soul-native",
				},
			}, nil
		}
		state.updateWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string, cmd workorchmanagementcontract.WorkspaceSoulBindings) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace soul bindings update event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" || cmd.DefaultSoulID != "soul-native" || cmd.Bindings["execution"] != "soul-native-execution" {
				t.Fatalf("soul bindings input = %+v for workspace %q, want native soul bindings", cmd, workspaceID)
			}
			cmd.WorkspaceID = workspaceID
			return cmd, nil
		}
		state.checkGoalDoDQuality = func(ctx context.Context, _, _ string, cmd qualitycore.Command) (qualitycore.Check, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("goal/dod quality check event hub = %#v, want injected hub", got)
			}
			if cmd.WorkspaceID != "ws-native-read" || cmd.GoalID != "goal-native" || cmd.DoDID != "dod-native" {
				t.Fatalf("quality check cmd = %+v, want ws-native-read/goal-native/dod-native", cmd)
			}
			return qualitycore.Check{
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
		state.readRun = func(ctx context.Context, _, _, runID string) (appmodelcore.Run, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run get event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: "created"}, nil
		}
		state.readRunSnapshot = func(ctx context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
			if runID == "run-native-read" {
				got := appservice.CommandHubFromContext(ctx)
				if got != nil && got != state.commandHub {
					t.Fatalf("run snapshot event hub = %#v, want injected hub", got)
				}
				state := "created"
				if got == nil {
					state = "completed"
				}
				return readmodelcore.RunSnapshotState{Run: appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: state}}, nil
			}
			if runID == "run-native-activation" {
				return readmodelcore.RunSnapshotState{Run: appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}}, nil
			}
			t.Fatalf("runID = %q, want run-native-read or run-native-activation", runID)
			return readmodelcore.RunSnapshotState{}, nil
		}
		state.listRunSteps = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.StepPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run steps event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "step-native-1" || query.Limit != 2 {
				t.Fatalf("run steps query = %+v, want run-native-read cursor step-native-1 limit 2", query)
			}
			return readmodelcore.StepPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "step-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunToolCalls = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ToolCallPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run tool calls event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "tool-native-1" || query.Limit != 2 {
				t.Fatalf("run tool calls query = %+v, want run-native-read cursor tool-native-1 limit 2", query)
			}
			return readmodelcore.ToolCallPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "tool-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunRuntimeCheckpoints = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.RuntimeCheckpointPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run runtime checkpoints event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "checkpoint-native-1" || query.Limit != 2 {
				t.Fatalf("run runtime checkpoints query = %+v, want run-native-read cursor checkpoint-native-1 limit 2", query)
			}
			return readmodelcore.RuntimeCheckpointPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "checkpoint-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ResultPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run results event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "result-native-1" || query.Limit != 2 {
				t.Fatalf("run results query = %+v, want run-native-read cursor result-native-1 limit 2", query)
			}
			return readmodelcore.ResultPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "result-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunPatches = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.PatchPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run patches event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "patch-native-1" || query.Limit != 2 {
				t.Fatalf("run patches query = %+v, want run-native-read cursor patch-native-1 limit 2", query)
			}
			return readmodelcore.PatchPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "patch-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunValidationResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ValidationResultPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run validation results event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "validation-native-1" || query.Limit != 2 {
				t.Fatalf("run validation results query = %+v, want run-native-read cursor validation-native-1 limit 2", query)
			}
			return readmodelcore.ValidationResultPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "validation-native-2",
				HasMore:           true,
			}, nil
		}
		state.listRunArtifacts = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ArtifactPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run artifacts event hub = %#v, want injected hub", got)
			}
			if query.RunID != "run-native-read" || query.CursorAfterID != "artifact-native-1" || query.Limit != 2 {
				t.Fatalf("run artifacts query = %+v, want run-native-read cursor artifact-native-1 limit 2", query)
			}
			return readmodelcore.ArtifactPage{
				RunID:             query.RunID,
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "artifact-native-2",
				HasMore:           true,
			}, nil
		}
		state.readRunStatus = func(ctx context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run status event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return projectioncontract.RunStatus{ID: runID, WorkspaceID: "ws-native-read", State: "created"}, nil
		}
		state.readRunObservability = func(ctx context.Context, _, _, runID string) (readmodelcore.RunObservabilityState, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run observability event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return readmodelcore.RunObservabilityState{Run: appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: "created"}}, nil
		}
		state.readRunTaskProgress = func(ctx context.Context, _, _, runID string) (projectioncontract.TaskProgress, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run task progress event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return projectioncontract.TaskProgress{WorkspaceID: "ws-native-read", RunID: runID, State: "running", Recovery: projectioncontract.RecoveryPlan{Mode: "resume"}}, nil
		}
		state.readRunContextBundle = func(ctx context.Context, _, _, runID string) (projectioncontract.ContextBundle, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run context bundle event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return projectioncontract.ContextBundle{WorkspaceID: "ws-native-read", RunID: runID, Mode: "run_recovery", Recovery: projectioncontract.RecoveryPlan{Mode: "resume"}}, nil
		}
		state.readRunTaskDecompositions = func(ctx context.Context, _, _, runID string) ([]projectioncontract.TaskDecomposition, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run task decomposition event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return []projectioncontract.TaskDecomposition{{ID: "task-native", WorkspaceID: "ws-native-read", RunID: runID, State: "running"}}, nil
		}
		state.listDecisions = func(ctx context.Context, _, _, workspaceID, runID string) ([]projectioncontract.Decision, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("decision list event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" || runID != "run-native-read" {
				t.Fatalf("decision filters = %q/%q, want ws-native-read/run-native-read", workspaceID, runID)
			}
			return []projectioncontract.Decision{{ID: "ticket-native-read", WorkspaceID: workspaceID, RunID: runID, State: "open"}}, nil
		}
		state.listDecisionPage = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.DecisionPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("decision page event hub = %#v, want injected hub", got)
			}
			if query.WorkspaceID != "ws-native-read" || query.RunID != "run-native-read" || query.CursorAfterID != "ticket-native-cursor" || query.Limit != 2 {
				t.Fatalf("decision page query = %+v, want ws-native-read/run-native-read cursor ticket-native-cursor limit 2", query)
			}
			return appmodelcore.DecisionPage{
				WorkspaceID:       query.WorkspaceID,
				RunID:             query.RunID,
				Decisions:         []projectioncontract.Decision{{ID: "ticket-native-read", WorkspaceID: query.WorkspaceID, RunID: query.RunID, State: "open"}},
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "ticket-native-read",
			}, nil
		}
		state.readDecision = func(ctx context.Context, _, _, ticketID string) (projectioncontract.Decision, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("decision get event hub = %#v, want injected hub", got)
			}
			if ticketID != "ticket-native-read" {
				t.Fatalf("ticketID = %q, want ticket-native-read", ticketID)
			}
			return projectioncontract.Decision{ID: ticketID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "open"}, nil
		}
		state.listApprovals = func(ctx context.Context, _, _, workspaceID, runID string) ([]projectioncontract.Approval, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval list event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" || runID != "run-native-read" {
				t.Fatalf("approval filters = %q/%q, want ws-native-read/run-native-read", workspaceID, runID)
			}
			return []projectioncontract.Approval{{ID: "approval-native-read", WorkspaceID: workspaceID, RunID: runID, State: "pending"}}, nil
		}
		state.listApprovalPage = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.ApprovalPage, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval page event hub = %#v, want injected hub", got)
			}
			if query.WorkspaceID != "ws-native-read" || query.RunID != "run-native-read" || query.CursorAfterID != "approval-native-cursor" || query.Limit != 2 {
				t.Fatalf("approval page query = %+v, want ws-native-read/run-native-read cursor approval-native-cursor limit 2", query)
			}
			return appmodelcore.ApprovalPage{
				WorkspaceID:       query.WorkspaceID,
				RunID:             query.RunID,
				Approvals:         []projectioncontract.Approval{{ID: "approval-native-read", WorkspaceID: query.WorkspaceID, RunID: query.RunID, State: "pending"}},
				Limit:             query.Limit,
				CursorAfterID:     query.CursorAfterID,
				NextCursorAfterID: "approval-native-read",
			}, nil
		}
		state.readApproval = func(ctx context.Context, _, _, approvalID string) (projectioncontract.Approval, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval get event hub = %#v, want injected hub", got)
			}
			if approvalID != "approval-native-read" {
				t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
			}
			return projectioncontract.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "pending"}, nil
		}
		state.createWorkspace = func(ctx context.Context, _, _ string, cmd workorchmanagementcontract.WorkspaceCreateRequest) (appmodelcore.Workspace, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace create event hub = %#v, want injected hub", got)
			}
			if cmd.Title != "native write" || cmd.Description != "native description" || cmd.DeliverableDirectory != "workspaces/native-write" || cmd.TemplateID != "template-native" {
				t.Fatalf("workspace create input = %q/%q/%q/%q, want native write/native description/workspaces/native-write/template-native", cmd.Title, cmd.Description, cmd.DeliverableDirectory, cmd.TemplateID)
			}
			return appmodelcore.Workspace{ID: "ws-native-write", Title: cmd.Title, Description: cmd.Description, DeliverableDirectory: cmd.DeliverableDirectory, State: "active"}, nil
		}
		state.deleteWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace delete event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" || !cmd.Purge {
				t.Fatalf("workspace delete input = %q/%+v, want ws-native-read purge", workspaceID, cmd)
			}
			return nil
		}
		state.startRun = func(ctx context.Context, _, _, runID string) (appmodelcore.Run, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run start event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}, nil
		}
		state.resumeRun = func(ctx context.Context, _, _, runID string) (appmodelcore.Run, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("run resume event hub = %#v, want injected hub", got)
			}
			if runID != "run-native-read" {
				t.Fatalf("runID = %q, want run-native-read", runID)
			}
			return appmodelcore.Run{ID: runID, WorkspaceID: "ws-native-read", State: "completed"}, nil
		}
		state.resumeWorkspaceExecution = func(ctx context.Context, _, _, workspaceID string, cmd appmodelcore.ActivateWorkspaceExecutionCommand) (readmodelcore.WorkspaceExecutionActivationState, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("workspace activation event hub = %#v, want injected hub", got)
			}
			if workspaceID != "ws-native-read" ||
				cmd.GoalID != "goal-native" ||
				cmd.DoDID != "dod-native" ||
				cmd.SoulID != "soul-native" ||
				cmd.SoulBindings["validation"] != "soul-native-validation" ||
				cmd.DialogLanguage != "zh-CN" ||
				cmd.Priority != 9 ||
				!stringSliceEqual(cmd.SkillIDs, []string{"skill-native"}) ||
				!stringSliceEqual(cmd.MCPServerIDs, []string{"mcp-native"}) ||
				!stringSliceEqual(cmd.MemoryIDs, []string{"memory-native"}) {
				t.Fatalf("workspace activation input = %+v for workspace %q, want full context selection", cmd, workspaceID)
			}
			return readmodelcore.WorkspaceExecutionActivationState{
				Workspace: appmodelcore.Workspace{ID: workspaceID, ActiveRunID: "run-native-activation"},
				Run:       appmodelcore.Run{ID: "run-native-activation", WorkspaceID: workspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID, State: "completed"},
				Snapshot:  readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: workspaceID}},
			}, nil
		}
		state.resolveDecision = func(ctx context.Context, _, _, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (projectioncontract.Decision, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("decision resolve event hub = %#v, want injected hub", got)
			}
			if ticketID != "ticket-native-read" {
				t.Fatalf("ticketID = %q, want ticket-native-read", ticketID)
			}
			return projectioncontract.Decision{ID: ticketID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "resolved", ResolutionType: cmd.ResolutionType}, nil
		}
		state.batchResolveDecisions = func(ctx context.Context, _, _ string, cmd appmodelcore.BatchResolveDecisionCommand) ([]projectioncontract.Decision, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("decision batch resolve event hub = %#v, want injected hub", got)
			}
			if !stringSliceEqual(cmd.TicketIDs, []string{"ticket-native-read"}) || cmd.ResolutionType != decisioncommon.ResolutionTypeReplan {
				t.Fatalf("batch resolve cmd = %+v, want ticket-native-read replan", cmd)
			}
			return []projectioncontract.Decision{{ID: "ticket-native-read", WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "resolved", ResolutionType: cmd.ResolutionType}}, nil
		}
		state.approveApproval = func(ctx context.Context, _, _, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval approve event hub = %#v, want injected hub", got)
			}
			if approvalID != "approval-native-read" {
				t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
			}
			if cmd.Action != "approve" {
				t.Fatalf("approval approve action = %q, want approve", cmd.Action)
			}
			return projectioncontract.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "approved"}, nil
		}
		state.rejectApproval = func(ctx context.Context, _, _, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval reject event hub = %#v, want injected hub", got)
			}
			if approvalID != "approval-native-read" {
				t.Fatalf("approvalID = %q, want approval-native-read", approvalID)
			}
			if cmd.Action != "reject" {
				t.Fatalf("approval reject action = %q, want reject", cmd.Action)
			}
			return projectioncontract.Approval{ID: approvalID, WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "rejected"}, nil
		}
		state.batchResolveApprovals = func(ctx context.Context, _, _ string, cmd appmodelcore.BatchResolveApprovalCommand) ([]projectioncontract.Approval, error) {
			if got := appservice.CommandHubFromContext(ctx); got != state.commandHub {
				t.Fatalf("approval batch resolve event hub = %#v, want injected hub", got)
			}
			if !stringSliceEqual(cmd.ApprovalIDs, []string{"approval-native-read"}) || cmd.Action != "approve" {
				t.Fatalf("batch approval cmd = %+v, want approval-native-read approve", cmd)
			}
			return []projectioncontract.Approval{{ID: "approval-native-read", WorkspaceID: "ws-native-read", RunID: "run-native-read", State: "approved"}}, nil
		}
		handler := newMuxWithState(state)

		for _, tc := range []struct {
			method string
			path   string
			body   string
			status int
		}{
			{method: http.MethodGet, path: "/api/v1/workorch/workspaces", status: http.StatusOK},
			{method: http.MethodPost, path: "/api/v1/workorch/workspaces", body: `{"title":"native write","description":"native description","deliverableDirectory":"workspaces/native-write","templateId":"template-native"}`, status: http.StatusCreated},
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
			{method: http.MethodPost, path: "/api/v1/workorch/workspaces/ws-native-read/execution-activations", body: `{"goalId":"goal-native","doDId":"dod-native","llmTarget":{"providerId":"ai-proxy","modelId":"gpt-4o"},"soulId":"soul-native","soulBindings":{"validation":"soul-native-validation"},"skillIds":["skill-native"],"mcpServerIds":["mcp-native"],"memoryIds":["memory-native"],"dialogLanguage":"zh-CN","priority":9}`, status: http.StatusOK},
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
	*/
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.listWorkspaceTemplates = func(context.Context, string, string) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
		return []workorchmanagementcontract.WorkspaceTemplate{{
			ID:       "generic-template",
			Name:     "Generic Template",
			GoalText: "Template goal",
			DoDItems: []string{"Template DoD"},
		}}, nil
	}
	generateCalls := 0
	state.generateWorkspaceSetupDraft = func(_ context.Context, _, _ string, cmd setupdraftcore.GenerationCommand) (setupdraftcore.Draft, error) {
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
		return setupdraftcore.Draft{
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.listWorkspaceTemplates = func(context.Context, string, string) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
		return []workorchmanagementcontract.WorkspaceTemplate{{
			ID:       "generic-template",
			Name:     "Generic Template",
			GoalText: "Template goal",
			DoDItems: []string{"Template DoD"},
		}}, nil
	}
	state.generateWorkspaceSetupDraft = func(_ context.Context, _, _ string, cmd setupdraftcore.GenerationCommand) (setupdraftcore.Draft, error) {
		if cmd.TemplateID != "generic-template" {
			t.Fatalf("draft command template = %q, want restored template", cmd.TemplateID)
		}
		if cmd.CurrentDraft.GoalText != "Template goal" || !reflect.DeepEqual(cmd.CurrentDraft.DoDItems, []string{"Template DoD"}) {
			t.Fatalf("draft command current draft = %+v, want restored template draft", cmd.CurrentDraft)
		}
		if len(cmd.CurrentDraft.ClarifyingQuestions) != 0 {
			t.Fatalf("draft command current draft = %+v, want stale clarifications cleared before revision", cmd.CurrentDraft)
		}
		return setupdraftcore.Draft{ClarifyingQuestions: []string{"Need more details"}}, nil
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

func TestDetailContentEndpointsRequireExplicitRefsBeforeReading(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	var artifactReads atomic.Int32
	var rawReads atomic.Int32
	state.readArtifactContent = func(context.Context, string, string, string) (contentcore.ArtifactContent, error) {
		artifactReads.Add(1)
		return contentcore.ArtifactContent{}, errors.New("artifact reader should not be called without uri")
	}
	state.readRuntimeRawContent = func(context.Context, string, string, string) (contentcore.RuntimeRawContent, error) {
		rawReads.Add(1)
		return contentcore.RuntimeRawContent{}, errors.New("runtime raw reader should not be called without ref")
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
	session := newOwnerRuntimeSession(t, "file", dataRoot)
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
	state.configureSessionBackedOperations(session)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.readWorkspace = func(context.Context, string, string, string) (appmodelcore.Workspace, error) {
		return appmodelcore.Workspace{
			ID:               string(workspaceID),
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-get-prompt-owner-state",
			BlockingTicketID: "ticket-get-prompt-owner-state",
		}, nil
	}
	handler := newMuxWithState(state)
	// The test installs a deliberately stale projection fixture. Wait for the
	// initial asynchronous rebuild to finish first, otherwise it can race and
	// overwrite that fixture before the read route is exercised.
	waitForCondition(t, time.Second, func() bool {
		return state.workspaceProjectionStatus().State == workspaceProjectionStateReady
	})
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
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

func TestWorkspaceCreateEndpointRejectsRawRuntimeContractDeclaration(t *testing.T) {
	handler := newTestMux(t, Config{Backend: "memory"})

	for _, body := range []string{
		`{"title":"raw hints","contextHints":{"profileContracts":{}}}`,
		`{"title":"raw policies","evidencePolicies":[{"id":"recipe"}]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("raw workspace contract status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
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

func TestRunCreateEndpointReturnsCreatedRun(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
	if body.Run.ID != "run-http" || body.Run.WorkspaceID != workspace.ID || body.Run.State != string(common.RunStatePending) {
		t.Fatalf("created run = %+v, want run-http/%s/pending", body.Run, workspace.ID)
	}
	if body.Run.GoalID != goalID || body.Run.DoDID != dodID {
		t.Fatalf("created run goal/dod = %+v, want %s/%s", body.Run, goalID, dodID)
	}
}

func TestRunListAndGetEndpointsReturnPersistedRuns(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
		WorkspaceID: "ws-status-owner-event",
		RunID:       "run-status-owner-event",
		State:       common.RunStateActive,
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
	if body.Snapshot.Workspace.State != string(common.WorkspaceStateBlockedOnApproval) || body.Snapshot.Workspace.ActiveRunID != approvalRun.ID {
		t.Fatalf("workspace projection = %+v, want blocked_on_approval active run %s", body.Snapshot.Workspace, approvalRun.ID)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "file", DataRoot: dataRoot}))
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
		if err := repos.Runs.Create(ctx, &runmodel.Run{
			ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive,
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
			Version:     recordpayload.WorkorchEventVersion,
			Topic:       "/run/llm_usage/recorded",
			OccurredAt:  fmt.Sprintf("2026-06-12T00:%02d:00Z", idx),
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      common.StepID(fmt.Sprintf("%s/step-%02d", runID, idx)),
			Payload: mustServerEventPayload(t, map[string]any{
				recordpayload.PayloadKeyWorkspaceID:  string(workspaceID),
				recordpayload.PayloadKeyRunID:        string(runID),
				recordpayload.PayloadKeyStepID:       fmt.Sprintf("%s/step-%02d", runID, idx),
				recordpayload.PayloadKeyProviderID:   "provider-compact",
				recordpayload.PayloadKeyProvider:     "compatible",
				recordpayload.PayloadKeyModel:        "model-compact",
				recordpayload.PayloadKeyOperation:    "responses",
				recordpayload.PayloadKeyInputTokens:  10 + idx,
				recordpayload.PayloadKeyOutputTokens: 1,
				recordpayload.PayloadKeyTotalTokens:  11 + idx,
				recordpayload.PayloadKeyStatus:       "completed",
			}),
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
	state.workspaceProjection.UpsertWorkspace(appmodelcore.Workspace{
		ID:          string(workspaceID),
		State:       string(common.WorkspaceStateActive),
		ActiveRunID: "run-explicit-compact-detail",
	})
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
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
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
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
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-detail-prompt-same-source",
		},
	})
	state.readWorkspaceDetailSnapshot = func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error) {
		return readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{
				ID:               string(workspaceID),
				State:            string(common.WorkspaceStateActive),
				ActiveRunID:      "run-detail-prompt-same-source",
				BlockingTicketID: "ticket-detail-prompt-same-source",
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
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

func TestRunCreateEndpointReturnsConflictForDuplicateID(t *testing.T) {
	skipObsoleteSynchronousExecutionEndpointTest(t)
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
	_, _ = defineGoalAndDoDForHTTP(t, handler, workspace.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-http-activation",
		"reason":"http_test",
		"source":"web"
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
		body["reason"] != "http_test" {
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
		snapshot.RunCommand.Source != "web" {
		t.Fatalf("projected run command = %+v, want workspace-scoped HTTP request metadata", snapshot.RunCommand)
	}
	var accepted readmodelcore.WorkspaceSnapshotState
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		item, _, readErr := state.readProjectedWorkspaceSnapshot(workspace.ID)
		if readErr == nil {
			accepted = item
			if item.RunCommand != nil && item.RunCommand.State == "rejected" && item.RunCommand.Disposition == "restart_required" && item.RunCommand.RequestID == "request-http-activation" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if accepted.RunCommand == nil || accepted.RunCommand.State != "rejected" || accepted.RunCommand.Disposition != "restart_required" || accepted.RunCommand.RequestID != "request-http-activation" {
		t.Fatalf("owner outcome projection = %+v diagnostics=%+v", accepted.RunCommand, accepted.SchedulerDiagnostics)
	}
	runs, err := state.listRuns(context.Background(), state.config.Backend, state.config.DataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("list runs after resume without active run: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("resume without active run created runs = %+v, want restart-required admission without implicit replacement", runs)
	}
}

func TestWorkorchDActivationHTTPReturnsWhileSchedulerIsStillRunning(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "async activation", "workspaces/async-activation")
	scheduler := &blockingWorkspaceRunScheduler{
		state: state, started: make(chan struct{}), release: make(chan struct{}),
	}
	state.workspaceRunScheduler = scheduler

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
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("activation HTTP handler waited for scheduler completion")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("activation status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	select {
	case <-scheduler.started:
	case <-time.After(time.Second):
		t.Fatal("background scheduler did not start")
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "queued" || snapshot.RunCommand.RequestID != "request-http-async-activation" || snapshot.RunCommand.QueuedAt == "" {
		t.Fatalf("queued run command = %+v", snapshot.RunCommand)
	}

	close(scheduler.release)
	waitForCondition(t, time.Second, func() bool {
		latest, _, readErr := state.readProjectedWorkspaceSnapshot(workspace.ID)
		return readErr == nil && latest.RunCommand != nil && latest.RunCommand.State == "accepted"
	})
	accepted, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RunCommand.QueuedAt == "" || accepted.RunCommand.AcceptedAt == "" {
		t.Fatalf("accepted run command = %+v, want queued and accepted timestamps", accepted.RunCommand)
	}
}

func TestWorkorchDActivationHTTPProjectsBackgroundSchedulerFailure(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "failed async activation", "workspaces/failed-async-activation")
	state.workspaceRunScheduler = failingWorkspaceRunScheduler{err: errors.New("scheduler submission failed")}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-http-async-failure",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("activation status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := state.waitBackground(waitCtx); err != nil {
		t.Fatalf("wait for scheduler failure projection: %v", err)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand.RequestID != "request-http-async-failure" || snapshot.RunCommand.QueuedAt == "" || snapshot.RunCommand.RejectedAt == "" || !strings.Contains(snapshot.RunCommand.Error, "scheduler submission failed") {
		t.Fatalf("rejected run command = %+v", snapshot.RunCommand)
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
	_, _ = defineGoalAndDoDForHTTP(t, handler, workspace.ID)
	state.workspaceRunScheduler = acceptedRestartWorkspaceRunScheduler{}

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
		snapshot.RunCommand.IdempotencyKey != "restart/"+workspace.ID ||
		snapshot.RunCommand.Reason != "operator_restart" ||
		snapshot.RunCommand.Source != "web" || snapshot.RunCommand.Error != "" {
		t.Fatalf("projected run command = %+v, want accepted HTTP restart metadata", snapshot.RunCommand)
	}
}

func TestWorkorchDActivationEnqueueFailureProjectsRejected(t *testing.T) {
	routine := &recordingBackgroundRoutine{err: errors.New("background unavailable")}
	state := newIsolatedTestServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), routine)
	state.workspaceRunScheduler = directWorkspaceRunScheduler{state: state}
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-enqueue-failure", State: "active"},
	})
	handler := newMuxWithState(state)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-enqueue-failure/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"request-enqueue-failure",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("enqueue failure status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	snapshot, _, readErr := state.readProjectedWorkspaceSnapshot("ws-enqueue-failure")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "rejected" || snapshot.RunCommand.RequestID != "request-enqueue-failure" || snapshot.RunCommand.QueuedAt == "" || snapshot.RunCommand.Error == "" {
		t.Fatalf("rejected run command = %+v", snapshot.RunCommand)
	}
}

func TestWorkorchDSchedulerCommandRejectsRecoverIntent(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newIsolatedTestServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), routine)
	state.workspaceRunScheduler = directWorkspaceRunScheduler{state: state}
	const workspaceID = "ws-scheduler-reject-recover"
	state.workspaceProjection.UpsertSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: workspaceID, State: "active"},
	})

	_, err := state.publishWorkorchDSchedulerCommand(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID(workspaceID),
		Intent:      "recover",
		RequestID:   "request-recover-rejected",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("publish recover scheduler command error = %v, want invalid argument", err)
	}
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("projected run command = %+v, want no projection for adapter-level invalid intent", snapshot.RunCommand)
	}
}

func TestWorkspaceActivationFailureIsVisibleWithoutRun(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	handler := newMuxWithState(state)
	workspace := createWorkspaceForHTTP(t, handler, "activation diagnostic", "workspaces/activation-diagnostic")
	const requestID = "request-activation-diagnostic"
	state.workspaceRunScheduler = failingWorkspaceRunScheduler{err: errors.New("activation preflight: provider target is not ready")}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspace.ID+"/execution-activations", strings.NewReader(`{
		"intent":"resume",
		"requestId":"`+requestID+`",
		"source":"web"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("activation status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	var snapshot readmodelcore.WorkspaceSnapshotState
	waitForCondition(t, time.Second, func() bool {
		item, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
		if err != nil {
			return false
		}
		snapshot = item
		return snapshot.RunCommand != nil && snapshot.RunCommand.State == "rejected" && snapshot.SchedulerDiagnostics != nil && snapshot.SchedulerDiagnostics.LatestContinuation != nil
	})
	snapshot, _, err := state.readProjectedWorkspaceSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("read projected workspace snapshot: %v", err)
	}
	if snapshot.Run != nil {
		t.Fatalf("run = %+v, want no Run when activation preflight fails", snapshot.Run)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "rejected" || snapshot.RunCommand.RequestID != requestID || !strings.Contains(snapshot.RunCommand.Error, "provider target is not ready") {
		t.Fatalf("run command = %+v, want rejected command with exact activation error", snapshot.RunCommand)
	}
	if snapshot.SchedulerDiagnostics == nil || snapshot.SchedulerDiagnostics.LatestContinuation == nil {
		t.Fatalf("scheduler diagnostics = %+v, want workspace activation failure", snapshot.SchedulerDiagnostics)
	}
	latest := snapshot.SchedulerDiagnostics.LatestContinuation
	if latest.Kind != schedulerKindWorkspaceActivation || latest.State != schedulerDiagnosticStateFailed || latest.Reason != schedulerReasonWorkspaceActivationFailed || !strings.Contains(latest.Detail, "provider target is not ready") {
		t.Fatalf("latest scheduler diagnostic = %+v, want classified exact activation failure", latest)
	}
}

func TestWorkorchDSchedulerCommandRequiresApplicationScheduler(t *testing.T) {
	state := newIsolatedTestServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), &recordingBackgroundRoutine{})

	_, err := state.publishWorkorchDSchedulerCommand(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-missing-scheduler",
		Intent:      "resume",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("publish scheduler command error = %v, want invalid argument for missing application scheduler", err)
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
	semantic, ok := approved.Approval.Context.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	source, _ := semantic.String("source")
	if !ok || intent != "approve_execution" || source != "web" {
		t.Fatalf("approval governance semantic = %#v, want approve from web", semantic)
	}

	blocked := waitRunStateForHTTP(t, dataRoot, runID, string(common.RunStateBlockedOnApproval))
	if blocked.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("run = %+v, want legacy approval blocker unchanged", blocked)
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
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	state.schedulerDiagnostics.recordCommand(schedulerKindRunContinuation, "accepted")
	state.schedulerDiagnostics.recordCommandEnqueueFailure(schedulerKindRunContinuation, schedulerReasonEnqueueFailed)
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: "ws-metrics-diagnostics",
		RunID:       "run-metrics-diagnostics",
		Kind:        schedulerKindRunContinuation,
		State:       schedulerDiagnosticStateFailed,
		Reason:      schedulerReasonOwnerCommandFailed,
		Detail:      "owner command failed after accepted scheduler event",
	})
	state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: "ws-metrics-diagnostics",
		RunID:       "run-metrics-diagnostics",
		Kind:        schedulerKindRunContinuation,
		State:       schedulerDiagnosticStateNoActionNeeded,
		Reason:      schedulerReasonRunNotEligible,
		Detail:      "run no longer needs continuation",
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
		`workorch_scheduler_continuation_failures_total{kind="run_continuation",reason="owner_command_failed"} 1`,
		`workorch_scheduler_continuation_no_action_total{kind="run_continuation",reason="run_not_eligible"} 1`,
		`workorch_watchdog_timer_registrations_total 1`,
		`workorch_watchdog_interval_seconds 0`,
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
	skipObsoleteSynchronousExecutionEndpointTest(t)
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

	session := newOwnerRuntimeSession(t, "file", dataRoot)
	var err error
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, "run-stuck-validation")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateActive
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	routine := &recordingBackgroundRoutine{}
	second := newIsolatedTestHTTPServerWithBackground(Config{Backend: "file", DataRoot: dataRoot}, routine)
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
	getRun := func() projectioncontract.Run {
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
	if run := getRun(); run.State != string(common.RunStateActive) {
		t.Fatalf("run state after Validate = %s, want running until explicit recovery", run.State)
	}
	if routine.timerTask != nil || routine.timerCalls != 0 {
		t.Fatalf("startup registered watchdog timer calls=%d task=%v, want Runtime-only startup", routine.timerCalls, routine.timerTask != nil)
	}

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
	if body.Run.State != string(common.RunStateActive) || body.Run.FailureReason != "" {
		t.Fatalf("run = %+v, want running run preserved for manual recovery", body.Run)
	}
}

var runtimeHTTPFixtureSequence atomic.Uint64

func createWorkspaceForHTTP(t *testing.T, handler http.Handler, title string, deliverableDirectory string) projectioncontract.Workspace {
	t.Helper()

	// Runtime deliberately rejects an activation without an explicitly sealed
	// acceptance-slice policy.  Keep the general HTTP fixture valid through the
	// same template path used by production instead of reintroducing a runtime
	// default only for tests.
	templateID := fmt.Sprintf("runtime-http-fixture-%d", runtimeHTTPFixtureSequence.Add(1))
	templateBody := `{"id":"` + templateID + `","name":"Runtime HTTP fixture","contextHints":{"acceptanceSlicePolicy":{"schemaVersion":"workorch.acceptance_slice_policy","maxWorkItems":1,"maxFocuses":8,"maxCandidateContextItems":4,"completionPolicy":"all_required_items_accepted","resultPolicy":"slice_result_allowed","groups":[{"id":"http-fixture-all","title":"HTTP fixture acceptance","scope":"http_fixture","priority":10,"maxWorkItems":1,"match":{"all":true}}]},"profileContracts":{"http-fixture-proof":{"evidenceRequirements":[{"id":"*","scope":"dodItem","requiredEvidenceKinds":["validation"],"acceptedStepKinds":["validation/managed_session"],"targetRole":"http_fixture_target","targetDiscoverySources":["workspace_manifest"],"targetSelectorRef":"file/http_fixture_target","targetSelectorOwnerRef":"http-fixture-proof","targetSelectorSubjectRef":"dodItem","validationRecipeId":"http-fixture-validation","requiredCheckRefs":["http-fixture-check"]}]}}}}}`
	postAndExpect(t, handler, "/api/v1/workorch/workspace-templates", templateBody, http.StatusCreated)

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"title":"`+title+`","deliverableDirectory":"`+deliverableDirectory+`","templateId":"`+templateID+`"}`))
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

func createWorkspaceAndRunForHTTP(t *testing.T, handler http.Handler, title string, runID string) projectioncontract.Workspace {
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
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "workorchd/server/http-test", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
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
	if _, ok := data.(decisionevents.DecisionCommandResult); !ok {
		t.Fatalf("EnsureResolvedTicket result type = %T, want DecisionCommandResult", data)
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

	commandHub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("command hub: %v", err)
	}
	topic := approvalevents.TopicApprovalCommandEnsureApproved
	var command any = approvalevents.EnsureApprovedCommand{ApprovalID: common.ApprovalID(approvalID)}
	if reject {
		topic = approvalevents.TopicApprovalCommandEnsureRejected
		command = approvalevents.EnsureRejectedCommand{ApprovalID: common.ApprovalID(approvalID)}
	}
	event := magicevent.NewEventWithContext(topic, "workorchd/server/http-test", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(approvalevents.Contracts(), topic, eventcontract.LaneScope{SubjectType: "approval", SubjectID: approvalID}))
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
	if _, ok := data.(approvalevents.ApprovalCommandResult); !ok {
		t.Fatalf("resolve approval result type = %T, want ApprovalCommandResult", data)
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
