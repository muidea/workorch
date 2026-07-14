package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadRuntimeRawContentWithBackendReturnsLLMCallFile(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceID := "ws-raw-content"
	runID := "run-raw-content"
	stepPath := "run-raw-content/llm-1"
	root := filepath.Join(dataRoot, "internal", "workspaces", workspaceID, "runs", runID, "llm-calls", runID, "001-llm-1")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir llm raw root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "response.sse"), []byte("data: {\"type\":\"done\"}\n\n"), 0o600); err != nil {
		t.Fatalf("write response.sse: %v", err)
	}

	ref := "workorch://workspaces/ws-raw-content/runs/run-raw-content/llm-calls/" + stepPath + "/response.sse"
	item, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, ref)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend: %v", err)
	}
	if item.Ref != ref || item.FileName != "response.sse" || item.ContentType != "text/event-stream; charset=utf-8" {
		t.Fatalf("raw content metadata = %+v, want response.sse event stream", item)
	}
	if string(item.Content) != "data: {\"type\":\"done\"}\n\n" {
		t.Fatalf("content = %q, want SSE payload", string(item.Content))
	}
}

func TestReadRuntimeRawContentWithBackendReturnsStepAndToolPayloadFiles(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceID := "ws-raw-runtime"
	runID := "run-raw-runtime"
	stepID := "run-raw-runtime/step-1"
	toolCallID := "run-raw-runtime/step-1/tool-1"
	stepDir := filepath.Join(dataRoot, "internal", "workspaces", workspaceID, "runs", runID, "steps", runID, "001-step-1")
	toolDir := filepath.Join(dataRoot, "internal", "workspaces", workspaceID, "runs", runID, "tool-calls", runID, "001-step-1-tool-1")
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		t.Fatalf("mkdir step dir: %v", err)
	}
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("mkdir tool dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stepDir, "output.json"), []byte(`{"stage":"done"}`), 0o600); err != nil {
		t.Fatalf("write step output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "arguments.json"), []byte(`{"command":"test"}`), 0o600); err != nil {
		t.Fatalf("write tool arguments: %v", err)
	}

	stepRef := "workorch://workspaces/" + workspaceID + "/runs/" + runID + "/steps/" + stepID + "/output.json"
	stepPayload, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, stepRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend step: %v", err)
	}
	if stepPayload.ContentType != "application/json; charset=utf-8" || string(stepPayload.Content) != `{"stage":"done"}` {
		t.Fatalf("step payload = %+v/%q, want json output", stepPayload, string(stepPayload.Content))
	}

	toolRef := "workorch://workspaces/" + workspaceID + "/runs/" + runID + "/tool-calls/" + toolCallID + "/arguments.json"
	toolPayload, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, toolRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend tool: %v", err)
	}
	if toolPayload.ContentType != "application/json; charset=utf-8" || string(toolPayload.Content) != `{"command":"test"}` {
		t.Fatalf("tool payload = %+v/%q, want json arguments", toolPayload, string(toolPayload.Content))
	}
}

func TestReadRuntimeRawContentWithBackendReturnsVirtualCheckpointAndResultDetails(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-raw-virtual")
	runID := common.RunID("run-raw-virtual")
	checkpointID := common.CheckpointID("run-raw-virtual/checkpoint-1")
	resultID := common.ResultID("run-raw-virtual/result-1")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "raw virtual", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"failureClass": "none"}),
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          resultID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.ResultStateAccepted,
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: true,
			Summary:   "accepted",
		},
		SummaryDetail: resultmodel.ResultSummary{
			GoalOutcome: "done",
		},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}

	checkpointRef := "workorch://workspaces/" + string(workspaceID) + "/runs/" + string(runID) + "/runtime-checkpoints/" + string(checkpointID) + "/data.json"
	checkpointPayload, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, checkpointRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend checkpoint: %v", err)
	}
	if checkpointPayload.ContentType != "application/json; charset=utf-8" || !strings.Contains(string(checkpointPayload.Content), `"failureClass": "none"`) {
		t.Fatalf("checkpoint payload = %+v/%q, want virtual data json", checkpointPayload, string(checkpointPayload.Content))
	}

	resultRef := "workorch://workspaces/" + string(workspaceID) + "/runs/" + string(runID) + "/results/" + string(resultID) + "/summary-detail.json"
	resultPayload, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, resultRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend result: %v", err)
	}
	if resultPayload.ContentType != "application/json; charset=utf-8" || !strings.Contains(string(resultPayload.Content), `"GoalOutcome": "done"`) {
		t.Fatalf("result payload = %+v/%q, want virtual summary detail json", resultPayload, string(resultPayload.Content))
	}
}

func TestReadRuntimeRawContentWithBackendScopesVirtualReadsByURIWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceA := common.WorkspaceID("ws-raw-scope-a")
	workspaceB := common.WorkspaceID("ws-raw-scope-b")
	runID := common.RunID("run-raw-scope")
	checkpointID := common.CheckpointID("run-raw-scope/checkpoint-1")
	resultID := common.ResultID("run-raw-scope/result-1")
	ctxA := common.ContextWithRuntimeWorkspaceID(ctx, workspaceA)
	ctxB := common.ContextWithRuntimeWorkspaceID(ctx, workspaceB)
	for _, workspaceID := range []common.WorkspaceID{workspaceA, workspaceB} {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := exports.Repositories.Workspaces.Create(scopedCtx, &workspacemodel.Workspace{ID: workspaceID, Title: string(workspaceID), State: common.WorkspaceStateActive}); err != nil {
			t.Fatalf("create workspace %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.Runs.Create(scopedCtx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctxA, &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceA,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"workspace": string(workspaceA)}),
	}); err != nil {
		t.Fatalf("create checkpoint A: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctxB, &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceB,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"workspace": string(workspaceB)}),
	}); err != nil {
		t.Fatalf("create checkpoint B: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctxA, &resultmodel.Result{
		ID:            resultID,
		WorkspaceID:   workspaceA,
		RunID:         runID,
		State:         common.ResultStateAccepted,
		SummaryDetail: resultmodel.ResultSummary{GoalOutcome: "workspace-a"},
	}); err != nil {
		t.Fatalf("create result A: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctxB, &resultmodel.Result{
		ID:            resultID,
		WorkspaceID:   workspaceB,
		RunID:         runID,
		State:         common.ResultStateAccepted,
		SummaryDetail: resultmodel.ResultSummary{GoalOutcome: "workspace-b"},
	}); err != nil {
		t.Fatalf("create result B: %v", err)
	}

	checkpointRef := "workorch://workspaces/" + string(workspaceB) + "/runs/" + string(runID) + "/runtime-checkpoints/" + string(checkpointID) + "/data.json"
	checkpointPayload, err := ReadRuntimeRawContentWithBackend(context.Background(), "file", dataRoot, checkpointRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend checkpoint: %v", err)
	}
	if !strings.Contains(string(checkpointPayload.Content), `"workspace": "ws-raw-scope-b"`) {
		t.Fatalf("checkpoint payload = %q, want workspace B scoped content", string(checkpointPayload.Content))
	}

	resultRef := "workorch://workspaces/" + string(workspaceB) + "/runs/" + string(runID) + "/results/" + string(resultID) + "/summary-detail.json"
	resultPayload, err := ReadRuntimeRawContentWithBackend(context.Background(), "file", dataRoot, resultRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend result: %v", err)
	}
	if !strings.Contains(string(resultPayload.Content), `"GoalOutcome": "workspace-b"`) {
		t.Fatalf("result payload = %q, want workspace B scoped content", string(resultPayload.Content))
	}
}

func TestReadRuntimeRawContentWithBackendRejectsUnsafeRefs(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "file:///tmp/secret"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("external ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/../runs/run-a/llm-calls/step/request.json"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsafe workspace ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/ws-a/runs/run-a/llm-calls/step/secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unknown file ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "memory", dataRoot, "workorch://workspaces/ws-a/runs/run-a/llm-calls/step/request.json"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("memory backend err = %v, want not found", err)
	}
}
