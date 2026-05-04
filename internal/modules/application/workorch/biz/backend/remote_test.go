package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestRemoteEventsBackendSubscribesToSSEStream(t *testing.T) {
	envelope := events.Envelope{
		Topic:       events.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-remote",
		Payload:     events.WorkspacePayload("remote workspace", "", "/tmp/remote", common.WorkspaceStateRunning, "run-remote"),
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

	select {
	case evt := <-received:
		if evt.Type != EventWorkspaceStateChanged || evt.WorkspaceID != "ws-remote" {
			t.Fatalf("event = %+v, want workspace state event for ws-remote", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote SSE event")
	}
}
