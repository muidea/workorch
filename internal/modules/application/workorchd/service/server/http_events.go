package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func registerEventReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/events", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.eventListRequests.Add(1)
		items, err := state.listEventRecords(r.Context(), state.config.Backend, state.config.DataRoot, eventRecordQueryFromRequest(r))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, eventRecordsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			EventRecords: items,
		})
	})
	router.AddHandler("/api/v1/workorch/events/stream", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stop := context.AfterFunc(state.streamCtx, cancel)
		defer stop()
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "streaming unsupported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		streamView := normalizeEventStreamView(r.URL.Query().Get("view"))
		filterEnvelope := func(envelope streamEvent) bool {
			if isProjectionEventStreamView(streamView) && envelope.Topic != workorchdevents.TopicWorkspaceProjectionChanged {
				return false
			}
			if workspaceID != "" && envelope.WorkspaceID != workspaceID {
				return false
			}
			if runID != "" && envelope.RunID != runID {
				return false
			}
			return true
		}
		eventCh, unsubscribe := state.subscribeStreamEvents()
		defer unsubscribe()
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
		if lastEventID == "" {
			lastEventID = strings.TrimSpace(r.URL.Query().Get("lastEventId"))
		}
		if !state.replayEventStream(ctx, lastEventID, workspaceID, runID, streamView, filterEnvelope, func(envelope streamEvent) bool {
			return writeSSEEnvelope(w, flusher, envelope)
		}) {
			return
		}
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case envelope := <-eventCh:
				if !filterEnvelope(envelope) {
					continue
				}
				if !writeSSEEnvelope(w, flusher, envelope) {
					return
				}
			}
		}
	})
}
