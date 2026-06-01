package server

import (
	"context"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

func registerMemoryReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/memory", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		query := memoryQueryFromValues(r.URL.Query())
		workspaceID := query.WorkspaceID
		if workspaceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is required"})
			return
		}
		state.metrics.memoryListRequests.Add(1)
		items, err := state.searchMemory(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   items,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		item, err := state.readMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := state.deleteMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID); err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "workorch",
			"deleted": true,
		})
	})
}

func registerMemoryWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory" {
			http.NotFound(w, r)
			return
		}
		state.metrics.memoryCreateRequests.Add(1)
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
}
