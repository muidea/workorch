package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchprojectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func registerRunReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
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
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
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

func runListMetadata(items []appmodelcore.Run) []workorchprojectioncontract.Run {
	return appmodelcore.ProjectRuns(items)
}

func (s *serverState) handleWorkspaceScopedRunGet(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusOK, runInstanceResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Run: appmodelcore.ProjectRun(item)})
	case "snapshot":
		s.metrics.runSnapshotRequests.Add(1)
		item, err := s.readRunSnapshotCached(ctx, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runSnapshotResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Snapshot: appservice.PublicRunSnapshot(appservice.ProjectRunSnapshotDetail(item))})
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
		writeJSON(w, http.StatusOK, runObservabilityResponse{Service: "workorch", Backend: s.config.Backend, DataRoot: s.config.DataRoot, Observability: appservice.PublicRunObservability(item)})
	case "memory-recalls":
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.WorkspaceID = common.WorkspaceID(workspaceID)
		query.RunID = common.RunID(runID)
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
	_, _, _, ok := workspaceRunActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Run-ID addressed lifecycle endpoints were removed. Runtime ingress is
	// workspace-scoped at /execution-activations so Workspace and Run owners
	// can select the current Run without a client-supplied target.
	http.NotFound(w, r)
}
