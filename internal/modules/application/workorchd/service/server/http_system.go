package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

func registerSystemRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/debug/pprof/**", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		http.DefaultServeMux.ServeHTTP(w, r)
	})
	router.AddHandler("/healthz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.healthzRequests.Add(1)
		writeJSON(w, http.StatusOK, statusResponse{
			Service: "workorch",
			Status:  "ok",
		})
	})
	router.AddHandler("/readyz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.readyzRequests.Add(1)
		if err := state.validate(r.Context(), state.config.Backend, state.config.DataRoot); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
			return
		}
		projection := state.workspaceProjectionStatus()
		switch projection.State {
		case workspaceProjectionStateReady, workspaceProjectionStateStale, workspaceProjectionStateDegraded:
		default:
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{
				Service:    "workorch",
				Status:     "not_ready",
				Projection: &projection,
			})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{
			Service:    "workorch",
			Status:     "ready",
			Projection: &projection,
		})
	})
	router.AddHandler("/metrics", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		state.metrics.metricsRequests.Add(1)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprint(w, state.metrics.render(state.config, state.workspaceProjection))
	})
}
