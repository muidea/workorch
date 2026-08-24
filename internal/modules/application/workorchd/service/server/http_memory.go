package server

import (
	"context"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func registerMemoryReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/memory", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory/:memoryId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedMemoryGet(w, r)
	})
}

func registerMemoryWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory" {
			http.NotFound(w, r)
			return
		}
		state.metrics.memoryCreateRequests.Add(1)
		var req workorchmanagementcontract.MemoryMutation
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

func (s *serverState) handleWorkspaceScopedMemoryGet(w http.ResponseWriter, r *http.Request) {
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
	workspaceID, memoryID, ok := workspaceResourceIDFromPath(r.URL.Path, "memory")
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req workorchmanagementcontract.MemoryMutation
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
