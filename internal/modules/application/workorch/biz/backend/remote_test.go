package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestRemoteEventsBackendSubscribesToSSEStream(t *testing.T) {
	envelope := events.Envelope{
		Topic:       events.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-remote",
		Payload:     events.WorkspacePayload("remote workspace", "", "/tmp/remote", "", common.WorkspaceStateRunning, "run-remote"),
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

func TestRemoteEventsBackendReconnectsAndSignalsWorkspaceListRefresh(t *testing.T) {
	var requests atomic.Int32
	envelope := events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicWorkspaceCreated,
		WorkspaceID: "ws-created-after-reconnect",
		Payload:     events.WorkspacePayload("created after reconnect", "", "/tmp/reconnect", "", common.WorkspaceStateActive, ""),
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

func TestRemoteRunStatusMapsCurrentStepFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/runs/run-remote/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":{
			"id":"run-remote",
			"workspaceId":"ws-remote",
			"state":"running",
			"currentStepId":"run-remote/llm-after-decision",
			"currentStepKind":"llm/workspace_execution",
			"currentStepState":"dispatched",
			"stepCount":6
		}}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	status, err := be.Run().Status(context.Background(), common.RunID("run-remote"))
	if err != nil {
		t.Fatalf("remote run status: %v", err)
	}
	if status.CurrentStepID != "run-remote/llm-after-decision" ||
		status.CurrentStepKind != "llm/workspace_execution" ||
		status.CurrentStepState != common.StepStateDispatched {
		t.Fatalf("status current step = %s/%s/%s, want mapped current step fields", status.CurrentStepID, status.CurrentStepKind, status.CurrentStepState)
	}
}
