package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func registerRunReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), state.eventHub, state.commandHub))
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
			Runs:     runListMetadata(items),
		})
	})
	router.AddHandler("/api/v1/workorch/artifacts/content", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), state.eventHub, state.commandHub))
		state.metrics.artifactContentRequests.Add(1)
		artifactURI := strings.TrimSpace(r.URL.Query().Get("uri"))
		if artifactURI == "" {
			writeBusinessError(w, fmt.Errorf("%w: artifact uri is required", common.ErrInvalidArgument))
			return
		}
		item, err := state.readArtifactContentCached(r.Context(), artifactURI)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeArtifactContent(w, item)
	})
	router.AddHandler("/api/v1/workorch/runtime/payloads", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), state.eventHub, state.commandHub))
		ref := strings.TrimSpace(r.URL.Query().Get("ref"))
		if ref == "" {
			writeBusinessError(w, fmt.Errorf("%w: runtime payload ref is required", common.ErrInvalidArgument))
			return
		}
		item, err := state.readRuntimeRawContent(r.Context(), state.config.Backend, state.config.DataRoot, ref)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeRuntimeRawContent(w, item)
	})
}

func registerRunWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), state.eventHub, state.commandHub))
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
		state.rebuildProjectedWorkspace(r.Context(), workspaceID)
		writeJSON(w, http.StatusCreated, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), state.eventHub, state.commandHub))
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
			Runs:     runListMetadata(items),
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
}

func runListMetadata(items []appservice.Run) []appservice.Run {
	ret := make([]appservice.Run, 0, len(items))
	for _, item := range items {
		item.ExecutionContext = nil
		ret = append(ret, item)
	}
	return ret
}

func (s *serverState) handleWorkspaceScopedRunGet(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), s.eventHub, s.commandHub))
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
	r = r.WithContext(appservice.ContextWithEventHubs(r.Context(), s.eventHub, s.commandHub))
	workspaceID, runID, action, ok := workspaceRunActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := common.ContextWithRuntimeWorkspaceID(r.Context(), common.WorkspaceID(workspaceID))
	switch action {
	case "start":
		var req struct {
			RequestID          string `json:"requestId,omitempty"`
			Reason             string `json:"reason,omitempty"`
			Source             string `json:"source,omitempty"`
			DefinitionRevision string `json:"definitionRevision,omitempty"`
			DecisionID         string `json:"decisionId,omitempty"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		priority := runPriorityFromRequest(r, 0)
		command := prepareHTTPSchedulerCommand(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), Intent: "resume", Action: "start",
			Reason: req.Reason, Source: req.Source, RequestID: req.RequestID, DefinitionRevision: req.DefinitionRevision,
			DecisionID: req.DecisionID, Priority: priority,
		})
		if err := s.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command); err != nil {
			writeBusinessError(w, err)
			return
		}
		s.rebuildProjectedWorkspace(ctx, workspaceID)
		writeJSON(w, http.StatusAccepted, schedulerAcceptedHTTPBody(s.config, "start", command))
	case "resume":
		var req struct {
			ExecutionContextPatch map[string]any `json:"executionContextPatch,omitempty"`
			Intent                string         `json:"intent,omitempty"`
			RequestID             string         `json:"requestId,omitempty"`
			Reason                string         `json:"reason,omitempty"`
			Source                string         `json:"source,omitempty"`
			DefinitionRevision    string         `json:"definitionRevision,omitempty"`
			DecisionID            string         `json:"decisionId,omitempty"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		intent := strings.TrimSpace(req.Intent)
		if intent == "" {
			intent = "resume"
		}
		switch intent {
		case "resume", "replan":
		default:
			writeBusinessError(w, common.ErrInvalidArgument)
			return
		}
		priority := runPriorityFromRequest(r, 0)
		command := prepareHTTPSchedulerCommand(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), Intent: intent, Action: "resume",
			Reason: req.Reason, Source: req.Source, RequestID: req.RequestID, DefinitionRevision: req.DefinitionRevision,
			DecisionID: req.DecisionID, Priority: priority, ExecutionContextPatch: req.ExecutionContextPatch,
		})
		if err := s.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command); err != nil {
			writeBusinessError(w, err)
			return
		}
		s.rebuildProjectedWorkspace(ctx, workspaceID)
		writeJSON(w, http.StatusAccepted, schedulerAcceptedHTTPBody(s.config, "resume", command))
	default:
		http.NotFound(w, r)
	}
}
