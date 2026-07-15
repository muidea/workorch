package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	client "github.com/muidea/workorch/internal/pkg/workorchclient"
)

func TestRemoteEventsBackendSubscribesToSSEStream(t *testing.T) {
	envelope := backendEventData{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-remote",
		Payload:     events.WorkspacePayloadData{Title: "remote workspace", DeliverableDirectory: "/tmp/remote", State: common.WorkspaceStateActive, ActiveRunID: "run-remote"}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan Event, 1)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == EventWorkspaceListChanged {
				continue
			}
			if evt.Type != EventWorkspaceStateChanged || evt.WorkspaceID != "ws-remote" {
				t.Fatalf("event = %+v, want workspace state event for ws-remote", evt)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for remote SSE event")
		}
	}
}

func TestRemoteSchedulerDiagnosticsPreservesContinuationReadback(t *testing.T) {
	got := schedulerDiagnosticsInfoFromClient(&client.SchedulerDiagnostics{
		LatestContinuation: &client.ContinuationDiagnostic{
			WorkspaceID: "ws-remote-readback",
			RunID:       "run-remote-readback",
			Kind:        "run_continuation",
			State:       "failed",
			Reason:      "owner_readback_not_advanced",
			Readback: &client.ContinuationReadbackDiagnostic{
				WorkspaceID:             "ws-remote-readback",
				RunID:                   "run-remote-readback",
				Action:                  "resume",
				Found:                   true,
				OldRunState:             "blocked_on_decision",
				NewRunState:             "blocked_on_decision",
				OldBlockingTicketID:     "ticket-old",
				NewBlockingTicketID:     "ticket-old",
				Reason:                  "blocked_on_resolved_decision",
				ProjectionRevision:      11,
				SourceEventRevision:     12,
				ProjectedSourceRevision: 12,
			},
		},
	})
	if got == nil || got.LatestContinuation == nil || got.LatestContinuation.Readback == nil {
		t.Fatalf("diagnostics = %+v, want readback", got)
	}
	readback := got.LatestContinuation.Readback
	if readback.Action != "resume" ||
		readback.NewBlockingTicketID != "ticket-old" ||
		readback.Reason != "blocked_on_resolved_decision" ||
		readback.ProjectionRevision != 11 {
		t.Fatalf("readback = %+v, want preserved remote readback", readback)
	}
}

func TestRemoteProjectionEventsBackendRequestsProjectionStreamView(t *testing.T) {
	seenView := make(chan string, 1)
	envelope := backendEventData{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-projection",
		Payload: map[string]any{
			"type":              "workspace_projection_changed",
			"workspaceId":       "ws-projection",
			"workspaceRevision": 3,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		seenView <- r.URL.Query().Get("view")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan Event, 2)
	be := NewRemoteProjectionEvents(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe projection remote events: %v", err)
	}
	defer unsubscribe()

	select {
	case view := <-seenView:
		if view != "projection" {
			t.Fatalf("stream view = %q, want projection", view)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for projection stream request")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == EventWorkspaceListChanged {
				continue
			}
			if evt.Type != EventWorkspaceProjectionChanged || evt.WorkspaceID != "ws-projection" {
				t.Fatalf("event = %+v, want workspace projection event for ws-projection", evt)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for projection remote SSE event")
		}
	}
}

func TestRemoteWorkspaceListPreservesWorkspaceProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/workspaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"service":"workorch",
			"projection":{"state":"ready","globalRevision":99,"sourceEventRevision":99,"projectedSourceRevision":99},
			"workspaces":[{
				"id":"ws-remote",
				"title":"Remote",
				"state":"completed",
				"projection":{"state":"ready","globalRevision":21,"workspaceRevision":7,"sourceEventRevision":12,"projectedSourceRevision":12}
			}]
		}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Workspace().List(context.Background())
	if err != nil {
		t.Fatalf("remote workspace list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("workspaces = %+v, want one workspace", got)
	}
	projection := got[0].Projection
	if projection.GlobalRevision != 21 || projection.WorkspaceRevision != 7 || projection.SourceEventRevision != 12 || projection.ProjectedSourceRevision != 12 {
		t.Fatalf("workspace projection = %+v, want item projection without envelope global overwrite", projection)
	}
}

func TestRemoteEventsBackendReconnectsAndSignalsWorkspaceListRefresh(t *testing.T) {
	var requests atomic.Int32
	envelope := backendEventData{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-created-after-reconnect",
		Payload:     events.WorkspacePayloadData{Title: "created after reconnect", DeliverableDirectory: "/tmp/reconnect", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		if requests.Add(1) == 1 {
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan Event, 8)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	sawRefresh := false
	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == EventWorkspaceListChanged {
				sawRefresh = true
				continue
			}
			if evt.Type != EventWorkspaceCreated || evt.WorkspaceID != "ws-created-after-reconnect" {
				t.Fatalf("event = %+v, want workspace created after reconnect", evt)
			}
			if !sawRefresh {
				t.Fatal("expected workspace list refresh signal before replayed live event")
			}
			if requests.Load() < 2 {
				t.Fatalf("requests = %d, want reconnect", requests.Load())
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for reconnected remote SSE event")
		}
	}
}

func TestRemoteEventsBackendReconnectsWithLastEventID(t *testing.T) {
	var requests atomic.Int32
	seenLastEventID := make(chan string, 2)
	first := backendEventData{
		ID:          "evt-first",
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-replay",
		Payload:     events.WorkspacePayloadData{Title: "replay", DeliverableDirectory: "/tmp/replay", State: common.WorkspaceStateActive, ActiveRunID: "run-replay"}.EventPayload().Map(),
	}
	second := backendEventData{
		ID:          "evt-second",
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-replay",
		Payload:     events.WorkspacePayloadData{Title: "replay", DeliverableDirectory: "/tmp/replay", State: common.WorkspaceStateActive, ActiveRunID: "run-replay"}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		lastID := r.Header.Get("Last-Event-ID")
		if lastID == "" {
			lastID = r.URL.Query().Get("lastEventId")
		}
		seenLastEventID <- lastID
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		envelope := first
		if requests.Add(1) > 1 {
			envelope = second
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "id: %s\n", envelope.ID)
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan Event, 8)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	sawSecond := false
	deadline := time.After(3 * time.Second)
	for !sawSecond {
		select {
		case evt := <-received:
			if evt.Type == EventWorkspaceListChanged {
				continue
			}
			if evt.ID == "evt-second" {
				sawSecond = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for replayed remote SSE event")
		}
	}
	if requests.Load() < 2 {
		t.Fatalf("requests = %d, want reconnect", requests.Load())
	}
	firstLastID := <-seenLastEventID
	secondLastID := <-seenLastEventID
	if firstLastID != "" {
		t.Fatalf("first stream lastEventId = %q, want empty", firstLastID)
	}
	if secondLastID != "evt-first" {
		t.Fatalf("reconnect lastEventId = %q, want evt-first", secondLastID)
	}
}

func TestRemoteRunStatusMapsCurrentStepFields(t *testing.T) {
	be := NewRemote("http://127.0.0.1:1")
	status, err := be.Run().Status(context.Background(), "", common.RunID("run-remote"))
	if err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("remote run status err = %v, want invalid argument", err)
	}
	if status != nil {
		t.Fatalf("status = %+v, want nil on missing workspace scope", status)
	}
}

func TestRemoteRunCommandFirstResumeSendsWorkspaceCommandMetadata(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:        "ws-remote",
		Intent:             RunCommandIntentResume,
		RequestID:          "request-first-resume",
		Reason:             "user_requested",
		Source:             "test",
		DefinitionRevision: "rev-first-resume",
		Activation: ActivateRunRequest{
			GoalID:        "goal-remote",
			DoDID:         "dod-remote",
			LLMProviderID: "provider-remote",
			SoulID:        "soul-remote",
			SoulBindings:  map[string]common.SoulID{"validation": "soul-remote-validation"},
			SkillIDs:      []common.SkillID{"skill-remote"},
			MCPServerIDs:  []common.MCPServerID{"mcp-remote"},
			CapabilityIDs: []common.MCPCapabilityID{"mcp-remote:tool:validate"},
			MemoryIDs:     []common.MemoryID{"memory-remote"},
		},
	})
	if err != nil {
		t.Fatalf("remote first resume command: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["intent"] != string(RunCommandIntentResume) ||
			body["requestId"] != "request-first-resume" ||
			body["reason"] != "user_requested" ||
			body["source"] != "test" ||
			body["definitionRevision"] != "rev-first-resume" {
			t.Fatalf("request body = %+v, want resume command metadata", body)
		}
		if body["goalId"] != "goal-remote" ||
			body["doDId"] != "dod-remote" ||
			body["llmProviderId"] != "provider-remote" ||
			body["soulId"] != "soul-remote" {
			t.Fatalf("request body = %+v, want activation goal/dod payload preserved", body)
		}
		assertRemoteBodyStringMapValue(t, body, "soulBindings", "validation", "soul-remote-validation")
		assertRemoteBodyStringSlice(t, body, "skillIds", "skill-remote")
		assertRemoteBodyStringSlice(t, body, "mcpServerIds", "mcp-remote")
		assertRemoteBodyStringSlice(t, body, "capabilityIds", "mcp-remote:tool:validate")
		assertRemoteBodyStringSlice(t, body, "memoryIds", "memory-remote")
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive first resume request")
	}
}

func TestRemoteRunCommandResumeSendsRequestID(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:        "ws-remote",
		ExpectedRunID:      "run-001",
		Intent:             RunCommandIntentResume,
		RequestID:          "request-remote-resume",
		Reason:             "test_remote_resume",
		Source:             "test",
		DefinitionRevision: "rev-remote-resume",
		DecisionID:         "ticket-remote-resume",
	})
	if err != nil {
		t.Fatalf("remote run command: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["runId"] != "run-001" ||
			body["intent"] != string(RunCommandIntentResume) ||
			body["requestId"] != "request-remote-resume" ||
			body["reason"] != "test_remote_resume" ||
			body["source"] != "test" ||
			body["definitionRevision"] != "rev-remote-resume" ||
			body["decisionId"] != "ticket-remote-resume" {
			t.Fatalf("request body = %+v, want request id, reason, source, definition revision, and decision id", body)
		}
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive resume request")
	}
}

func TestRemoteRunCommandReplanSendsUnifiedIntent(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:        "ws-remote",
		ExpectedRunID:      "run-001",
		Intent:             RunCommandIntentReplan,
		RequestID:          "request-remote-replan",
		Reason:             "decision_replan_resolved",
		Source:             "tui",
		DefinitionRevision: "rev-remote-replan",
		DecisionID:         "ticket-remote-replan",
		Activation: ActivateRunRequest{
			SkillIDs:      []common.SkillID{"skill-remote-replan"},
			MCPServerIDs:  []common.MCPServerID{"mcp-remote-replan"},
			CapabilityIDs: []common.MCPCapabilityID{"mcp-remote-replan:tool:validate"},
			MemoryIDs:     []common.MemoryID{"memory-remote-replan"},
		},
	})
	if err != nil {
		t.Fatalf("remote run command replan: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["runId"] != "run-001" ||
			body["intent"] != string(RunCommandIntentReplan) ||
			body["requestId"] != "request-remote-replan" ||
			body["reason"] != "decision_replan_resolved" ||
			body["source"] != "tui" ||
			body["definitionRevision"] != "rev-remote-replan" ||
			body["decisionId"] != "ticket-remote-replan" {
			t.Fatalf("request body = %+v, want unified replan intent and command metadata", body)
		}
		assertRemoteBodyStringSlice(t, body, "skillIds", "skill-remote-replan")
		assertRemoteBodyStringSlice(t, body, "mcpServerIds", "mcp-remote-replan")
		assertRemoteBodyStringSlice(t, body, "capabilityIds", "mcp-remote-replan:tool:validate")
		assertRemoteBodyStringSlice(t, body, "memoryIds", "memory-remote-replan")
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive replan request")
	}
}

func TestRemoteRunCommandRestartSendsWorkspaceRestartCommandMetadata(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:        "ws-remote",
		Intent:             RunCommandIntentRestart,
		RequestID:          "request-remote-restart",
		Reason:             "operator_restart",
		Source:             "tui",
		Priority:           6,
		DefinitionRevision: "rev-remote-restart",
		DecisionID:         "ticket-remote-restart",
		Activation: ActivateRunRequest{
			GoalID:        "goal-remote-restart",
			DoDID:         "dod-remote-restart",
			LLMProviderID: "provider-remote-restart",
			SoulID:        "soul-remote-restart",
			SoulBindings:  map[string]common.SoulID{"validation": "soul-remote-restart-validation"},
			SkillIDs:      []common.SkillID{"skill-remote-restart"},
			MCPServerIDs:  []common.MCPServerID{"mcp-remote-restart"},
			CapabilityIDs: []common.MCPCapabilityID{"mcp-remote-restart:tool:validate"},
			MemoryIDs:     []common.MemoryID{"memory-remote-restart"},
		},
	})
	if err != nil {
		t.Fatalf("remote run command restart: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["intent"] != string(RunCommandIntentRestart) ||
			body["restart"] != true ||
			body["requestId"] != "request-remote-restart" ||
			body["reason"] != "operator_restart" ||
			body["source"] != "tui" ||
			body["definitionRevision"] != "rev-remote-restart" ||
			body["decisionId"] != "ticket-remote-restart" ||
			body["priority"] != float64(6) {
			t.Fatalf("request body = %+v, want restart command metadata", body)
		}
		if body["goalId"] != "goal-remote-restart" ||
			body["doDId"] != "dod-remote-restart" ||
			body["llmProviderId"] != "provider-remote-restart" ||
			body["soulId"] != "soul-remote-restart" {
			t.Fatalf("request body = %+v, want restart activation payload preserved", body)
		}
		assertRemoteBodyStringMapValue(t, body, "soulBindings", "validation", "soul-remote-restart-validation")
		assertRemoteBodyStringSlice(t, body, "skillIds", "skill-remote-restart")
		assertRemoteBodyStringSlice(t, body, "mcpServerIds", "mcp-remote-restart")
		assertRemoteBodyStringSlice(t, body, "capabilityIds", "mcp-remote-restart:tool:validate")
		assertRemoteBodyStringSlice(t, body, "memoryIds", "memory-remote-restart")
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive restart request")
	}
}

func TestRemoteRunCommandRejectsUnsupportedIntent(t *testing.T) {
	requests := atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	_, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:   "ws-remote",
		ExpectedRunID: "run-001",
		Intent:        RunCommandIntent("unsupported"),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want no remote request for unsupported intent", requests.Load())
	}
}

func TestRemoteRunCommandRejectsInvalidIntentScopeBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  WorkspaceRunCommand
	}{
		{
			name: "restart with run scope",
			req: WorkspaceRunCommand{
				WorkspaceID:   "ws-remote",
				ExpectedRunID: "run-remote",
				Intent:        RunCommandIntentRestart,
			},
		},
		{
			name: "replan without run scope",
			req: WorkspaceRunCommand{
				WorkspaceID: "ws-remote",
				Intent:      RunCommandIntentReplan,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := atomic.Int64{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()

			be := NewRemote(server.URL)
			_, err := be.Run().Command(context.Background(), tc.req)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("requests = %d, want no remote request for invalid intent scope", requests.Load())
			}
		})
	}
}

func assertRemoteBodyStringSlice(t *testing.T, body map[string]any, key string, want string) {
	t.Helper()
	values, ok := body[key].([]any)
	if !ok {
		t.Fatalf("request body %s = %+v, want string slice containing %q", key, body[key], want)
	}
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("request body %s = %+v, want %q", key, body[key], want)
}

func assertRemoteBodyStringMapValue(t *testing.T, body map[string]any, key string, mapKey string, want string) {
	t.Helper()
	values, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("request body %s = %+v, want object with %s=%q", key, body[key], mapKey, want)
	}
	if values[mapKey] != want {
		t.Fatalf("request body %s = %+v, want %s=%q", key, values, mapKey, want)
	}
}

func TestRemoteRunGetMapsDefinitionRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/runs/run-001" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"service":"workorch","run":{"id":"run-001","workspaceId":"ws-remote","goalId":"goal-001","doDId":"dod-001","definitionRevision":"goal:goal-001|dod:dod-001","state":"running"}}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Run().Get(context.Background(), "ws-remote", "run-001")
	if err != nil {
		t.Fatalf("remote run get: %v", err)
	}
	if got.DefinitionRevision != "goal:goal-001|dod:dod-001" {
		t.Fatalf("definitionRevision = %q, want mapped owner revision", got.DefinitionRevision)
	}
}

func TestRemoteRunListMapsDefinitionRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/runs" || r.URL.Query().Get("workspaceId") != "ws-remote" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"service":"workorch","runs":[{"id":"run-001","workspaceId":"ws-remote","definitionRevision":"rev-list-001","state":"running"}]}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Run().List(context.Background(), "ws-remote")
	if err != nil {
		t.Fatalf("remote run list: %v", err)
	}
	if len(got) != 1 || got[0].DefinitionRevision != "rev-list-001" {
		t.Fatalf("runs = %+v, want mapped definition revision", got)
	}
}

func TestRemoteRunStatusUsesWorkspaceScopedPathWhenContextCarriesWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/runs/run-001/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":{"id":"run-001","workspaceId":"ws-remote","state":"running"}}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-remote")
	status, err := be.Run().Status(ctx, "ws-remote", common.RunID("run-001"))
	if err != nil {
		t.Fatalf("remote run status: %v", err)
	}
	if status.WorkspaceID != "ws-remote" || status.ID != "run-001" {
		t.Fatalf("status = %+v, want scoped workspace/run", status)
	}
}

// TC10: typed projection SSE publishes connecting → live only after HTTP 2xx,
// then reconnecting on disconnect, and restores live with Last-Event-ID.
func TestRemoteProjectionEventsHealthAndReconnectWithLastEventID(t *testing.T) {
	var requests atomic.Int32
	seenLastEventID := make(chan string, 4)
	firstDTO := map[string]any{
		"id":             "proj-evt-1",
		"streamRevision": int64(11),
		"workspaceId":    "ws-proj-reconnect",
		"runId":          "run-1",
		"revision": map[string]any{
			"streamRevision":       int64(11),
			"workspaceRevision":    int64(5),
			"runtimeStateRevision": uint64(42),
			"evidenceSequence":     uint64(7),
			"activityRevision":     int64(9),
		},
		"changedSections": []string{"runtime_progress", "activity"},
		"projectionState": "ready",
		"updatedAt":       "2026-07-15T14:30:00Z",
	}
	secondDTO := map[string]any{
		"id":             "proj-evt-2",
		"streamRevision": int64(12),
		"workspaceId":    "ws-proj-reconnect",
		"runId":          "run-1",
		"revision": map[string]any{
			"streamRevision":       int64(12),
			"workspaceRevision":    int64(6),
			"runtimeStateRevision": uint64(43),
			"evidenceSequence":     uint64(8),
			"activityRevision":     int64(10),
		},
		"changedSections": []string{"activity"},
		"projectionState": "ready",
		"updatedAt":       "2026-07-15T14:31:00Z",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("view") != "projection" {
			http.Error(w, "want projection view", http.StatusBadRequest)
			return
		}
		lastID := r.Header.Get("Last-Event-ID")
		if lastID == "" {
			lastID = r.URL.Query().Get("lastEventId")
		}
		seenLastEventID <- lastID
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		dto := firstDTO
		reqN := requests.Add(1)
		if reqN > 1 {
			dto = secondDTO
		}
		data, err := json.Marshal(dto)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "id: %s\n", dto["id"])
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		// First connection ends immediately to force reconnect.
	}))
	defer server.Close()

	statuses := make(chan SubscriptionStatusInfo, 16)
	changed := make(chan WorkspaceProjectionChangedInfo, 8)
	be := NewRemoteProjectionEvents(server.URL)
	unsub, err := be.ProjectionEvents().SubscribeWorkspaceProjection(
		context.Background(),
		func(_ context.Context, info WorkspaceProjectionChangedInfo) {
			changed <- info
		},
		func(_ context.Context, status SubscriptionStatusInfo) {
			statuses <- status
		},
	)
	if err != nil {
		t.Fatalf("subscribe projection events: %v", err)
	}
	defer unsub()

	// Subscribe returns before live; first status must not skip connecting.
	sawConnecting := false
	sawLive := false
	sawReconnecting := false
	sawSecondLive := false
	var firstInfo, secondInfo WorkspaceProjectionChangedInfo
	deadline := time.After(5 * time.Second)
	for !(sawSecondLive && secondInfo.ID == "proj-evt-2") {
		select {
		case status := <-statuses:
			switch status.State {
			case SubscriptionConnecting:
				sawConnecting = true
			case SubscriptionLive:
				if !sawLive {
					sawLive = true
				} else if sawReconnecting {
					sawSecondLive = true
				}
			case SubscriptionReconnecting:
				sawReconnecting = true
			}
		case info := <-changed:
			if info.ID == "proj-evt-1" {
				firstInfo = info
			}
			if info.ID == "proj-evt-2" {
				secondInfo = info
			}
		case <-deadline:
			t.Fatalf("timed out: connecting=%v live=%v reconnecting=%v secondLive=%v first=%+v second=%+v requests=%d",
				sawConnecting, sawLive, sawReconnecting, sawSecondLive, firstInfo, secondInfo, requests.Load())
		}
	}
	if !sawConnecting {
		t.Fatal("expected connecting status before live")
	}
	if !sawLive {
		t.Fatal("expected live after SSE connected")
	}
	if !sawReconnecting {
		t.Fatal("expected reconnecting after first stream ends")
	}
	if firstInfo.Revision.RuntimeStateRevision != 42 || firstInfo.Revision.ActivityRevision != 9 {
		t.Fatalf("first typed notification = %+v, want runtime/activity revisions", firstInfo)
	}
	if len(firstInfo.ChangedSections) != 2 {
		t.Fatalf("first ChangedSections = %v, want runtime_progress+activity", firstInfo.ChangedSections)
	}
	if secondInfo.Revision.RuntimeStateRevision != 43 || secondInfo.Revision.ActivityRevision != 10 {
		t.Fatalf("second typed notification = %+v, want advanced revisions", secondInfo)
	}
	if requests.Load() < 2 {
		t.Fatalf("requests = %d, want reconnect", requests.Load())
	}
	firstLastID := <-seenLastEventID
	secondLastID := <-seenLastEventID
	if firstLastID != "" {
		t.Fatalf("first stream Last-Event-ID = %q, want empty", firstLastID)
	}
	if secondLastID != "proj-evt-1" {
		t.Fatalf("reconnect Last-Event-ID = %q, want proj-evt-1", secondLastID)
	}
}

// TC10: degraded status on connection failure; Subscribe itself does not report live.
func TestRemoteProjectionEventsDegradedOnConnectFailure(t *testing.T) {
	// Closed listener forces dial error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := server.URL
	server.Close()

	statuses := make(chan SubscriptionStatusInfo, 8)
	be := NewRemoteProjectionEvents(addr)
	unsub, err := be.ProjectionEvents().SubscribeWorkspaceProjection(
		context.Background(),
		func(context.Context, WorkspaceProjectionChangedInfo) {},
		func(_ context.Context, status SubscriptionStatusInfo) {
			statuses <- status
		},
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	deadline := time.After(2 * time.Second)
	sawConnecting := false
	for {
		select {
		case status := <-statuses:
			if status.State == SubscriptionConnecting {
				sawConnecting = true
				continue
			}
			if status.State == SubscriptionLive {
				t.Fatal("must not report live when SSE cannot connect")
			}
			if status.State == SubscriptionDegraded || status.State == SubscriptionReconnecting {
				if !sawConnecting {
					t.Fatal("expected connecting before degraded/reconnecting")
				}
				if strings.TrimSpace(status.LastError) == "" {
					t.Fatalf("status = %+v, want LastError", status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for degraded/reconnecting status")
		}
	}
}
