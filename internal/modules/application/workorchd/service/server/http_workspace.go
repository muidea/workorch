package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func registerWorkspaceReadRoutes(router engine.RouteRegistry, state *serverState) {
	registerWorkspaceResourceReadRoutes(router, state)
	registerWorkspaceProjectionReadRoutes(router, state)
	registerWorkspaceContextReadRoutes(router, state)
	registerWorkspaceDefinitionReadRoutes(router, state)
}

func registerWorkspaceResourceReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.workspaceListRequests.Add(1)
		items, projection, err := state.listProjectedWorkspaces()
		if err != nil {
			writeJSON(w, projectionHTTPStatus(err), workspacesResponse{
				Service:    "workorch",
				Backend:    state.config.Backend,
				DataRoot:   state.config.DataRoot,
				Projection: projection,
				Error:      err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, workspacesResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Projection: projection,
			Workspaces: appmodelcore.ProjectWorkspaces(items),
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceGetRequests.Add(1)
		item, err := state.readWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		if projected, _, readErr := state.readProjectedWorkspaceSnapshot(workspaceID); readErr == nil {
			detail := mergeWorkspaceProjectionViewModel(readmodelcore.WorkspaceSnapshotState{
				Workspace: item,
				Run:       projected.Run,
				Decisions: projected.Decisions,
				Approvals: projected.Approvals,
			}, projected.ViewModel)
			if detail.ViewModel.WorkspaceID != "" {
				item.ViewModel = workspaceMetadataViewModelFromProjection(detail.ViewModel)
			}
		}
		writeJSON(w, http.StatusOK, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: appmodelcore.ProjectWorkspace(item),
		})
	})
}

func registerWorkspaceProjectionReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceSnapshotRequests.Add(1)
		projection := state.workspaceProjectionStatus()
		switch workspaceSnapshotReadView(r) {
		case workspaceSnapshotViewObservability:
			item, err := state.readWorkspaceSnapshotCached(r.Context(), workspaceID)
			if err != nil {
				writeJSON(w, projectionHTTPStatus(err), workspaceSnapshotResponse{
					Service:    "workorch",
					Backend:    state.config.Backend,
					DataRoot:   state.config.DataRoot,
					Projection: projection,
					Error:      err.Error(),
				})
				return
			}
			if projected, projectedStatus, readErr := state.readProjectedWorkspaceSnapshot(workspaceID); readErr == nil {
				item = mergeWorkspaceProjectionOverlay(item, projected)
				projection = projectedStatus
			}
			writeJSON(w, http.StatusOK, workspaceSnapshotResponse{
				Service:    "workorch",
				Backend:    state.config.Backend,
				DataRoot:   state.config.DataRoot,
				Projection: projection,
				Snapshot:   readmodelprojection.WorkspaceProjection().PublicSnapshot(item),
			})
			return
		case workspaceSnapshotViewSummary:
			item, projectedStatus, err := state.readProjectedWorkspaceSummarySnapshot(workspaceID)
			if err != nil {
				writeJSON(w, projectionHTTPStatus(err), workspaceSnapshotResponse{
					Service:    "workorch",
					Backend:    state.config.Backend,
					DataRoot:   state.config.DataRoot,
					Projection: projection,
					Error:      err.Error(),
				})
				return
			}
			projection = projectedStatus
			writeJSON(w, http.StatusOK, workspaceSnapshotResponse{
				Service:    "workorch",
				Backend:    state.config.Backend,
				DataRoot:   state.config.DataRoot,
				Projection: projection,
				Snapshot:   readmodelprojection.WorkspaceProjection().PublicSnapshot(readmodelprojection.WorkspaceProjection().Summary(item)),
			})
			return
		}
		item, err := state.readWorkspaceDetailSnapshotCached(r.Context(), workspaceID)
		if err != nil {
			writeJSON(w, projectionHTTPStatus(err), workspaceSnapshotResponse{
				Service:    "workorch",
				Backend:    state.config.Backend,
				DataRoot:   state.config.DataRoot,
				Projection: projection,
				Error:      err.Error(),
			})
			return
		}
		if projected, projectedStatus, readErr := state.readProjectedWorkspaceSnapshot(workspaceID); readErr == nil {
			item = mergeWorkspaceProjectionOverlay(item, projected)
			projection = projectedStatus
		}
		writeJSON(w, http.StatusOK, workspaceSnapshotResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Projection: projection,
			Snapshot:   readmodelprojection.WorkspaceProjection().PublicSnapshot(readmodelprojection.WorkspaceProjection().Detail(item)),
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runtime-view", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runtime-view" {
			http.NotFound(w, r)
			return
		}
		activityLimit := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("activityLimit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				activityLimit = parsed
			}
		}
		beforeCursor := strings.TrimSpace(r.URL.Query().Get("beforeCursor"))
		projection := state.workspaceProjectionStatus()
		item, err := state.readWorkspaceDetailSnapshotCached(r.Context(), workspaceID)
		if err != nil {
			writeJSON(w, projectionHTTPStatus(err), workspaceRuntimeViewResponse{
				Service:    "workorch",
				Backend:    state.config.Backend,
				DataRoot:   state.config.DataRoot,
				Projection: projection,
				Error:      err.Error(),
			})
			return
		}
		if projected, projectedStatus, readErr := state.readProjectedWorkspaceSnapshot(workspaceID); readErr == nil {
			item = mergeWorkspaceProjectionOverlay(item, projected)
			projection = projectedStatus
		}
		item = readmodelprojection.WorkspaceProjection().Detail(item)
		// CR01/TC10: use store-owned activity so ActivityRevision is transactional
		// authority and survives read path without fingerprint regeneration.
		storedActivity, hasStoredActivity := workorchprojectioncontract.WorkspaceActivityFeed{}, false
		if state.workspaceProjection != nil {
			storedActivity, hasStoredActivity = state.workspaceProjection.ReadWorkspaceActivity(workspaceID)
		}
		view := readmodelprojection.WorkspaceProjection().RuntimeViewWithActivity(item, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
		writeJSON(w, http.StatusOK, workspaceRuntimeViewResponse{
			Service:     "workorch",
			Backend:     state.config.Backend,
			DataRoot:    state.config.DataRoot,
			Projection:  projection,
			RuntimeView: view,
		})
	})
}

func registerWorkspaceContextReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory-recalls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory-recalls" {
			http.NotFound(w, r)
			return
		}
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.WorkspaceID = common.WorkspaceID(workspaceID)
		items, err := state.listMemoryRecallTraces(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryRecallTracesResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			MemoryRecallTraces: items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/soul-bindings", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "soul-bindings" {
			http.NotFound(w, r)
			return
		}
		item, err := state.readWorkspaceSoulBindings(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSoulBindingsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			SoulBindings: item,
		})
	})
}

func registerWorkspaceDefinitionReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalListRequests.Add(1)
		items, err := state.listGoals(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goals:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals/:goalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, goalID, ok := workspaceResourceIDFromPath(r.URL.Path, "goals")
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
		state.metrics.goalGetRequests.Add(1)
		item, err := state.readGoal(ctx, state.config.Backend, state.config.DataRoot, workspaceID, goalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		if item.WorkspaceID != workspaceID {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "goal not found in workspace"})
			return
		}
		writeJSON(w, http.StatusOK, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodListRequests.Add(1)
		items, err := state.listDoDs(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoDs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods/:dodId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, dodID, ok := workspaceResourceIDFromPath(r.URL.Path, "dods")
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
		state.metrics.dodGetRequests.Add(1)
		item, err := state.readDoD(ctx, state.config.Backend, state.config.DataRoot, workspaceID, dodID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		if item.WorkspaceID != workspaceID {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "dod not found in workspace"})
			return
		}
		writeJSON(w, http.StatusOK, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
}

const (
	workspaceSnapshotViewSummary       = "summary"
	workspaceSnapshotViewDetail        = "detail"
	workspaceSnapshotViewObservability = "observability"
)

func workspaceSnapshotReadView(r *http.Request) string {
	if r == nil {
		return workspaceSnapshotViewDetail
	}
	switch strings.TrimSpace(strings.ToLower(r.URL.Query().Get("view"))) {
	case "summary", "overview", "status":
		return workspaceSnapshotViewSummary
	case "observability", "debug":
		return workspaceSnapshotViewObservability
	default:
		return workspaceSnapshotViewDetail
	}
}

func workspaceMetadataViewModelFromProjection(input workorchprojectioncontract.WorkspaceViewModel) workorchprojectioncontract.WorkspaceViewModel {
	ret := workorchprojectioncontract.WorkspaceViewModel{
		WorkspaceID:             input.WorkspaceID,
		State:                   input.State,
		PromptMode:              input.PromptMode,
		Revision:                input.Revision,
		GlobalRevision:          input.GlobalRevision,
		WorkspaceRevision:       input.WorkspaceRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
		UpdatedAt:               input.UpdatedAt,
		RebuiltAt:               input.RebuiltAt,
		Degraded:                input.Degraded,
		StaleReason:             input.StaleReason,
		Error:                   input.Error,
	}
	if len(input.Sections) > 0 {
		ret.Sections = make(map[string]workorchprojectioncontract.WorkspaceViewModelSection, len(input.Sections))
		for key, value := range input.Sections {
			ret.Sections[key] = workorchprojectioncontract.WorkspaceViewModelSection{
				Name:                    value.Name,
				Revision:                value.Revision,
				WorkspaceRevision:       value.WorkspaceRevision,
				SourceEventRevision:     value.SourceEventRevision,
				ProjectedSourceRevision: value.ProjectedSourceRevision,
				State:                   value.State,
			}
		}
	}
	return ret
}

func projectionHTTPStatus(err error) int {
	switch {
	case errors.Is(err, common.ErrInvalidArgument):
		return http.StatusBadRequest
	case errors.Is(err, common.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrBlocked):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func registerWorkspaceWriteRoutes(router engine.RouteRegistry, state *serverState) {
	registerWorkspaceInferenceWriteRoutes(router, state)
	registerWorkspaceDeleteRoute(router, state)
	registerWorkspaceExecutionWriteRoute(router, state)
	registerWorkspaceContextWriteRoute(router, state)
}

func registerWorkspaceInferenceWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/llm-selection", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "llm-selection" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			ProviderID       string `json:"providerId"`
			ModelID          string `json:"modelId"`
			ReasoningEffort  string `json:"reasoningEffort,omitempty"`
			ExpectedRevision uint64 `json:"expectedRevision"`
		}
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		selection := runtimecontract.ProviderInferenceSelection{
			Target:          runtimecontract.ProviderModelTarget{ProviderID: request.ProviderID, ModelID: request.ModelID},
			ReasoningEffort: request.ReasoningEffort,
		}
		item, err := state.updateWorkspaceLLMSelection(r.Context(), workspaceID, &selection, request.ExpectedRevision)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.upsertProjectedWorkspace(item)
		writeJSON(w, http.StatusOK, workspaceResponse{Service: "workorch", Backend: state.config.Backend, DataRoot: state.config.DataRoot, Workspace: appmodelcore.ProjectWorkspace(item)})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/llm-selection", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "llm-selection" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			ExpectedRevision uint64 `json:"expectedRevision"`
		}
		if r.Body != nil && r.ContentLength != 0 {
			if err := decodeJSON(r, &request); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
		}
		item, err := state.updateWorkspaceLLMSelection(r.Context(), workspaceID, nil, request.ExpectedRevision)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.upsertProjectedWorkspace(item)
		writeJSON(w, http.StatusOK, workspaceResponse{Service: "workorch", Backend: state.config.Backend, DataRoot: state.config.DataRoot, Workspace: appmodelcore.ProjectWorkspace(item)})
	})
}

func registerWorkspaceDeleteRoute(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		cmd := appmodelcore.DeleteWorkspaceCommand{}
		if r.Body != nil && r.ContentLength != 0 {
			if err := decodeJSON(r, &cmd); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
		}
		if value := strings.TrimSpace(r.URL.Query().Get("purge")); value != "" {
			purge, err := strconv.ParseBool(value)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid purge query"})
				return
			}
			cmd.Purge = purge
		}
		cmd = cmd.Normalized(workspaceID)
		state.metrics.workspaceDeleteRequests.Add(1)
		if err := state.deleteWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, cmd); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.deleteProjectedWorkspace(workspaceID)
		w.WriteHeader(http.StatusNoContent)
	})
}

func registerWorkspaceExecutionWriteRoute(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/execution-activations", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "execution-activations" {
			http.NotFound(w, r)
			return
		}
		var req workorchmanagementcontract.WorkspaceExecutionCommandRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.workspaceActivationRequests.Add(1)
		topic := workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested
		intent := strings.TrimSpace(req.Intent)
		if intent == "" {
			intent = "resume"
		}
		switch intent {
		case "resume":
		case "restart":
		default:
			writeBusinessError(w, fmt.Errorf("%w: workspace execution command supports resume or restart intent, got %q", common.ErrInvalidArgument, intent))
			return
		}
		command, commandErr := workspaceExecutionCommand(workspaceID, req)
		if commandErr != nil {
			writeBusinessError(w, commandErr)
			return
		}
		command.Intent = intent
		result, commandErr := state.admitWorkspaceExecution(r.Context(), topic, command)
		if commandErr != nil {
			writeBusinessError(w, commandErr)
			return
		}
		response, statusCode, responseErr := executionAdmissionHTTPResponse(state.config, "execution-activation", command, result)
		if responseErr != nil {
			writeBusinessError(w, fmt.Errorf("%w: %v", common.ErrConflict, responseErr))
			return
		}
		writeJSON(w, statusCode, response)
	})
}

func registerWorkspaceContextWriteRoute(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/soul-bindings", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "soul-bindings" {
			http.NotFound(w, r)
			return
		}
		var req workorchmanagementcontract.WorkspaceSoulBindings
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWorkspaceSoulBindings(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusOK, workspaceSoulBindingsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			SoulBindings: item,
		})
	})
}

func workspaceExecutionCommand(workspaceID string, req workorchmanagementcontract.WorkspaceExecutionCommandRequest) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	return workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID(workspaceID),
		Reason:      req.Reason,
		Source:      req.Source,
		RequestID:   req.RequestID,
	}, nil
}

func (s *serverState) updateWorkspaceLLMSelection(ctx context.Context, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
	if s != nil && s.updateWorkspaceLLMSelectionPort != nil {
		return s.updateWorkspaceLLMSelectionPort(ctx, workspaceID, selection, expectedRevision)
	}
	return appmodelcore.Workspace{}, fmt.Errorf("%w: workspace LLM selection application port is not configured", common.ErrConflict)
}
