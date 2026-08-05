package server

import (
	"context"
	"net/http"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func registerCatalogReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/souls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.soulListRequests.Add(1)
		items, err := state.listSouls(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Souls:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulGetRequests.Add(1)
		item, err := state.readSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		items, err := state.listWorkspaceTemplates(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceTemplatesResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			WorkspaceTemplates: items,
		})
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.skillListRequests.Add(1)
		query := contextregistrycore.SkillQuery{WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId"))}
		items, err := state.listSkills(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skills:   items,
		})
	})
	router.AddHandler("/api/v1/workorch/skills/:skillId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		skillID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/skills/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.skillGetRequests.Add(1)
		query := contextregistrycore.SkillQuery{WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId"))}
		item, err := state.readSkill(r.Context(), state.config.Backend, state.config.DataRoot, skillID, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
}

func registerCatalogWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/souls", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.soulCreateRequests.Add(1)
		var req workorchmanagementcontract.SoulMutation
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSoul(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulUpdateRequests.Add(1)
		var req workorchmanagementcontract.SoulMutation
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulDeleteRequests.Add(1)
		if err := state.deleteSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID); err != nil {
			writeBusinessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/workspace-templates", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		var req workorchmanagementcontract.WorkspaceTemplate
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspaceTemplateResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			WorkspaceTemplate: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates/:templateId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		templateID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/workspace-templates/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		var req workorchmanagementcontract.WorkspaceTemplate
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, templateID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceTemplateResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			WorkspaceTemplate: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates/:templateId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		templateID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/workspace-templates/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := state.deleteWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, templateID); err != nil {
			writeBusinessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.skillCreateRequests.Add(1)
		var req workorchmanagementcontract.SkillMutation
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSkill(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
}
