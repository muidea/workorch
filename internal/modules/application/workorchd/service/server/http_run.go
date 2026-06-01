package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

func registerRunReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.runListRequests.Add(1)
		items, err := state.listRuns(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("workspaceId"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Runs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runGetRequests.Add(1)
		item, err := state.readRun(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runSnapshotRequests.Add(1)
		ctx := requestRunWorkspaceContext(r.Context(), r)
		item, err := state.readRunSnapshotCached(ctx, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runSnapshotResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Snapshot: appservice.ProjectRunSnapshotDetail(item),
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/steps", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "steps" {
			http.NotFound(w, r)
			return
		}
		query := stepQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunSteps(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStepsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Steps:    page.Steps,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/tool-calls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "tool-calls" {
			http.NotFound(w, r)
			return
		}
		query := toolCallQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunToolCalls(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runToolCallsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			ToolCalls: page.ToolCalls,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/runtime-checkpoints", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "runtime-checkpoints" {
			http.NotFound(w, r)
			return
		}
		query := runtimeCheckpointQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunRuntimeCheckpoints(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runRuntimeCheckpointsResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			RuntimeCheckpoints: page.RuntimeCheckpoints,
			Page:               page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/results", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "results" {
			http.NotFound(w, r)
			return
		}
		query := resultQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunResults(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runResultsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Results:  page.Results,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/patches", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "patches" {
			http.NotFound(w, r)
			return
		}
		query := patchQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunPatches(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runPatchesResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Patches:  page.Patches,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/validation-results", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "validation-results" {
			http.NotFound(w, r)
			return
		}
		query := validationResultQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunValidationResults(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runValidationResultsResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			ValidationResults: page.ValidationResults,
			Page:              page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/artifacts", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "artifacts" {
			http.NotFound(w, r)
			return
		}
		query := artifactQueryFromRequest(r)
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		page, err := state.listRunArtifacts(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runArtifactsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Artifacts: page.Artifacts,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/status", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "status" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStatusRequests.Add(1)
		started := time.Now()
		item, waitMatched, err := state.readRunStatusWithWait(requestRunWorkspaceContext(r.Context(), r), runID, r.URL.Query())
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStatusResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			Status:        item,
			WaitMatched:   waitMatched,
			WaitElapsedMs: int64(time.Since(started) / time.Millisecond),
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/observability", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "observability" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runObservabilityRequests.Add(1)
		item, err := state.readRunObservabilityCached(requestRunWorkspaceContext(r.Context(), r), runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runObservabilityResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			Observability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/memory-recalls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "memory-recalls" {
			http.NotFound(w, r)
			return
		}
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.RunID = runID
		query.WorkspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		items, err := state.listMemoryRecallTraces(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, query)
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
	router.AddHandler("/api/v1/workorch/artifacts/content", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.artifactContentRequests.Add(1)
		item, err := state.readArtifactContentCached(r.Context(), r.URL.Query().Get("uri"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeArtifactContent(w, item)
	})
	router.AddHandler("/api/v1/workorch/runtime/payloads", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		item, err := state.readRuntimeRawContent(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("ref"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeRuntimeRawContent(w, item)
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/task-progress", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "task-progress" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runTaskProgressRequests.Add(1)
		item, err := state.readRunTaskProgress(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskProgressResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			TaskProgress: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/context-bundle", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "context-bundle" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runContextBundleRequests.Add(1)
		item, err := state.readRunContextBundle(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, contextBundleResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			ContextBundle: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/task-decompositions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "task-decompositions" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runTaskDecompositionRequests.Add(1)
		items, err := state.readRunTaskDecompositions(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskDecompositionsResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			TaskDecompositions: items,
		})
	})
}

func registerRunWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runCreateRequests.Add(1)
		var req createRunRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if req.GoalID == "" || req.DoDID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "goalId and doDId must reference defined workspace goal and DoD"})
			return
		}
		item, err := state.createRun(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.RunID, req.GoalID, req.DoDID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runListRequests.Add(1)
		items, err := state.listRuns(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Runs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs/:runId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedRunGet(w, r)
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedRunGet(w, r)
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs/:runId/:action", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.handleWorkspaceScopedRunPost(w, r)
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/start", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "start" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStartRequests.Add(1)
		release, err := state.acquireRunSlotWithPriority(r.Context(), runPriorityFromRequest(r, 0))
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.startRun(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			state.scheduleReplanContinuationIfNeeded(r.Context(), item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		state.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/resume", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "resume" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			ExecutionContextPatch map[string]any `json:"executionContextPatch,omitempty"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.runResumeRequests.Add(1)
		release, err := state.acquireRunSlotWithPriority(r.Context(), runPriorityFromRequest(r, 0))
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.resumeRun(requestRunWorkspaceContext(r.Context(), r), state.config.Backend, state.config.DataRoot, runID, req.ExecutionContextPatch)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			state.scheduleReplanContinuationIfNeeded(r.Context(), item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		state.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
}

func (s *serverState) handleWorkspaceScopedRunGet(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithEventHub(r.Context(), s.eventHub))
	workspaceID, runID, action, ok := workspaceRunActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	switch action {
	case "":
		s.metrics.runGetRequests.Add(1)
		item, err := s.readRun(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runInstanceResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Run: item})
	case "snapshot":
		s.metrics.runSnapshotRequests.Add(1)
		item, err := s.readRunSnapshotCached(ctx, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runSnapshotResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Snapshot: appservice.ProjectRunSnapshotDetail(item)})
	case "steps":
		query := stepQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunSteps(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStepsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Steps: page.Steps, Page: page})
	case "tool-calls":
		query := toolCallQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunToolCalls(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runToolCallsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, ToolCalls: page.ToolCalls, Page: page})
	case "runtime-checkpoints":
		query := runtimeCheckpointQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunRuntimeCheckpoints(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runRuntimeCheckpointsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, RuntimeCheckpoints: page.RuntimeCheckpoints, Page: page})
	case "results":
		query := resultQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunResults(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runResultsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Results: page.Results, Page: page})
	case "patches":
		query := patchQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunPatches(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runPatchesResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Patches: page.Patches, Page: page})
	case "validation-results":
		query := validationResultQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunValidationResults(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runValidationResultsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, ValidationResults: page.ValidationResults, Page: page})
	case "artifacts":
		query := artifactQueryFromRequest(r)
		query.WorkspaceID = workspaceID
		query.RunID = runID
		page, err := s.listRunArtifacts(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runArtifactsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Artifacts: page.Artifacts, Page: page})
	case "status":
		s.metrics.runStatusRequests.Add(1)
		started := time.Now()
		item, waitMatched, err := s.readRunStatusWithWait(ctx, runID, r.URL.Query())
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStatusResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Status: item, WaitMatched: waitMatched, WaitElapsedMs: int64(time.Since(started) / time.Millisecond)})
	case "observability":
		s.metrics.runObservabilityRequests.Add(1)
		item, err := s.readRunObservabilityCached(ctx, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runObservabilityResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Observability: item})
	case "memory-recalls":
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.WorkspaceID = workspaceID
		query.RunID = runID
		items, err := s.listMemoryRecallTraces(ctx, s.config.Backend, s.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryRecallTracesResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, MemoryRecallTraces: items})
	case "task-progress":
		s.metrics.runTaskProgressRequests.Add(1)
		item, err := s.readRunTaskProgress(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskProgressResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, TaskProgress: item})
	case "context-bundle":
		s.metrics.runContextBundleRequests.Add(1)
		item, err := s.readRunContextBundle(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, contextBundleResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, ContextBundle: item})
	case "task-decompositions":
		s.metrics.runTaskDecompositionRequests.Add(1)
		items, err := s.readRunTaskDecompositions(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskDecompositionsResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, TaskDecompositions: items})
	default:
		http.NotFound(w, r)
	}
}

func (s *serverState) handleWorkspaceScopedRunPost(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithEventHub(r.Context(), s.eventHub))
	workspaceID, runID, action, ok := workspaceRunActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	switch action {
	case "start":
		s.metrics.runStartRequests.Add(1)
		release, err := s.acquireRunSlotWithPriority(ctx, runPriorityFromRequest(r, 0))
		if err != nil {
			s.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := s.startRun(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		s.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			s.scheduleReplanContinuationIfNeeded(ctx, item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		s.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Run: item})
	case "resume":
		var req struct {
			ExecutionContextPatch map[string]any `json:"executionContextPatch,omitempty"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		s.metrics.runResumeRequests.Add(1)
		release, err := s.acquireRunSlotWithPriority(ctx, runPriorityFromRequest(r, 0))
		if err != nil {
			s.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := s.resumeRun(ctx, s.config.Backend, s.config.DataRoot, runID, req.ExecutionContextPatch)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		s.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			s.scheduleReplanContinuationIfNeeded(ctx, item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		s.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Run: item})
	default:
		http.NotFound(w, r)
	}
}
