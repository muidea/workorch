package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func registerWorkspaceReadRoutes(router engine.RouteRegistry, state *serverState) {
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
			Workspaces: items,
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
			detail := mergeWorkspaceProjectionViewModel(readmodelcore.WorkspaceSnapshot{
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
			Workspace: item,
		})
	})
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
				Snapshot:   item,
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
				Snapshot:   appservice.ProjectWorkspaceSnapshotSummary(item),
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
			Snapshot:   appservice.ProjectWorkspaceSnapshotDetail(item),
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
		item = appservice.ProjectWorkspaceSnapshotDetail(item)
		// CR01/TC10: use store-owned activity so ActivityRevision is transactional
		// authority and survives read path without fingerprint regeneration.
		storedActivity, hasStoredActivity := readmodelcore.WorkspaceActivityFeed{}, false
		if state.workspaceProjection != nil {
			storedActivity, hasStoredActivity = state.workspaceProjection.ReadWorkspaceActivity(workspaceID)
		}
		view := appservice.ProjectWorkspaceRuntimeViewWithActivity(item, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
		writeJSON(w, http.StatusOK, workspaceRuntimeViewResponse{
			Service:     "workorch",
			Backend:     state.config.Backend,
			DataRoot:    state.config.DataRoot,
			Projection:  projection,
			RuntimeView: view,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory-recalls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory-recalls" {
			http.NotFound(w, r)
			return
		}
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.WorkspaceID = workspaceID
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
		item, err := state.readGoal(ctx, state.config.Backend, state.config.DataRoot, goalID)
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
		item, err := state.readDoD(ctx, state.config.Backend, state.config.DataRoot, dodID)
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

func workspaceMetadataViewModelFromProjection(input appmodelcore.WorkspaceViewModel) appmodelcore.WorkspaceViewModel {
	ret := appmodelcore.WorkspaceViewModel{
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
		ret.Sections = make(map[string]appmodelcore.WorkspaceViewModelSection, len(input.Sections))
		for key, value := range input.Sections {
			ret.Sections[key] = appmodelcore.WorkspaceViewModelSection{
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
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.workspaceCreateRequests.Add(1)
		var req createWorkspaceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if req.WorkspaceID != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is generated by workorch"})
			return
		}
		item, err := state.createWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, appmodelcore.CreateWorkspaceCommand{
			Title:                req.Title,
			Description:          req.Description,
			DeliverableDirectory: req.DeliverableDirectory,
			TemplateID:           req.TemplateID,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.upsertProjectedWorkspace(item)
		writeJSON(w, http.StatusCreated, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/llm-target", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "llm-target" {
			http.NotFound(w, r)
			return
		}
		var target runtimecontract.ProviderModelTarget
		if err := decodeJSON(r, &target); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWorkspaceLLMTarget(r.Context(), workspaceID, &target)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.upsertProjectedWorkspace(item)
		writeJSON(w, http.StatusOK, workspaceResponse{Service: "workorch", Backend: state.config.Backend, DataRoot: state.config.DataRoot, Workspace: item})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/llm-target", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "llm-target" {
			http.NotFound(w, r)
			return
		}
		item, err := state.updateWorkspaceLLMTarget(r.Context(), workspaceID, nil)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.upsertProjectedWorkspace(item)
		writeJSON(w, http.StatusOK, workspaceResponse{Service: "workorch", Backend: state.config.Backend, DataRoot: state.config.DataRoot, Workspace: item})
	})
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
		state.metrics.workspaceDeleteRequests.Add(1)
		if err := state.deleteWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, cmd); err != nil {
			writeBusinessError(w, err)
			return
		}
		if state.workspaceProjection != nil {
			state.workspaceProjection.DeleteWorkspace(workspaceID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/execution-activations", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "execution-activations" {
			http.NotFound(w, r)
			return
		}
		var req workspaceExecutionCommandRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.workspaceActivationRequests.Add(1)
		priority := runPriorityFromRequest(r, req.Priority)
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
		command, commandErr := workspaceExecutionCommandSchedulerCommand(workspaceID, req, priority)
		if commandErr != nil {
			writeBusinessError(w, commandErr)
			return
		}
		command.Intent = intent
		command = prepareWorkspaceRunSchedulerCommand(topic, command)
		if err := state.enqueueWorkorchDSchedulerCommand(topic, command); err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, schedulerAcceptedHTTPBody(state.config, "execution-activation", command))
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/soul-bindings", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "soul-bindings" {
			http.NotFound(w, r)
			return
		}
		var req appmodelcore.WorkspaceSoulBindings
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDefineRequests.Add(1)
		var req defineGoalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineGoal(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Text)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusCreated, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals/:goalId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, goalID, ok := workspaceResourceIDFromPath(r.URL.Path, "goals")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDefineRequests.Add(1)
		var req defineGoalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
		current, err := state.readGoal(ctx, state.config.Backend, state.config.DataRoot, goalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		if current.WorkspaceID != workspaceID {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "goal not found in workspace"})
			return
		}
		item, err := state.updateGoal(ctx, state.config.Backend, state.config.DataRoot, goalID, req.Text)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusOK, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodDefineRequests.Add(1)
		var req defineDoDRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineDoD(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Items)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusCreated, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods/:dodId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, dodID, ok := workspaceResourceIDFromPath(r.URL.Path, "dods")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodDefineRequests.Add(1)
		var req defineDoDRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
		current, err := state.readDoD(ctx, state.config.Backend, state.config.DataRoot, dodID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		if current.WorkspaceID != workspaceID {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "dod not found in workspace"})
			return
		}
		item, err := state.updateDoD(ctx, state.config.Backend, state.config.DataRoot, dodID, req.Items)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusOK, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goal-dod-quality-check" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDoDQualityCheckRequests.Add(1)
		var req goalDoDQualityCheckRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.checkGoalDoDQuality(r.Context(), state.config.Backend, state.config.DataRoot, qualitycore.Command{
			WorkspaceID: workspaceID,
			GoalID:      req.GoalID,
			DoDID:       req.DoDID,
			GoalText:    req.GoalText,
			DoDItems:    req.DoDItems,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalDoDQualityCheckResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			QualityCheck: item,
		})
	})
}

func workspaceExecutionCommandSchedulerCommand(workspaceID string, req workspaceExecutionCommandRequest, priority int) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	return workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID(workspaceID),
		Reason:      req.Reason,
		Source:      schedulerCommandSource(req.Source, "http"),
		RequestID:   req.RequestID,
		Priority:    priority,
	}, nil
}
