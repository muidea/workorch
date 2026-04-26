package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type stubRuntimeAdapter struct {
	output map[string]any
	err    error
	calls  []common.StepID
	fn     func(common.StepID) (map[string]any, error)
}

func (s *stubRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	stepID := request.StepID
	s.calls = append(s.calls, stepID)
	if s.fn != nil {
		output, err := s.fn(stepID)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if s.err != nil {
		return runtimemodel.StepExecutionResult{}, s.err
	}
	return runtimemodel.StepExecutionResult{Output: s.output}, nil
}

type requestCapturingRuntimeAdapter struct {
	requests []runtimemodel.StepExecutionRequest
	output   map[string]any
}

func (a *requestCapturingRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.requests = append(a.requests, request)
	return runtimemodel.StepExecutionResult{Output: a.output}, nil
}

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
	if len(runtime.calls) != 1 || runtime.calls[0] != stepID {
		t.Fatalf("runtime calls = %#v, want [%q]", runtime.calls, stepID)
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
	nonLegacyRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "non-legacy"}}
	registry := NewRuntimeAdapterRegistry(defaultRuntime)
	if err := registry.Register("external/mock", nonLegacyRuntime); err != nil {
		t.Fatalf("register non-legacy runtime: %v", err)
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

	if len(nonLegacyRuntime.requests) != 1 {
		t.Fatalf("non legacy requests = %d, want 1", len(nonLegacyRuntime.requests))
	}
	request := nonLegacyRuntime.requests[0]
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
	if step.Output["status"] != "non-legacy" {
		t.Fatalf("step output = %+v, want non-legacy", step.Output)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepRunsShellCommandAdapter(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	registry := NewRuntimeAdapterRegistry(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workingDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-shell")
	runID := common.RunID("run-shell")
	stepID := common.StepID("run-shell/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
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
	if strings.TrimSpace(stdout) != workingDirectory {
		t.Fatalf("stdout = %q, want %q", stdout, workingDirectory)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
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
	registry := NewRuntimeAdapterRegistry(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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
	registry := NewRuntimeAdapterRegistry(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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

	workingDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-patch")
	runID := common.RunID("run-patch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workingDirectory, "docs", "result.txt"))
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

func TestRuntimeOrchestrator_StartRunAppliesUnifiedDiffPatchEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	targetPath := filepath.Join(workingDirectory, "docs", "result.txt")
	if err := os.WriteFile(targetPath, []byte("before\nkeep\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1,2 +1,2 @@\n-before\n+after\n keep\n",
				"summary":   "apply unified diff",
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
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff")
	runID := common.RunID("run-unified-diff")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(content) != "after\nkeep\n" {
		t.Fatalf("target content = %q, want unified diff result", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].PatchType != "unified_diff" || patches[0].State != common.PatchStateApplied {
		t.Fatalf("patches = %+v, want applied unified diff patch", patches)
	}
}

func TestRuntimeOrchestrator_StartRunPersistsUnifiedDiffPatchEvidenceWithFileBackend(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceRepo := filerepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspaces.json"))
	runRepo := filerepo.NewRunRepository(filepath.Join(dataRoot, "runs.json"))
	stepRepo := filerepo.NewStepRepository(filepath.Join(dataRoot, "steps.json"))
	ticketRepo := filerepo.NewDecisionTicketRepository(filepath.Join(dataRoot, "decision_tickets.json"))
	resolutionRepo := filerepo.NewDecisionResolutionRepository(filepath.Join(dataRoot, "decision_resolutions.json"))
	approvalRepo := filerepo.NewApprovalRepository(filepath.Join(dataRoot, "approvals.json"))
	patchPath := filepath.Join(dataRoot, "patches.json")
	patchRepo := filerepo.NewPatchRepository(patchPath)
	workingDirectory := filepath.Join(dataRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1 +1 @@\n-before\n+after\n",
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
			ToolCalls:          filerepo.NewToolCallRepository(filepath.Join(dataRoot, "tool_calls.json")),
			Patches:            patchRepo,
			ValidationResults:  filerepo.NewValidationResultRepository(filepath.Join(dataRoot, "validation_results.json")),
			RuntimeCheckpoints: filerepo.NewRuntimeCheckpointRepository(filepath.Join(dataRoot, "runtime_checkpoints.json")),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff-file")
	runID := common.RunID("run-unified-diff-file")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	reloadedPatchRepo := filerepo.NewPatchRepository(patchPath)
	patches, err := reloadedPatchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list reloaded patches: %v", err)
	}
	if len(patches) != 1 || patches[0].PatchType != "unified_diff" || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" {
		t.Fatalf("reloaded patches = %+v, want persisted applied unified diff patch", patches)
	}
}

func TestRuntimeOrchestrator_StartRunFailsInvalidUnifiedDiffPatch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1 +1 @@\n-missing\n+after\n",
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
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-unified-diff-invalid")
	runID := common.RunID("run-unified-diff-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateFailed {
		t.Fatalf("patches = %+v, want failed unified diff patch", patches)
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(content) != "before\n" {
		t.Fatalf("target content = %q, want unchanged content", string(content))
	}
}

func TestRuntimeOrchestrator_StartRunFailsInvalidRuntimePatchTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"targetRef": "../escape.txt",
				"content":   "invalid\n",
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
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-invalid")
	runID := common.RunID("run-patch-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateFailed {
		t.Fatalf("patches = %+v, want one failed patch", patches)
	}
}

func TestRuntimeOrchestrator_StartRunPublishesRunAndStepEvents(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-evt")
	runID := common.RunID("run-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := hub.Published()
	if len(published) < 6 {
		t.Fatalf("published events = %d, want at least 6", len(published))
	}
	if published[0].Version != events.WorkorchEventVersion {
		t.Fatalf("first event version = %s, want %s", published[0].Version, events.WorkorchEventVersion)
	}
	if published[0].Topic != events.TopicRunStateChanged || published[0].Payload[events.PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("first event = %#v, want run running", published[0])
	}
	if published[1].Topic != events.TopicWorkspaceStateChanged || published[1].Payload[events.PayloadKeyActiveRunID] != string(runID) {
		t.Fatalf("second event = %#v, want workspace active run bound", published[1])
	}
	if published[2].Topic != events.TopicStepDispatched || published[2].Payload[events.PayloadKeyState] != string(common.StepStateDispatched) {
		t.Fatalf("third event = %#v, want step dispatched", published[2])
	}
	if published[3].Topic != events.TopicStepStateChanged || published[3].Payload[events.PayloadKeyState] != string(common.StepStateCompleted) {
		t.Fatalf("fourth event = %#v, want step completed", published[3])
	}
	if published[4].Topic != events.TopicRunStateChanged || published[4].Payload[events.PayloadKeyState] != string(common.RunStateValidating) {
		t.Fatalf("fifth event = %#v, want run validating", published[4])
	}
	if published[5].Topic != events.TopicRunStateChanged || published[5].Payload[events.PayloadKeyState] != string(common.RunStateCompleted) {
		t.Fatalf("sixth event = %#v, want run completed", published[5])
	}
	last := published[len(published)-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace active run cleared", last)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesMultipleStepsWhenRuntimeRequestsNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi/step-1":
				return map[string]any{
					"status":   "completed",
					"nextStep": "run-multi/step-2",
					"nextKind": "legacy/minimal",
					"stage":    "prepare",
				}, nil
			case "run-multi/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi")
	runID := common.RunID("run-multi")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start multi-step run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-multi/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateCompleted {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if step1.Output["nextStep"] != "run-multi/step-2" {
		t.Fatalf("step1 nextStep = %v, want run-multi/step-2", step1.Output["nextStep"])
	}

	step2, err := stepRepo.Get(ctx, "run-multi/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-multi/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-multi/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-multi/step-1" || runtime.calls[1] != "run-multi/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-multi/step-1 run-multi/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunExecutesBlockExecutionGraphBranch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "build", Kind: "graph/mock", DependsOn: []string{"prepare"}, Branch: "ship"},
		{ID: "reject", Kind: "graph/mock", DependsOn: []string{"prepare"}, Branch: "reject"},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-branch/block/prepare":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
					"graphBranch":    "ship",
				}, nil
			case "run-graph-branch/block/build":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-branch")
	runID := common.RunID("run-graph-branch")
	prepareStepID := common.StepID("run-graph-branch/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-graph-branch/block/prepare" || runtime.calls[1] != "run-graph-branch/block/build" {
		t.Fatalf("runtime calls = %#v, want prepare then build only", runtime.calls)
	}
	if _, err := stepRepo.Get(ctx, "run-graph-branch/block/reject"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("reject branch step err = %v, want not found", err)
	}
	buildStep, err := stepRepo.Get(ctx, "run-graph-branch/block/build")
	if err != nil {
		t.Fatalf("get build step: %v", err)
	}
	if buildStep.Input["graphBranch"] != "ship" || buildStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("build input = %+v, want graph branch and previous node", buildStep.Input)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointGraphBranch(checkpoints, "ship") {
		t.Fatalf("checkpoints = %+v, want graph branch evidence", checkpoints)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesBlockExecutionGraphAfterDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "review", Kind: "graph/mock", DependsOn: []string{"prepare"}},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-decision/block/prepare":
				return map[string]any{
					"status":              "completed",
					"executionGraph":      graph,
					"blockDecisionReason": "need operator input",
				}, nil
			case "run-graph-decision/block/review":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
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
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-decision")
	runID := common.RunID("run-graph-decision")
	prepareStepID := common.StepID("run-graph-decision/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}
	blockedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blockedRun.State != common.RunStateBlockedOnDecision || blockedRun.BlockingTicketID == "" {
		t.Fatalf("blocked run = %+v, want decision block", blockedRun)
	}
	ticket, err := ticketRepo.Get(ctx, blockedRun.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-graph-decision"),
		TicketID: ticket.ID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	reviewStep, err := stepRepo.Get(ctx, "run-graph-decision/block/review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if reviewStep.State != common.StepStateCompleted || reviewStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("review step = %+v, want resumed graph node after prepare", reviewStep)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesBlockExecutionGraphAfterApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "apply", Kind: "graph/mock", DependsOn: []string{"prepare"}},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-approval/block/prepare":
				return map[string]any{
					"status":              "completed",
					"executionGraph":      graph,
					"blockApprovalReason": "requires approval",
				}, nil
			case "run-graph-approval/block/apply":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-approval")
	runID := common.RunID("run-graph-approval")
	prepareStepID := common.StepID("run-graph-approval/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}
	blockedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blockedRun.State != common.RunStateBlockedOnApproval || blockedRun.BlockingApprovalID == "" {
		t.Fatalf("blocked run = %+v, want approval block", blockedRun)
	}
	approval, err := approvalRepo.Get(ctx, blockedRun.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("approve item: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	applyStep, err := stepRepo.Get(ctx, "run-graph-approval/block/apply")
	if err != nil {
		t.Fatalf("get apply step: %v", err)
	}
	if applyStep.State != common.StepStateCompleted || applyStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("apply step = %+v, want resumed graph node after prepare", applyStep)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalBranchStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "fast-track",
					"nextStep": "run-branch/step-fast",
					"nextKind": "legacy/minimal",
				}, nil
			case "run-branch/step-fast":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch")
	runID := common.RunID("run-branch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "fast-track" {
		t.Fatalf("step1 branch = %v, want fast-track", step1.Output["branch"])
	}

	stepFast, err := stepRepo.Get(ctx, "run-branch/step-fast")
	if err != nil {
		t.Fatalf("get branch step: %v", err)
	}
	if stepFast.State != common.StepStateCompleted {
		t.Fatalf("branch step state = %s, want completed", stepFast.State)
	}
	if stepFast.Input["previousStepId"] != "run-branch/step-1" {
		t.Fatalf("branch step previousStepId = %v, want run-branch/step-1", stepFast.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch/step-1" || runtime.calls[1] != "run-branch/step-fast" {
		t.Fatalf("runtime calls = %#v, want [run-branch/step-1 run-branch/step-fast]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalReviewStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch-review/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "review-path",
					"nextStep": "run-branch-review/step-review",
					"nextKind": "legacy/minimal",
				}, nil
			case "run-branch-review/step-review":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch-review")
	runID := common.RunID("run-branch-review")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch review run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch-review/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "review-path" {
		t.Fatalf("step1 branch = %v, want review-path", step1.Output["branch"])
	}

	stepReview, err := stepRepo.Get(ctx, "run-branch-review/step-review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if stepReview.State != common.StepStateCompleted {
		t.Fatalf("review step state = %s, want completed", stepReview.State)
	}
	if stepReview.Input["previousStepId"] != "run-branch-review/step-1" {
		t.Fatalf("review step previousStepId = %v, want run-branch-review/step-1", stepReview.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch-review/step-1" || runtime.calls[1] != "run-branch-review/step-review" {
		t.Fatalf("runtime calls = %#v, want [run-branch-review/step-1 run-branch-review/step-review]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunBlocksOnDecisionWhenRuntimeRequestsDecisionGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		output: map[string]any{
			"status":                           "completed",
			stepOutputKeyBlockDecisionReason:   "runtime decision gate",
			stepOutputKeyBlockDecisionTicketID: "ticket-runtime-block",
			stepOutputKeyNextStep:              "run-runtime-block/step-2",
			stepOutputKeyNextKind:              "legacy/minimal",
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-block")
	runID := common.RunID("run-runtime-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision {
		t.Fatalf("run state = %s, want blocked_on_decision", runItem.State)
	}
	if runItem.BlockingTicketID != "ticket-runtime-block" {
		t.Fatalf("blocking ticket = %s, want ticket-runtime-block", runItem.BlockingTicketID)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-block")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", ticket.State)
	}
	if ticket.StepID != "run-runtime-block/step-1" {
		t.Fatalf("ticket step id = %s, want run-runtime-block/step-1", ticket.StepID)
	}

	step1, err := stepRepo.Get(ctx, "run-runtime-block/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateCompleted {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-block/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step2 err = %v, want not found", err)
	}

	published := hub.Published()
	foundTicketCreated := false
	for _, envelope := range published {
		if envelope.Topic == events.TopicTicketCreated && envelope.SubjectID == "ticket-runtime-block" {
			foundTicketCreated = true
			break
		}
	}
	if !foundTicketCreated {
		t.Fatalf("published events = %#v, want ticket created event", published)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeDecisionResolved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-resume/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-resume",
					stepOutputKeyNextStep:              "run-runtime-resume/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-runtime-resume/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-resume")
	runID := common.RunID("run-runtime-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-resume")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-resume"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-resume/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-resume/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-resume/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-resume/step-1" || runtime.calls[1] != "run-runtime-resume/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-resume/step-1 run-runtime-resume/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeDecisionWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-decision-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-decision-reject",
					stepOutputKeyNextStep:              "run-runtime-decision-reject/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-decision-reject")
	runID := common.RunID("run-runtime-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision reject run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-decision-reject")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-decision-reject"),
		TicketID: ticket.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester", "outcome": "reject"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", runItem.BlockingTicketID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-decision-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-decision-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-decision-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeApprovalApproved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "runtime approval gate",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval",
					stepOutputKeyNextStep:            "run-runtime-approval/step-2",
					stepOutputKeyNextKind:            "legacy/minimal",
				}, nil
			case "run-runtime-approval/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval")
	runID := common.RunID("run-runtime-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-approval/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-approval/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-approval/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-approval/step-1" || runtime.calls[1] != "run-runtime-approval/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval/step-1 run-runtime-approval/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeApprovalWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval-reject/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "runtime approval gate",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval-reject",
					stepOutputKeyNextStep:            "run-runtime-approval-reject/step-2",
					stepOutputKeyNextKind:            "legacy/minimal",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval-reject")
	runID := common.RunID("run-runtime-approval-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval reject run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval-reject")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime approval: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-approval-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-approval-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsMultipleDecisionGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-1",
					stepOutputKeyNextStep:              "run-multi-gate/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-multi-gate/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-2",
					stepOutputKeyNextStep:              "run-multi-gate/step-3",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-multi-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate")
	runID := common.RunID("run-multi-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	resolveTicket := func(ticketID common.TicketID, resolutionID common.ResolutionID) {
		t.Helper()
		ticket, err := ticketRepo.Get(ctx, ticketID)
		if err != nil {
			t.Fatalf("get ticket %s: %v", ticketID, err)
		}
		ticket.State = common.DecisionTicketStateResolved
		if err := ticketRepo.Update(ctx, ticket); err != nil {
			t.Fatalf("update ticket %s: %v", ticketID, err)
		}
		if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
			ID:       resolutionID,
			TicketID: ticketID,
			Type:     "provide_missing_info",
			Payload:  map[string]any{"operator": "tester"},
		}); err != nil {
			t.Fatalf("create resolution for %s: %v", ticketID, err)
		}
	}

	resolveTicket("ticket-multi-gate-1", "resolution-multi-gate-1")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after first resume: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID != "ticket-multi-gate-2" {
		t.Fatalf("run after first resume = %+v, want blocked on second gate", runItem)
	}

	resolveTicket("ticket-multi-gate-2", "resolution-multi-gate-2")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after second gate: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-multi-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateCompleted {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-multi-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-multi-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-multi-gate/step-1" || runtime.calls[1] != "run-multi-gate/step-2" || runtime.calls[2] != "run-multi-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate/step-1 run-multi-gate/step-2 run-multi-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsSecondDecisionGateWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-1",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-multi-gate-reject/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-2",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-3",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate-reject")
	runID := common.RunID("run-multi-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket1, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket1: %v", err)
	}
	ticket1.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket1); err != nil {
		t.Fatalf("update ticket1: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-1"),
		TicketID: ticket1.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution1: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	ticket2, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-2")
	if err != nil {
		t.Fatalf("get ticket2: %v", err)
	}
	ticket2.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket2); err != nil {
		t.Fatalf("update ticket2: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-2"),
		TicketID: ticket2.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution2: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected second gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-multi-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-multi-gate-reject/step-1" || runtime.calls[1] != "run-multi-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate-reject/step-1 run-multi-gate-reject/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenApprovalGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-1",
					stepOutputKeyNextStep:              "run-mixed-gate/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-mixed-gate/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-1",
					stepOutputKeyNextStep:            "run-mixed-gate/step-3",
					stepOutputKeyNextKind:            "legacy/minimal",
				}, nil
			case "run-mixed-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate")
	runID := common.RunID("run-mixed-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-mixed-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateCompleted {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-mixed-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-mixed-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-mixed-gate/step-1" || runtime.calls[1] != "run-mixed-gate/step-2" || runtime.calls[2] != "run-mixed-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate/step-1 run-mixed-gate/step-2 run-mixed-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenRejectedApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-reject-1",
					stepOutputKeyNextStep:              "run-mixed-gate-reject/step-2",
					stepOutputKeyNextKind:              "legacy/minimal",
				}, nil
			case "run-mixed-gate-reject/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-reject-1",
					stepOutputKeyNextStep:            "run-mixed-gate-reject/step-3",
					stepOutputKeyNextKind:            "legacy/minimal",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate-reject")
	runID := common.RunID("run-mixed-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-reject-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-reject-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after rejected approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}

	step2, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-mixed-gate-reject/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-mixed-gate-reject/step-1", step2.Input["previousStepId"])
	}
	if _, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-mixed-gate-reject/step-1" || runtime.calls[1] != "run-mixed-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate-reject/step-1 run-mixed-gate-reject/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_RuntimeFailurePublishesWorkspaceReleaseEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-fail-evt")
	runID := common.RunID("run-fail-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := hub.Published()
	if len(published) < 5 {
		t.Fatalf("published events = %d, want at least 5", len(published))
	}
	last := published[len(published)-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload["activeRunId"] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_StartRunRejectsDifferentActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-active-conflict")
	runID := common.RunID("run-active-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("other-run"),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("start run err = %v, want conflict", err)
	}
}

func TestRuntimeOrchestrator_StartRunRejectsNonCreatedState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-start-state-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state common.RunState
	}{
		{name: "running", state: common.RunStateRunning},
		{name: "blocked decision", state: common.RunStateBlockedOnDecision},
		{name: "blocked approval", state: common.RunStateBlockedOnApproval},
		{name: "completed", state: common.RunStateCompleted},
		{name: "failed", state: common.RunStateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := common.RunID("run-start-state-conflict-" + tc.name)
			if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tc.state}); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("start run err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_StartRun_RuntimeFailureMarksRunAndStepFailed(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
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
		t.Fatalf("start run should close failure state in-repo, got err: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}

	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed {
		t.Fatalf("step state = %q, want failed", step.State)
	}
	if step.Output["reason"] != "runtime exploded" {
		t.Fatalf("step output reason = %v, want runtime exploded", step.Output["reason"])
	}
}

func TestRuntimeOrchestrator_DispatchNextStep_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()

	orch := NewRuntimeOrchestrator(nil, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)
	if err := orch.DispatchNextStep(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty runID err = %v, want invalid argument", err)
	}

	runID := common.RunID("run-no-runtime")
	workspaceID := common.WorkspaceID("ws-no-runtime")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := orch.DispatchNextStep(ctx, runID); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil runtime err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_BlockCommands_ValidateArgumentsAndSetExpectedRefs(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.BlockOnDecision(ctx, "", common.TicketID("ticket-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnDecision(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty ticket err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, "", common.ApprovalID("approval-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty approval err = %v, want invalid argument", err)
	}

	workspaceID := common.WorkspaceID("ws-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	decisionRunID := common.RunID("run-block-decision")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          decisionRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("seed decision run: %v", err)
	}
	if err := orch.BlockOnDecision(ctx, decisionRunID, common.TicketID("ticket-1")); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	blockedDecision, err := runRepo.Get(ctx, decisionRunID)
	if err != nil {
		t.Fatalf("get decision-blocked run: %v", err)
	}
	if blockedDecision.BlockingTicketID != common.TicketID("ticket-1") || blockedDecision.BlockingApprovalID != "" {
		t.Fatalf("unexpected decision block refs: %+v", blockedDecision)
	}

	approvalRunID := common.RunID("run-block-approval")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          approvalRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed approval run: %v", err)
	}
	if err := orch.BlockOnApproval(ctx, approvalRunID, common.ApprovalID("approval-1")); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	blockedApproval, err := runRepo.Get(ctx, approvalRunID)
	if err != nil {
		t.Fatalf("get approval-blocked run: %v", err)
	}
	if blockedApproval.BlockingApprovalID != common.ApprovalID("approval-1") || blockedApproval.BlockingTicketID != "" {
		t.Fatalf("unexpected approval block refs: %+v", blockedApproval)
	}
}

func TestRuntimeOrchestrator_BlockCommandsRejectBlockedOrTerminalRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-block-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "already blocked on decision",
			run: &runmodel.Run{
				ID:               common.RunID("run-block-conflict-decision"),
				WorkspaceID:      workspaceID,
				State:            common.RunStateBlockedOnDecision,
				BlockingTicketID: common.TicketID("ticket-existing"),
			},
		},
		{
			name: "already blocked on approval",
			run: &runmodel.Run{
				ID:                 common.RunID("run-block-conflict-approval"),
				WorkspaceID:        workspaceID,
				State:              common.RunStateBlockedOnApproval,
				BlockingApprovalID: common.ApprovalID("approval-existing"),
			},
		},
		{
			name: "completed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-completed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateCompleted,
			},
		},
		{
			name: "failed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-failed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateFailed,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.BlockOnDecision(ctx, tc.run.ID, common.TicketID("ticket-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on decision err = %v, want conflict", err)
			}
			if err := orch.BlockOnApproval(ctx, tc.run.ID, common.ApprovalID("approval-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on approval err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedTicket(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	ticketID := common.TicketID("ticket-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID})
	_ = ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by open ticket err = %v, want blocked", err)
	}

	ticket, _ := ticketRepo.Get(ctx, ticketID)
	ticket.State = common.DecisionTicketStateResolved
	_ = ticketRepo.Update(ctx, ticket)
	_ = resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-1"),
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after ticket resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", r.BlockingTicketID)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsDecisionWithoutDispatchingStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-decision-reject")
	runID := common.RunID("run-decision-reject")
	ticketID := common.TicketID("ticket-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-reject"),
		TicketID: ticketID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step, got %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
	last := hub.Published()[len(hub.Published())-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_ResumeRunReplanSupersedesRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-decision-replan")
	runID := common.RunID("run-decision-replan")
	ticketID := common.TicketID("ticket-decision-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-replan"),
		TicketID: ticketID,
		Type:     "replan",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume replan decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateSuperseded {
		t.Fatalf("run state = %q, want superseded", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want empty", ws.ActiveRunID)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedApproval(t *testing.T) {
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
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStatePending})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by pending approval err = %v, want blocked", err)
	}

	approval, _ := approvalRepo.Get(ctx, approvalID)
	approval.State = common.ApprovalStateApproved
	_ = approvalRepo.Update(ctx, approval)

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectedApprovalFailsRun(t *testing.T) {
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
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStateRejected})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval rejection should fail run in-repo, got err: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}

	ws, _ := workspaceRepo.Get(ctx, workspaceID)
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want no execution after rejection", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsMissingBlockingReference(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-missing-ref")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "decision missing ticket",
			run:  &runmodel.Run{ID: common.RunID("run-missing-ticket"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision},
		},
		{
			name: "approval missing approval",
			run:  &runmodel.Run{ID: common.RunID("run-missing-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.ResumeRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("resume err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_CompleteRunAndFailRun_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.CompleteRun(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("complete empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, "", "reason"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty reason err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_CompleteRunRejectsNonRunningState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-complete-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "created", run: &runmodel.Run{ID: common.RunID("run-complete-created"), WorkspaceID: workspaceID, State: common.RunStateCreated}},
		{name: "blocked decision", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-decision"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: common.TicketID("ticket-1")}},
		{name: "blocked approval", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: common.ApprovalID("approval-1")}},
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-complete-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-complete-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.CompleteRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("complete run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func TestRuntimeOrchestrator_FailRunRejectsTerminalState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-fail-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-fail-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-fail-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.FailRun(ctx, tc.run.ID, "manual fail"); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("fail run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func hasRunCheckpointState(items []*runmodel.RuntimeCheckpoint, state common.RunState) bool {
	for _, item := range items {
		if item != nil && item.RunStateSnapshot == state {
			return true
		}
	}
	return false
}

func hasCheckpointEvent(items []*runmodel.RuntimeCheckpoint, event string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["event"] == event {
			return true
		}
	}
	return false
}

func hasCheckpointGraphBranch(items []*runmodel.RuntimeCheckpoint, branch string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["graphBranch"] == branch {
			return true
		}
	}
	return false
}
