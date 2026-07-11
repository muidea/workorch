package server

import (
	"context"
	"net/http"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

func registerMemoryReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/memory", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
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
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx, ok := memoryRequestContext(w, r)
		if !ok {
			return
		}
		item, err := state.readMemoryItem(ctx, state.config.Backend, state.config.DataRoot, memoryID)
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
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx, ok := memoryRequestContext(w, r)
		if !ok {
			return
		}
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMemoryItem(ctx, state.config.Backend, state.config.DataRoot, memoryID, req)
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
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx, ok := memoryRequestContext(w, r)
		if !ok {
			return
		}
		if err := state.deleteMemoryItem(ctx, state.config.Backend, state.config.DataRoot, memoryID); err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "workorch",
			"deleted": true,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedMemoryGet(w, r)
	})
}

func registerMemoryWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), state.commandHub))
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedMemoryPut(w, r)
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedMemoryDelete(w, r)
	})
}

func memoryRequestContext(w http.ResponseWriter, r *http.Request) (context.Context, bool) {
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is required"})
		return nil, false
	}
	return common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID)), true
}

func (s *serverState) handleWorkspaceScopedMemoryGet(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), s.commandHub))
	workspaceID, memoryID, ok := workspaceResourceIDFromPath(r.URL.Path, "memory")
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	item, err := s.readMemoryItem(ctx, s.config.Backend, s.config.DataRoot, memoryID)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryItemResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Memory: item})
}

func (s *serverState) handleWorkspaceScopedMemoryPut(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), s.commandHub))
	workspaceID, memoryID, ok := workspaceResourceIDFromPath(r.URL.Path, "memory")
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req createMemoryItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	item, err := s.updateMemoryItem(ctx, s.config.Backend, s.config.DataRoot, memoryID, req)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryItemResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Memory: item})
}

func (s *serverState) handleWorkspaceScopedMemoryDelete(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithCommandHub(r.Context(), s.commandHub))
	workspaceID, memoryID, ok := workspaceResourceIDFromPath(r.URL.Path, "memory")
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	if err := s.deleteMemoryItem(ctx, s.config.Backend, s.config.DataRoot, memoryID); err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "workorch",
		"deleted": true,
	})
}
