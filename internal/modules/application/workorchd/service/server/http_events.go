package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func registerEventReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/events", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
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
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		if state.eventHub == nil {
			writeBusinessError(w, common.ErrNotFound)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "streaming unsupported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		filterEnvelope := func(envelope events.Envelope) bool {
			if workspaceID != "" && envelope.WorkspaceID != workspaceID {
				return false
			}
			if runID != "" && envelope.RunID != runID {
				return false
			}
			return true
		}
		eventCh := make(chan events.Envelope, 128)
		unsubscribe := state.eventHub.Subscribe("*", func(_ context.Context, envelope events.Envelope) error {
			if !filterEnvelope(envelope) {
				return nil
			}
			enqueueEventStreamEnvelope(eventCh, envelope)
			return nil
		})
		defer unsubscribe()
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
		if lastEventID == "" {
			lastEventID = strings.TrimSpace(r.URL.Query().Get("lastEventId"))
		}
		for _, envelope := range state.replayEventStreamEnvelopes(r.Context(), lastEventID, workspaceID, runID, filterEnvelope) {
			if !writeSSEEnvelope(w, flusher, envelope) {
				return
			}
		}
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case envelope := <-eventCh:
				if !writeSSEEnvelope(w, flusher, envelope) {
					return
				}
			}
		}
	})
}
