package server

import (
	"context"
	"net/http"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func registerConfigurationRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/configuration", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.configurationReadRequests.Add(1)
		var (
			item workorchconfigurationcontract.ServiceConfiguration
			err  error
		)
		if state.readConfigurationWithCatalog != nil {
			item, err = state.readConfigurationWithCatalog(r.Context())
		} else {
			item, err = state.readConfiguration()
		}
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
		state.metrics.configurationUpdateRequests.Add(1)
		var req configcore.UpdateServerConfigurationCommand
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
		state.metrics.configurationUpdateRequests.Add(1)
		var req configcore.UpdateRuntimeConfigurationCommand
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
		state.metrics.configurationUpdateRequests.Add(1)
		var req configcore.UpdateLLMConfigurationCommand
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
	router.AddHandler("/api/v1/workorch/configuration/llm/models", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.configurationReadRequests.Add(1)
		providerID := strings.TrimSpace(r.URL.Query().Get("providerId"))
		if providerID == "" {
			providerID = runtimecontract.BuiltinAIProxyProviderID
		}
		if state.getLLMModelCatalog == nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "llm model catalog handler is not configured"})
			return
		}
		catalog, err := state.getLLMModelCatalog(r.Context(), configcore.GetLLMModelCatalogCommand{ProviderID: providerID})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, llmModelCatalogResponse{
			Service: "workorch",
			Catalog: catalog,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/llm/models/refresh", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.configurationUpdateRequests.Add(1)
		providerID := strings.TrimSpace(r.URL.Query().Get("providerId"))
		if providerID == "" {
			// Optional JSON body may also carry providerId.
			var body struct {
				ProviderID string `json:"providerId"`
			}
			_ = decodeJSON(r, &body)
			providerID = strings.TrimSpace(body.ProviderID)
		}
		if providerID == "" {
			providerID = runtimecontract.BuiltinAIProxyProviderID
		}
		if state.refreshLLMModelCatalog == nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "llm model catalog refresh handler is not configured"})
			return
		}
		catalog, err := state.refreshLLMModelCatalog(r.Context(), configcore.RefreshLLMModelCatalogCommand{ProviderID: providerID})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, llmModelCatalogResponse{
			Service: "workorch",
			Catalog: catalog,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/web-search", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.configurationUpdateRequests.Add(1)
		var req configcore.UpdateWebSearchConfigurationCommand
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
