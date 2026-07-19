package server

import (
	"context"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/common"
)

func registerMCPReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.mcpServerListRequests.Add(1)
		items, err := state.listMCPServers(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServersResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Servers:  items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.mcpCapabilityListRequests.Add(1)
		items, err := state.listMCPCapabilities(r.Context(), state.config.Backend, state.config.DataRoot, mcpCapabilityQueryFromRequest(r))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilitiesResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			Capabilities: items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities/:capabilityId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		capabilityID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/mcp/capabilities/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpCapabilityGetRequests.Add(1)
		item, err := state.readMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, capabilityID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpServerGetRequests.Add(1)
		item, err := state.readMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/tools", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "tools" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpToolListRequests.Add(1)
		items, err := state.listMCPServerTools(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpToolsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Tools:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/resources", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "resources" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpResourceListRequests.Add(1)
		items, err := state.listMCPServerResources(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpResourcesResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Resources: items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/prompts", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "prompts" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpPromptListRequests.Add(1)
		items, err := state.listMCPServerPrompts(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpPromptsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Prompts:  items,
		})
	})
}

func registerMCPWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.mcpServerCreateRequests.Add(1)
		var req createMCPServerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "" {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: common.ErrNotFound.Error()})
			return
		}
		var req updateMCPServerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, serverID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.mcpCapabilityCreateRequests.Add(1)
		var req createMCPCapabilityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities/:capabilityId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		capabilityID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/mcp/capabilities/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpCapabilityUpdateRequests.Add(1)
		var req createMCPCapabilityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, capabilityID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
}
