package server

import (
	"context"
	"net/http"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type definitionAuthoringResponse struct {
	Authoring workorchmanagementcontract.DefinitionAuthoringView `json:"authoring"`
}

func registerDefinitionAuthoringRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspace-authoring", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		if state.authoring.Create == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "规格创作服务未配置"})
			return
		}
		var request workorchmanagementcontract.DefinitionAuthoringCreate
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		view, err := state.authoring.Create(r.Context(), state.config.Backend, state.config.DataRoot, request)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, definitionAuthoringResponse{Authoring: view})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/authoring", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		id, ok := definitionAuthoringWorkspaceFromPath(r.URL.Path, "")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if state.authoring.Read == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "规格创作服务未配置"})
			return
		}
		view, err := state.authoring.Read(r.Context(), id)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, definitionAuthoringResponse{Authoring: view})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/authoring", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		id, ok := definitionAuthoringWorkspaceFromPath(r.URL.Path, "")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if state.authoring.Save == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "规格创作服务未配置"})
			return
		}
		var request workorchmanagementcontract.DefinitionAuthoringSave
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		view, err := state.authoring.Save(r.Context(), id, request)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, definitionAuthoringResponse{Authoring: view})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/authoring/compile", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		id, ok := definitionAuthoringWorkspaceFromPath(r.URL.Path, "compile")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if state.authoring.Compile == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "规格创作服务未配置"})
			return
		}
		view, err := state.authoring.Compile(r.Context(), id)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		status := http.StatusOK
		if view.CompilationAccepted {
			status = http.StatusAccepted
		}
		writeJSON(w, status, definitionAuthoringResponse{Authoring: view})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/authoring/confirm", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		id, ok := definitionAuthoringWorkspaceFromPath(r.URL.Path, "confirm")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if state.authoring.Confirm == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "规格创作服务未配置"})
			return
		}
		var request workorchmanagementcontract.DefinitionAuthoringConfirm
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		view, err := state.authoring.Confirm(r.Context(), id, request)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, definitionAuthoringResponse{Authoring: view})
	})
}

func definitionAuthoringWorkspaceFromPath(path, action string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	length := 6
	if action != "" {
		length++
	}
	if len(parts) != length || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workorch" || parts[3] != "workspaces" || parts[4] == "" || parts[5] != "authoring" || (action != "" && parts[6] != action) {
		return "", false
	}
	return parts[4], true
}
