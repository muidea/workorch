package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpblockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	mcpbiz "github.com/muidea/workorch/internal/modules/kernel/mcp/biz"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_StartRunToComplete_Mainline(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}
	if ws.State != common.WorkspaceStateCompleted {
		t.Fatalf("workspace state = %q, want completed", ws.State)
	}

	stepID := common.StepID(string(runID) + "/step-1")
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
	if step.Output["status"] != "ok" {
		t.Fatalf("step output status = %v, want ok", step.Output["status"])
	}
	if step.Input[stepInputKeySoulStage] != soulStagePlanning {
		t.Fatalf("step soulStage = %v, want planning", step.Input[stepInputKeySoulStage])
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != stepID {
		t.Fatalf("runtime calls = %#v, want [%q]", runtime.calls, stepID)
	}
}

func TestRuntimeOrchestrator_StepDispatchEvidenceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 3)
	release := make(chan struct{})
	toolCallRepo := &blockingToolCallRepository{
		inner:              memory.NewToolCallRepository(),
		started:            started,
		release:            release,
		blockRunningCreate: true,
	}
	checkpointRepo := &blockingRuntimeCheckpointRepository{
		inner:   memory.NewRuntimeCheckpointRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
	}
	run := &runmodel.Run{ID: "run-dispatch-evidence-parallel", WorkspaceID: "ws-dispatch-evidence-parallel", State: common.RunStateRunning}
	step := &runmodel.Step{
		ID:    "run-dispatch-evidence-parallel/step-1",
		RunID: "run-dispatch-evidence-parallel",
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
		Input: map[string]any{"seed": "yes"},
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.recordStepDispatchEvidence(ctx, run, step)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["tool-call-started-create"] || !seen["step-dispatched-checkpoint-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("dispatch evidence writes before release = %+v, want tool call and checkpoint writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record dispatch evidence: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("record dispatch evidence did not finish after release: %v", ctx.Err())
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateRunning {
		t.Fatalf("tool calls = %+v, want one running tool call", toolCalls)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].CurrentStepID != step.ID || checkpoints[0].Data["event"] != "step_dispatched" {
		t.Fatalf("checkpoints = %+v, want one step_dispatched checkpoint", checkpoints)
	}
}

func TestRuntimeAdapterRegistryDispatchesByStepKind(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	defaultRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "default"}}
	externalRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "external"}}
	registry := newRuntimeAdapterRegistryForTest(defaultRuntime)
	if err := registry.Register("external/mock", externalRuntime); err != nil {
		t.Fatalf("register external runtime: %v", err)
	}
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-registry")
	runID := common.RunID("run-runtime-registry")
	stepID := common.StepID("run-runtime-registry/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "external/mock", State: common.StepStateDispatched, Input: map[string]any{"seed": "yes"}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch next step: %v", err)
	}

	if len(externalRuntime.requests) != 1 {
		t.Fatalf("external runtime requests = %d, want 1", len(externalRuntime.requests))
	}
	request := externalRuntime.requests[0]
	if request.WorkspaceID != workspaceID || request.RunID != runID || request.StepID != stepID || request.Kind != "external/mock" || request.Input["seed"] != "yes" {
		t.Fatalf("request = %+v, want full runtime request", request)
	}
	if len(defaultRuntime.requests) != 0 {
		t.Fatalf("default runtime requests = %d, want 0", len(defaultRuntime.requests))
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.Output["status"] != "external" {
		t.Fatalf("step output = %+v, want external", step.Output)
	}
}

func TestRuntimeAdapterRegistryDoesNotFallbackForUnknownRuntimeKind(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	defaultRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "default"}}
	registry := newRuntimeAdapterRegistryForTest(defaultRuntime)
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-unknown")
	runID := common.RunID("run-runtime-unknown")
	stepID := common.StepID("run-runtime-unknown/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "mcp/tool_call", State: common.StepStateDispatched}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch unknown runtime should close as failed without returning error: %v", err)
	}

	if len(defaultRuntime.requests) != 0 {
		t.Fatalf("default runtime requests = %d, want 0", len(defaultRuntime.requests))
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed ||
		step.Output["stage"] != "runtime_adapter_unavailable" ||
		step.Output["kind"] != "mcp/tool_call" {
		t.Fatalf("step = %+v, want adapter unavailable failure", step)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepRunsShellCommandAdapter(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	deliverableDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-shell")
	runID := common.RunID("run-shell")
	stepID := common.StepID("run-shell/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{"command": "pwd"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell step: %v", err)
	}

	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", step.Output["exitCode"])
	}
	stdout, _ := step.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want %q", stdout, deliverableDirectory)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepRunsMCPHTTPAdapter(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode mcp request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"summary": "mcp result"})
	}))
	defer server.Close()

	mcpService := mcpbiz.NewService(memory.NewMCPServerRepository(), memory.NewMCPCapabilityRepository())
	if err := mcpService.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-http"),
		Name:          "http tool",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(mcpblockcommon.StepKindToolCall, mcpblockbiz.NewRuntimeAdapter(mcpService)); err != nil {
		t.Fatalf("register mcp runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-mcp")
	runID := common.RunID("run-mcp")
	stepID := common.StepID("run-mcp/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  mcpblockcommon.StepKindToolCall,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"serverId": "mcp-http",
			"tool":     "repo.summary",
			"input":    map[string]any{"path": "."},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch mcp step: %v", err)
	}

	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Output["stage"] != "mcp_tool_call_completed" {
		t.Fatalf("step = %+v, want completed mcp step", step)
	}
	if captured["workspaceId"] != "ws-mcp" || captured["tool"] != "repo.summary" {
		t.Fatalf("captured = %+v, want workorch mcp payload", captured)
	}
	toolCall, err := toolCallRepo.Get(ctx, common.ToolCallID("run-mcp/step-1/tool-call/runtime-execute"))
	if err != nil {
		t.Fatalf("get tool call: %v", err)
	}
	if toolCall.State != common.ToolCallStateCompleted || toolCall.ToolName != mcpblockcommon.StepKindToolCall {
		t.Fatalf("toolCall = %+v, want completed mcp evidence", toolCall)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepPreservesShellFailureEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-shell-failed")
	runID := common.RunID("run-shell-failed")
	stepID := common.StepID("run-shell-failed/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "echo shell-out; echo shell-err >&2; exit 7"},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell step should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || step.Output["status"] != "failed" || step.Output["exitCode"] != 7 || step.Output["reason"] == "" {
		t.Fatalf("step = %+v, want failed shell output with exit evidence", step)
	}
	stderr, _ := step.Output["stderr"].(string)
	if !strings.Contains(stderr, "shell-err") {
		t.Fatalf("step stderr = %q, want shell-err", stderr)
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Output["exitCode"] != 7 {
		t.Fatalf("tool calls = %+v, want failed tool call with shell output", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStateFailed || validations[0].EvidenceRef != string(toolCalls[0].ID) {
		t.Fatalf("validations = %+v, want failed validation linked to tool call", validations)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepPreservesShellTimeoutEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapterWithTimeout(10*time.Millisecond)); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-shell-timeout")
	runID := common.RunID("run-shell-timeout")
	stepID := common.StepID("run-shell-timeout/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 1"},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell timeout should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || step.Output["status"] != "timeout" || step.Output["reason"] == "" {
		t.Fatalf("step = %+v, want timeout output", step)
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Output["status"] != "timeout" {
		t.Fatalf("tool calls = %+v, want failed timeout evidence", toolCalls)
	}
}

func TestRuntimeOrchestrator_StartRunRecordsExecutionEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-evidence")
	runID := common.RunID("run-evidence")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateCompleted || toolCalls[0].Output["status"] != "ok" {
		t.Fatalf("tool calls = %+v, want one completed call with output", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStatePassed || validations[0].EvidenceRef != string(toolCalls[0].ID) {
		t.Fatalf("validations = %+v, want one passed validation linked to tool call", validations)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasRunCheckpointState(checkpoints, common.RunStateRunning) ||
		!hasRunCheckpointState(checkpoints, common.RunStateValidating) ||
		!hasRunCheckpointState(checkpoints, common.RunStateCompleted) ||
		!hasCheckpointEvent(checkpoints, "step_dispatched") ||
		!hasCheckpointEvent(checkpoints, "step_completed") {
		t.Fatalf("checkpoints = %+v, want run and step execution checkpoints", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunRecordsRuntimeStepProgressCheckpoint(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if request.Progress == nil {
				t.Fatal("request.Progress is nil, want runtime progress recorder")
			}
			if err := request.Progress(runtimemodel.StepExecutionProgress{
				Event:   "runtime_step_progress",
				Stage:   "llm_provider_stream",
				Summary: "provider stream receiving",
				Data: map[string]any{
					"streaming":          true,
					"receivedDeltaCount": 2,
					"receivedBytes":      42,
				},
			}); err != nil {
				t.Fatalf("record progress: %v", err)
			}
			return map[string]any{"status": "ok"}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-progress-evidence")
	runID := common.RunID("run-progress-evidence")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointEvent(checkpoints, "runtime_step_progress") {
		t.Fatalf("checkpoints = %+v, want runtime_step_progress checkpoint", checkpoints)
	}
	if !hasRuntimeProgressCheckpointData(checkpoints, "llm_provider_stream", 42) {
		t.Fatalf("checkpoints = %+v, want provider stream progress diagnostics", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunRecordsFailedExecutionEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime failed")}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-evidence-failed")
	runID := common.RunID("run-evidence-failed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Error != "runtime failed" {
		t.Fatalf("tool calls = %+v, want one failed call", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStateFailed {
		t.Fatalf("validations = %+v, want one failed validation", validations)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasRunCheckpointState(checkpoints, common.RunStateFailed) || !hasCheckpointEvent(checkpoints, "step_failed") {
		t.Fatalf("checkpoints = %+v, want failed run and step checkpoints", checkpoints)
	}
}

func hasRuntimeProgressCheckpointData(items []*runmodel.RuntimeCheckpoint, stage string, receivedBytes int) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["event"] == "runtime_step_progress" &&
			item.Data["stage"] == stage &&
			item.Data["receivedBytes"] == receivedBytes {
			return true
		}
	}
	return false
}

func TestRuntimeOrchestrator_StartRunAppliesRuntimePatchEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"targetRef": "docs/result.txt",
				"content":   "runtime patch applied\n",
				"summary":   "write runtime result",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	deliverableDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-patch")
	runID := common.RunID("run-patch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(deliverableDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read applied patch file: %v", err)
	}
	if string(content) != "runtime patch applied\n" {
		t.Fatalf("applied file content = %q, want runtime patch applied", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" || patches[0].ToolCallID == "" {
		t.Fatalf("patches = %+v, want one applied patch linked to tool call", patches)
	}
}

func TestRuntimeOrchestrator_DispatchRejectsCoverageBoundPatchBeforeApply(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	deliverableDirectory := t.TempDir()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if request.Kind == llmWorkspaceExecutionStepKind {
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				stepOutputKeyExecutionGraph: map[string]any{
					"nodes": []map[string]any{{
						"id":   "repair-main-go",
						"kind": "patch/apply",
						"input": map[string]any{
							"patchType": "file_write",
							"targetRef": "main.go",
							"content":   "package main\n",
							"summary":   "repair main entry",
						},
					}},
				},
			}, nil
		}
		return map[string]any{"status": "completed"}, nil
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-coverage-preapply")
	runID := common.RunID("run-coverage-preapply")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, common.StepID(string(runID)+"/llm-coverage-root"), llmWorkspaceExecutionStepKind, map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"missingDoDItems": []map[string]any{
			{"item": "main entry covers request path", "state": "failed"},
		},
	}, &workspacemodel.Workspace{
		ID:                   workspaceID,
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateRunning,
	})
	if err != nil && !strings.Contains(err.Error(), "coverageTargets") {
		t.Fatalf("dispatch err = %v, want nil or coverageTargets recovery error", err)
	}
	if _, statErr := os.Stat(filepath.Join(deliverableDirectory, "main.go")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("main.go stat err = %v, want file not written", statErr)
	}
	patches, listErr := patchRepo.ListByRun(ctx, runID)
	if listErr != nil {
		t.Fatalf("list patches: %v", listErr)
	}
	if len(patches) != 0 {
		t.Fatalf("patches = %+v, want no patch evidence before contract passes", patches)
	}
	step, getErr := stepRepo.Get(ctx, common.StepID(string(runID)+"/llm-coverage-root"))
	if getErr != nil {
		t.Fatalf("get step: %v", getErr)
	}
	if step.State != common.StepStateFailed || !strings.Contains(stringFromMap(step.Output, "error"), "coverageTargets") {
		t.Fatalf("step = %+v, want failed before patch apply with coverageTargets error", step)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesPatchApplyGraphNode(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	toolCallRepo := memory.NewToolCallRepository()
	deliverableDirectory := t.TempDir()

	llmRuntime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-patch-graph/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-result",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "docs/result.txt",
								"content":   "graph patch applied\n",
								"summary":   "write graph result",
							},
						},
						{
							"id":        "verify-result",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-result"},
							"input": map[string]any{
								"command": "test",
								"args":    []string{"-f", "docs/result.txt"},
							},
						},
					},
				},
			}, nil
		case boolValue(request.Input["graphFollowUp"]) && request.Input["graphPreviousNodeId"] == "verify-result":
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "patch graph completed",
					"items": []map[string]any{
						{
							"item":         "result written",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-patch-graph/block/write-result/patch/1"},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected llm step %s", request.StepID)
		}
	}}
	shellRuntime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if _, err := os.Stat(filepath.Join(request.WorkspaceDeliverableDirectory, "docs", "result.txt")); err != nil {
			return map[string]any{"status": "failed", "error": err.Error()}, err
		}
		return map[string]any{"status": "completed", "stage": "shell_command_completed"}, nil
	}}
	registry := NewRuntimeAdapterRegistry()
	if err := registry.Register(llmcommon.StepKindWorkspaceExecution, llmRuntime); err != nil {
		t.Fatalf("register llm runtime: %v", err)
	}
	if err := registry.Register(llmcommon.RuntimeBlockKindPatchApply, NewPatchRuntimeAdapter()); err != nil {
		t.Fatalf("register patch runtime: %v", err)
	}
	if err := registry.Register(shellcommon.StepKindCommand, shellRuntime); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-graph")
	runID := common.RunID("run-patch-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-graph"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(deliverableDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read graph patch file: %v", err)
	}
	if string(content) != "graph patch applied\n" {
		t.Fatalf("graph patch file = %q, want applied content", string(content))
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" {
		t.Fatalf("patches = %+v, want applied graph patch", patches)
	}
}
