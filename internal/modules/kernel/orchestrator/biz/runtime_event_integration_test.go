package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_DispatchRequestedStepRefreshesAuthoritySnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-dispatch-authority-refresh")
	runID := common.RunID("run-dispatch-authority-refresh")
	stepID := common.StepID(runID + "/step-1")
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			"definitionRevision": "def-current",
			"dod": map[string]any{
				"id": "dod-dispatch-authority-refresh",
				"items": []map[string]any{{
					"id":                    "dispatch-authority-validation",
					"text":                  "current neutral coverage target",
					"requiredEvidenceKinds": []string{"validation"},
				}},
			},
			"convergenceContract": map[string]any{
				"state":                       convergenceStateNeedStateChange,
				"repairObjective":             repairObjectiveStateChangingOutput,
				"stateChangingOutputRequired": true,
			},
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository())

	err := orch.DispatchRequestedStep(ctx, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"workspaceId":   "stale-workspace",
				"runId":         "stale-run",
				"nextAction":    map[string]any{"kind": "stale_action"},
			},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"workspaceId":   "stale-workspace",
				"runId":         "stale-run",
				"nextAction":    map[string]any{"kind": "stale_action"},
			},
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
		},
	})
	if err != nil {
		t.Fatalf("DispatchRequestedStep: %v", err)
	}
	var requestInput map[string]any
	for _, request := range runtime.requests {
		if request.StepID == stepID {
			requestInput = request.Input
			break
		}
	}
	if len(requestInput) == 0 {
		t.Fatalf("runtime requests = %+v, want request for %s", runtime.requests, stepID)
	}
	input := requestInput
	baseline := mapFromValue(input[gapBaselineKey])
	if len(baseline) > 0 {
		if stringFromMap(baseline, "workspaceId") != string(workspaceID) || stringFromMap(baseline, "runId") != string(runID) {
			t.Fatalf("gapBaseline scope = %+v, want current workspace/run; input=%+v", baseline, input)
		}
		if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got == "" || got == "stale_action" {
			t.Fatalf("gapBaseline.nextAction.kind = %q, want current owner-derived action; input=%+v", got, input)
		} else if got != "validate" {
			t.Fatalf("gapBaseline.nextAction.kind = %q, want validate; input=%+v", got, input)
		}
	}
	authority := mapFromValue(input[convergenceAuthorityKey])
	if len(authority) > 0 {
		if stringFromMap(authority, "workspaceId") != string(workspaceID) || stringFromMap(authority, "runId") != string(runID) {
			t.Fatalf("convergenceAuthority scope = %+v, want current workspace/run; input=%+v", authority, input)
		}
		if got := stringFromMap(mapFromValue(authority["nextAction"]), "kind"); got == "" || got == "stale_action" {
			t.Fatalf("convergenceAuthority.nextAction.kind = %q, want current owner-derived action; input=%+v", got, input)
		} else if got != "validate" {
			t.Fatalf("convergenceAuthority.nextAction.kind = %q, want validate; input=%+v", got, input)
		}
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired leaked through event dispatch: %+v", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if got := stringFromMap(contract, "state"); got != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	if len(packet) == 0 {
		t.Fatalf("stagePacket missing from event dispatch input: %+v", input)
	}
	if got := stringFromMap(mapFromValue(packet["activeFocus"]), "id"); got != "cwi-001" {
		t.Fatalf("stagePacket.activeFocus = %+v, want cwi-001", packet["activeFocus"])
	}
}

func TestRequestDispatchStepRecordsEventPublishReadbackPending(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-dispatch-readback")
	runID := common.RunID("run-dispatch-readback")
	stepID := common.StepID(runID + "/step-1")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	hub, _ := newRecordingDispatchMagicHub(t)
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository())

	orch.EnableEventDrivenDispatch(hub.rawHub)

	err := orch.requestDispatchStep(ctx, runID, workspaceID, stepDispatchSpec{
		ID:    stepID,
		Kind:  "shell/command",
		Input: map[string]any{"command": "true"},
	})
	if err != nil {
		t.Fatalf("requestDispatchStep: %v", err)
	}
	if _, err := stepRepo.Get(ctx, stepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step readback err = %v, want not found without event consumer", err)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	readback := mapFromValue(persisted.ExecutionContext["dispatchReadback"])
	if readback["state"] != "pending_owner_consumption" ||
		readback["queuedCommandPublished"] != true ||
		readback["stepExists"] != false ||
		readback["stepId"] != string(stepID) {
		t.Fatalf("dispatchReadback = %+v, want queued pending readback", readback)
	}
}

func TestRecordDispatchPublishReadbackClearsPendingWhenStepExists(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-dispatch-readback-clear")
	runID := common.RunID("run-dispatch-readback-clear")
	stepID := common.StepID(runID + "/step-1")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			"dispatchReadback": map[string]any{
				"schemaVersion":          "workorch.dispatch_readback.v1",
				"state":                  "pending_owner_consumption",
				"queuedCommandPublished": true,
				"stepExists":             false,
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateExecuting,
		Input: map[string]any{"command": "true"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := &RuntimeOrchestrator{runs: runRepo, steps: stepRepo}

	err := orch.recordDispatchPublishReadback(ctx, run, orchestratorcontracts.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        "shell/command",
	})
	if err != nil {
		t.Fatalf("recordDispatchPublishReadback: %v", err)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if readback := mapFromValue(persisted.ExecutionContext["dispatchReadback"]); len(readback) != 0 {
		t.Fatalf("dispatchReadback = %+v, want cleared after owner step exists", readback)
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
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	targetPath := filepath.Join(deliverableDirectory, "docs", "result.txt")
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
		})

	workspaceID := common.WorkspaceID("ws-unified-diff")
	runID := common.RunID("run-unified-diff")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	if !strings.Contains(patches[0].Diff, "-before\n+after") {
		t.Fatalf("patch diff = %q, want persisted unified diff", patches[0].Diff)
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
	deliverableDirectory := filepath.Join(dataRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
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
		})

	workspaceID := common.WorkspaceID("ws-unified-diff-file")
	runID := common.RunID("run-unified-diff-file")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	if !strings.Contains(patches[0].Diff, "-before\n+after") {
		t.Fatalf("reloaded patch diff = %q, want persisted unified diff", patches[0].Diff)
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
		})

	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-unified-diff-invalid")
	runID := common.RunID("run-unified-diff-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || stringFromMap(step.Output, "stage") != "patch_apply" {
		t.Fatalf("step = %+v, want failed patch_apply block step", step)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 0 {
		t.Fatalf("patches = %+v, want no formal patch for failed block result", patches)
	}
	content, err := os.ReadFile(filepath.Join(deliverableDirectory, "docs", "result.txt"))
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
		})

	workspaceID := common.WorkspaceID("ws-patch-invalid")
	runID := common.RunID("run-patch-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || stringFromMap(step.Output, "stage") != "patch_apply" {
		t.Fatalf("step = %+v, want failed patch_apply block step", step)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 0 {
		t.Fatalf("patches = %+v, want no formal patch for failed block result", patches)
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	notifications := newRuntimeNotificationRecorder(t)
	notifications.attach(orch)

	workspaceID := common.WorkspaceID("ws-evt")
	runID := common.RunID("run-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := notifications.waitAtLeast(8, time.Second)
	if len(published) < 7 {
		t.Fatalf("published events = %d, want at least 7", len(published))
	}
	var runActive, workspaceBound, stepExecuting, stepSucceeded, runCompleted, workspaceReleased bool
	for _, item := range published {
		switch {
		case item.Topic == runevents.TopicRunStateChanged && item.RunID == string(runID) && item.State == string(common.RunStateActive):
			runActive = true
		case item.Topic == workspaceevents.TopicWorkspaceStateChanged && item.WorkspaceID == string(workspaceID) && item.ActiveRunID == string(runID):
			workspaceBound = true
		case item.Topic == runevents.TopicStepDispatched && item.RunID == string(runID) && item.State == string(common.StepStateExecuting):
			stepExecuting = true
		case item.Topic == runevents.TopicStepStateChanged && item.RunID == string(runID) && item.State == string(common.StepStateSucceeded):
			stepSucceeded = true
		case item.Topic == runevents.TopicRunStateChanged && item.RunID == string(runID) && item.State == string(common.RunStateCompleted):
			runCompleted = true
		case item.Topic == workspaceevents.TopicWorkspaceStateChanged && item.WorkspaceID == string(workspaceID) && item.ActiveRunID == "":
			workspaceReleased = true
		}
	}
	if !runActive || !workspaceBound || !stepExecuting || !stepSucceeded || !runCompleted || !workspaceReleased {
		t.Fatalf("published events = %+v, want run/workspace/step typed lifecycle notifications", published)
	}
}

func TestRuntimeOrchestrator_StartRunPublishesLLMStreamProgressEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"stage":  "llm_execution_graph_plan",
		"contextBudget": map[string]any{
			"providerId": "provider-a",
			"model":      "model-a",
		},
		"llmUsage": map[string]any{
			"providerId":    "provider-a",
			"provider":      "compatible",
			"model":         "model-a",
			"operation":     "chat_completions",
			"inputTokens":   100,
			"outputTokens":  20,
			"totalTokens":   120,
			"estimated":     true,
			"requestBytes":  1024,
			"responseBytes": 256,
			"streaming":     true,
			"status":        "completed",
		},
		"streamEvents": []map[string]any{
			{"type": "delta", "delta": "abc"},
			{"type": "delta", "delta": "def"},
			{"type": "done"},
		},
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	notifications := newRuntimeNotificationRecorder(t)
	notifications.attach(orch)

	workspaceID := common.WorkspaceID("ws-evt-stream")
	runID := common.RunID("run-evt-stream")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	records := notifications.waitAtLeast(10, time.Second)
	var streamProgress runtimeNotificationRecord
	found := false
	for _, item := range records {
		if item.Topic != runevents.TopicRunLLMStreamProgress {
			continue
		}
		streamProgress = item
		found = true
		break
	}
	if !found {
		t.Fatalf("published events missing %s", runevents.TopicRunLLMStreamProgress)
	}
	if streamProgress.WorkspaceID != string(workspaceID) || streamProgress.RunID != string(runID) {
		t.Fatalf("stream payload workspace/run = %+v, want %s/%s", streamProgress, workspaceID, runID)
	}
	if streamProgress.ChunkCount != 3 || streamProgress.ReceivedBytes != 6 {
		t.Fatalf("stream payload chunk/bytes = %+v, want 3/6", streamProgress)
	}
	if streamProgress.ProviderID != "provider-a" || streamProgress.Model != "model-a" || streamProgress.State != string(common.StepStateSucceeded) {
		t.Fatalf("stream payload provider/model/status = %+v", streamProgress)
	}

	var usageRecorded runtimeNotificationRecord
	usageFound := false
	for _, item := range records {
		if item.Topic != runevents.TopicRunLLMUsageRecorded {
			continue
		}
		usageRecorded = item
		usageFound = true
		break
	}
	if !usageFound {
		t.Fatalf("published events missing %s", runevents.TopicRunLLMUsageRecorded)
	}
	if usageRecorded.WorkspaceID != string(workspaceID) || usageRecorded.RunID != string(runID) || usageRecorded.StepID == "" {
		t.Fatalf("usage payload workspace/run/step = %+v", usageRecorded)
	}
	if usageRecorded.ProviderID != "provider-a" || usageRecorded.Model != "model-a" || !usageRecorded.Streaming {
		t.Fatalf("usage payload provider/model/streaming = %+v", usageRecorded)
	}
	if usageRecorded.State != "completed" || usageRecorded.TotalTokens == 0 {
		t.Fatalf("usage payload status/tokens = %+v", usageRecorded)
	}
}

func TestRuntimeOrchestrator_CompleteRunClearsStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	notifications := newRuntimeNotificationRecorder(t)
	notifications.attach(orch)

	workspaceID := common.WorkspaceID("ws-stale-active-complete")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-new-completed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: completedRunID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed completed run: %v", err)
	}

	if err := orch.CompleteRun(ctx, completedRunID); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateActive || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with no active run", ws)
	}
	_, ok := notifications.waitFor(time.Second, func(item runtimeNotificationRecord) bool {
		return item.Topic == workspaceevents.TopicWorkspaceStateChanged &&
			item.RunID == string(completedRunID) &&
			item.State == string(common.WorkspaceStateActive) &&
			item.ActiveRunID == ""
	})
	if !ok {
		t.Fatalf("notifications = %+v, want completed workspace release for new run", notifications.snapshot())
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
					"nextKind": "runtime/test",
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-multi")
	runID := common.RunID("run-multi")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run state = %s failureReason=%q calls=%#v steps=%+v, want completed", runItem.State, runItem.FailureReason, runtime.calls, steps)
	}

	step1, err := stepRepo.Get(ctx, "run-multi/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateSucceeded {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if step1.Output["nextStep"] != "run-multi/step-2" {
		t.Fatalf("step1 nextStep = %v, want run-multi/step-2", step1.Output["nextStep"])
	}

	step2, err := stepRepo.Get(ctx, "run-multi/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateSucceeded {
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
		{ID: "build", Kind: "graph/mock", DependsOn: []string{"prepare"}, Branch: "ship", Input: map[string]any{stepInputKeySoulStage: "artifact"}},
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
			case "run-graph-branch/graph-001/002-build":
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
		})

	workspaceID := common.WorkspaceID("ws-graph-branch")
	runID := common.RunID("run-graph-branch")
	prepareStepID := common.StepID("run-graph-branch/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateExecuting,
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
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-graph-branch/block/prepare" || runtime.calls[1] != "run-graph-branch/graph-001/002-build" {
		t.Fatalf("runtime calls = %#v, want prepare then build only", runtime.calls)
	}
	if _, err := stepRepo.Get(ctx, "run-graph-branch/graph-001/003-reject"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("reject branch step err = %v, want not found", err)
	}
	buildStep, err := stepRepo.Get(ctx, "run-graph-branch/graph-001/002-build")
	if err != nil {
		t.Fatalf("get build step: %v", err)
	}
	if buildStep.Input["graphBranch"] != "ship" || buildStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("build input = %+v, want graph branch and previous node", buildStep.Input)
	}
	if buildStep.Input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("build soulStage = %v, want execution", buildStep.Input[stepInputKeySoulStage])
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointGraphBranch(checkpoints, "ship") {
		t.Fatalf("checkpoints = %+v, want graph branch evidence", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesIndependentGraphNodesConcurrently(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "plan-a", Kind: "graph/mock"},
		{ID: "plan-b", Kind: "graph/mock"},
		{ID: "merge", Kind: "graph/mock", DependsOn: []string{"plan-a", "plan-b"}},
	}}
	var active atomic.Int32
	var maxActive atomic.Int32
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-graph-concurrent/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case "run-graph-concurrent/graph-001/001-plan-a", "run-graph-concurrent/graph-001/002-plan-b":
				current := active.Add(1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				active.Add(-1)
				return map[string]any{"status": "completed"}, nil
			case "run-graph-concurrent/graph-001/003-merge":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph concurrent step")
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
			MemoryItems:        memory.NewMemoryItemRepository(),
		})

	workspaceID := common.WorkspaceID("ws-graph-concurrent")
	runID := common.RunID("run-graph-concurrent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending, InitialStepKind: "llm/workspace_execution"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}

	if maxActive.Load() < 2 {
		t.Fatalf("maxActive = %d, want at least 2 concurrent ready graph nodes", maxActive.Load())
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	mergeStep, err := stepRepo.Get(ctx, "run-graph-concurrent/graph-001/003-merge")
	if err != nil {
		t.Fatalf("get merge step: %v", err)
	}
	completed, _ := mergeStep.Input["graphCompletedNodes"].([]string)
	if !reflect.DeepEqual(completed, []string{"plan-a", "plan-b"}) {
		t.Fatalf("merge completed nodes = %#v, want both parallel nodes", mergeStep.Input["graphCompletedNodes"])
	}
}

func TestRuntimeOrchestrator_GraphBatchContinuationUsesCurrentBatchRoot(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-batch-current-root")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-batch-current-root"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "patch-a", Kind: "graph/mock"},
		{ID: "patch-b", Kind: "graph/mock"},
		{ID: "verify", Kind: "graph/mock", DependsOn: []string{"patch-a", "patch-b"}},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/provider-retry-001-of-repair-001"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateSucceeded,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	steps := []*runmodel.Step{
		root,
		{
			ID:    graphScopedStepID(runID, 1, 1, "patch-a"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(root.ID),
				stepInputKeyGraphNodeID:     "patch-a",
				stepOutputKeyExecutionGraph: graph,
				"previousStepId":            string(root.ID),
			},
		},
		{
			ID:    graphScopedStepID(runID, 1, 2, "patch-b"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(root.ID),
				stepInputKeyGraphNodeID:     "patch-b",
				stepOutputKeyExecutionGraph: graph,
				"previousStepId":            string(root.ID),
			},
		},
		{
			ID:    common.StepID(string(runID) + "/graph-099/001-unrelated"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(runID) + "/unrelated-root",
				stepInputKeyGraphNodeID:     "unrelated",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{ID: "unrelated", Kind: "graph/mock"}}},
			},
		},
	}
	specs := []stepDispatchSpec{
		{ID: graphScopedStepID(runID, 1, 1, "patch-a"), Kind: "graph/mock", Input: map[string]any{stepInputKeyGraphRootStepID: string(root.ID)}},
		{ID: graphScopedStepID(runID, 1, 2, "patch-b"), Kind: "graph/mock", Input: map[string]any{stepInputKeyGraphRootStepID: string(root.ID)}},
	}

	last := latestCompletedGraphBatchStep(steps, specs)
	if last == nil || graphRootStepID(last) != root.ID {
		t.Fatalf("last = %+v, want completed step from current graph root", last)
	}
	continuedGraph, ok := runtimeGraphSpecForContinuation(steps, last)
	if !ok || len(continuedGraph.Nodes) != 3 {
		t.Fatalf("continued graph = %+v/%v, want root execution graph", continuedGraph, ok)
	}
	nextSpecs := newGraphSubsystem(nil, RuntimeEvidenceRepositories{}).readyGraphStepSpecsFromSteps(ctx, run, last, continuedGraph, steps)
	if len(nextSpecs) != 1 || nextSpecs[0].ID != graphScopedStepID(runID, 1, 3, "verify") {
		t.Fatalf("next specs = %+v, want dependent verify node from current graph", nextSpecs)
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
			case "run-graph-decision/graph-001/002-review":
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
		})

	workspaceID := common.WorkspaceID("ws-graph-decision")
	runID := common.RunID("run-graph-decision")
	prepareStepID := common.StepID("run-graph-decision/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateExecuting,
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
	reviewStep, err := stepRepo.Get(ctx, "run-graph-decision/graph-001/002-review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if reviewStep.State != common.StepStateSucceeded || reviewStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("review step = %+v, want resumed graph node after prepare", reviewStep)
	}
}
