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

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff")
	runID := common.RunID("run-unified-diff")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
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
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff-file")
	runID := common.RunID("run-unified-diff-file")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
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
		},
		events.NewMemoryHub(),
	)

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
	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || stringFromMap(step.Output, "stage") != "patch_apply_failed" {
		t.Fatalf("step = %+v, want failed patch_apply_failed step", step)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateFailed {
		t.Fatalf("patches = %+v, want failed unified diff patch", patches)
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
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-invalid")
	runID := common.RunID("run-patch-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
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
	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || stringFromMap(step.Output, "stage") != "patch_apply_failed" {
		t.Fatalf("step = %+v, want failed patch_apply_failed step", step)
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
	if len(published) < 7 {
		t.Fatalf("published events = %d, want at least 7", len(published))
	}
	if published[0].Version != events.WorkorchEventVersion {
		t.Fatalf("first event version = %s, want %s", published[0].Version, events.WorkorchEventVersion)
	}
	if published[0].Topic != runevents.TopicRunStateChanged || published[0].Payload[events.PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("first event = %#v, want run running", published[0])
	}
	if published[1].Topic != workspaceevents.TopicWorkspaceStateChanged || published[1].Payload[events.PayloadKeyActiveRunID] != string(runID) {
		t.Fatalf("second event = %#v, want workspace active run bound", published[1])
	}
	if published[1].Payload[events.PayloadKeyState] != string(common.WorkspaceStateRunning) {
		t.Fatalf("second event state = %v, want workspace running", published[1].Payload[events.PayloadKeyState])
	}
	if published[2].Topic != runevents.TopicStepDispatched || published[2].Payload[events.PayloadKeyState] != string(common.StepStateDispatched) {
		t.Fatalf("third event = %#v, want step dispatched", published[2])
	}
	if published[3].Topic != runevents.TopicStepStateChanged || published[3].Payload[events.PayloadKeyState] != string(common.StepStateCompleted) {
		t.Fatalf("fourth event = %#v, want step completed", published[3])
	}
	if published[4].Topic != runevents.TopicRunStateChanged || published[4].Payload[events.PayloadKeyState] != string(common.RunStateValidating) {
		t.Fatalf("fifth event = %#v, want run validating", published[4])
	}
	if published[5].Topic != workspaceevents.TopicWorkspaceStateChanged || published[5].Payload[events.PayloadKeyState] != string(common.WorkspaceStateValidating) {
		t.Fatalf("sixth event = %#v, want workspace validating", published[5])
	}
	if published[6].Topic != runevents.TopicRunStateChanged || published[6].Payload[events.PayloadKeyState] != string(common.RunStateCompleted) {
		t.Fatalf("seventh event = %#v, want run completed", published[6])
	}
	last := published[len(published)-1]
	if last.Topic != workspaceevents.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace active run cleared", last)
	}
	if last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateCompleted) {
		t.Fatalf("last event state = %v, want workspace completed", last.Payload[events.PayloadKeyState])
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
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-evt-stream")
	runID := common.RunID("run-evt-stream")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	var streamProgress events.Envelope
	found := false
	for _, item := range hub.Published() {
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
	if streamProgress.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) ||
		streamProgress.Payload[events.PayloadKeyRunID] != string(runID) {
		t.Fatalf("stream payload workspace/run = %+v, want %s/%s", streamProgress.Payload, workspaceID, runID)
	}
	if streamProgress.Payload[events.PayloadKeyChunkCount] != 3 ||
		streamProgress.Payload[events.PayloadKeyReceivedBytes] != 6 {
		t.Fatalf("stream payload chunk/bytes = %+v, want 3/6", streamProgress.Payload)
	}
	if streamProgress.Payload[events.PayloadKeyProviderID] != "provider-a" ||
		streamProgress.Payload[events.PayloadKeyModel] != "model-a" ||
		streamProgress.Payload[events.PayloadKeyStatus] != string(common.StepStateCompleted) {
		t.Fatalf("stream payload provider/model/status = %+v", streamProgress.Payload)
	}

	var usageRecorded events.Envelope
	usageFound := false
	for _, item := range hub.Published() {
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
	if usageRecorded.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) ||
		usageRecorded.Payload[events.PayloadKeyRunID] != string(runID) ||
		usageRecorded.Payload[events.PayloadKeyStepID] == "" {
		t.Fatalf("usage payload workspace/run/step = %+v", usageRecorded.Payload)
	}
	if usageRecorded.Payload[events.PayloadKeyProviderID] != "provider-a" ||
		usageRecorded.Payload[events.PayloadKeyModel] != "model-a" ||
		usageRecorded.Payload[events.PayloadKeyStreaming] != true {
		t.Fatalf("usage payload provider/model/streaming = %+v", usageRecorded.Payload)
	}
	if usageRecorded.Payload[events.PayloadKeyStatus] != "completed" || usageRecorded.Payload[events.PayloadKeyTotalTokens] == nil {
		t.Fatalf("usage payload status/tokens = %+v", usageRecorded.Payload)
	}
}

func TestRuntimeOrchestrator_CompleteRunClearsStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-stale-active-complete")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-new-completed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateFailed,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: completedRunID, WorkspaceID: workspaceID, State: common.RunStateValidating}); err != nil {
		t.Fatalf("seed completed run: %v", err)
	}

	if err := orch.CompleteRun(ctx, completedRunID); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateCompleted || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with no active run", ws)
	}
	last := hub.Published()[len(hub.Published())-1]
	if last.Topic != workspaceevents.TopicWorkspaceStateChanged ||
		last.RunID != string(completedRunID) ||
		last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateCompleted) ||
		last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want completed workspace release for new run", last)
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
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run state = %s failureReason=%q calls=%#v steps=%+v, want completed", runItem.State, runItem.FailureReason, runtime.calls, steps)
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
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-concurrent")
	runID := common.RunID("run-graph-concurrent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated, InitialStepKind: "llm/workspace_execution"}); err != nil {
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
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	steps := []*runmodel.Step{
		root,
		{
			ID:    graphScopedStepID(runID, 1, 1, "patch-a"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateCompleted,
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
			State: common.StepStateCompleted,
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
			State: common.StepStateCompleted,
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
	reviewStep, err := stepRepo.Get(ctx, "run-graph-decision/graph-001/002-review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if reviewStep.State != common.StepStateCompleted || reviewStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("review step = %+v, want resumed graph node after prepare", reviewStep)
	}
}
