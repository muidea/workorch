package server

import (
	"context"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

func registerConfigurationRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/configuration", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		state.metrics.configurationReadRequests.Add(1)
		item, err := state.readConfiguration()
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/server", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateServerConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateServerConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/runtime", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateRuntimeConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateRuntimeConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/llm", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateLLMConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateLLMConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/web-search", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateWebSearchConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWebSearchConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
}
