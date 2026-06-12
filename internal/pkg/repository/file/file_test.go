package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceRepositoryGetReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	if _, err := repo.Get(context.Background(), "ws-corrupt"); err == nil {
		t.Fatal("Get err = nil, want json error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workspace file after corrupt load: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("workspace file changed after corrupt load: got %q want %q", string(after), string(corrupt))
	}
}

func TestRunRepositoryCreateReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt run file: %v", err)
	}

	repo := NewRunRepository(path)
	err := repo.Create(context.Background(), &runmodel.Run{
		ID:          common.RunID("run-corrupt"),
		WorkspaceID: common.WorkspaceID("ws-corrupt"),
		State:       common.RunStateCreated,
	})
	if err == nil {
		t.Fatal("Create err = nil, want json error")
	}
	if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Create err = %v, want persisted json error", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read run file after corrupt create: %v", readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("run file changed after corrupt create: got %q want %q", string(after), string(corrupt))
	}
}

func TestWorkspaceRepositoryCreateStillSucceedsAfterEmptyPersistedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	if err := repo.Create(context.Background(), &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-empty"),
		Title: "empty workspace",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("Create with empty persisted file: %v", err)
	}

	item, err := repo.Get(context.Background(), "ws-empty")
	if err != nil {
		t.Fatalf("Get after create with empty persisted file: %v", err)
	}
	if item.ID != "ws-empty" || item.Title != "empty workspace" || item.State != common.WorkspaceStateActive {
		t.Fatalf("workspace = %+v, want ws-empty/empty workspace/active", item)
	}
}

func TestEventRecordRepositoryAppendsJSONLSegmentsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "event_record.json")
	repo := NewEventRecordRepository(root)
	payload := strings.Repeat("x", eventRecordPayloadInlineThreshold+1)

	if err := repo.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("event-jsonl-1"),
		Topic:       "/run/state/changed",
		OccurredAt:  "2026-05-16T00:00:00Z",
		WorkspaceID: common.WorkspaceID("ws-jsonl"),
		RunID:       common.RunID("run-jsonl"),
		Payload:     map[string]any{"large": payload},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("legacy event_record.json stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSuffix(root, ".json"), "events", "events-000001.jsonl")); err != nil {
		t.Fatalf("event JSONL segment stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSuffix(root, ".json"), "payloads", filepath.FromSlash(safeEventRecordFileName("event-jsonl-1"))+".json")); err != nil {
		t.Fatalf("large event payload ref stat: %v", err)
	}
	items, err := repo.List(ctx, repository.EventRecordQuery{RunID: common.RunID("run-jsonl")})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(items) != 1 || len(items[0].Payload) != 0 {
		t.Fatalf("events = %+v, want lightweight event without payload", items)
	}
	if items[0].PayloadRef == "" || len(items[0].PayloadSummary) == 0 {
		t.Fatalf("event payload projection = ref:%q summary:%+v, want ref and summary", items[0].PayloadRef, items[0].PayloadSummary)
	}
	withPayload, err := repo.List(ctx, repository.EventRecordQuery{RunID: common.RunID("run-jsonl"), IncludePayload: true})
	if err != nil {
		t.Fatalf("list events with payload: %v", err)
	}
	if len(withPayload) != 1 || withPayload[0].Payload["large"] != payload {
		t.Fatalf("events = %+v, want rehydrated large payload", withPayload)
	}
}

func TestArtifactContinuationRepositoryStoresContentOutsideMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "artifact-continuations")
	repo := NewArtifactContinuationRepository(root)
	continuationID := common.ArtifactContinuationID("artifact-continuation-path-001")
	chunkContent := strings.Repeat("chunk-body-", 64)
	assembledContent := strings.Repeat("assembled-body-", 64)

	if err := repo.Create(ctx, &orchestratormodel.ArtifactContinuation{
		ID:          continuationID,
		WorkspaceID: common.WorkspaceID("ws-path"),
		RunID:       common.RunID("run-path"),
		RootStepID:  common.StepID("run-path/step-1"),
		ArtifactID:  "artifact-001",
		TargetRef:   "deliverable/path.txt",
		State:       common.ArtifactContinuationStateChunking,
	}); err != nil {
		t.Fatalf("create artifact continuation: %v", err)
	}
	if err := repo.SaveChunk(ctx, &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID: continuationID,
		Sequence:       1,
		ArtifactID:     "artifact-001",
		TargetRef:      "deliverable/path.txt",
		ContentHash:    "hash-001",
		Bytes:          int64(len(chunkContent)),
	}, chunkContent); err != nil {
		t.Fatalf("save chunk: %v", err)
	}
	if err := repo.SaveAssembled(ctx, &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID: continuationID,
		TargetRef:      "deliverable/path.txt",
		Bytes:          int64(len(assembledContent)),
		RollingHash:    "rolling-hash",
	}, assembledContent); err != nil {
		t.Fatalf("save assembled: %v", err)
	}

	dir := filepath.Join(root, string(continuationID))
	chunkMetadata, err := os.ReadFile(filepath.Join(dir, "chunks", "001.json"))
	if err != nil {
		t.Fatalf("read chunk metadata: %v", err)
	}
	if strings.Contains(string(chunkMetadata), chunkContent) {
		t.Fatalf("chunk metadata contains chunk content")
	}
	chunkBody, err := os.ReadFile(filepath.Join(dir, "chunks", "001.content"))
	if err != nil {
		t.Fatalf("read chunk content: %v", err)
	}
	if string(chunkBody) != chunkContent {
		t.Fatalf("chunk content = %q, want original content", string(chunkBody))
	}
	assembledMetadata, err := os.ReadFile(filepath.Join(dir, "assembled", "metadata.json"))
	if err != nil {
		t.Fatalf("read assembled metadata: %v", err)
	}
	if strings.Contains(string(assembledMetadata), assembledContent) {
		t.Fatalf("assembled metadata contains assembled content")
	}
	assembledBody, err := os.ReadFile(filepath.Join(dir, "assembled", "content"))
	if err != nil {
		t.Fatalf("read assembled content: %v", err)
	}
	if string(assembledBody) != assembledContent {
		t.Fatalf("assembled content = %q, want original content", string(assembledBody))
	}
}

func TestStepRepositoryStoresItemsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "step.json")
	repo := NewStepRepository(root)
	stepID := common.StepID("run-item/step-1")

	if err := repo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID("run-item"),
		Kind:   "llm",
		State:  common.StepStateCompleted,
		Input:  map[string]any{"prompt": "hello"},
		Output: map[string]any{"answer": "world"},
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("legacy step.json stat err = %v, want not exist", err)
	}
	itemDir := filepath.Join(filepath.Dir(root), "steps", "run-item", "001-step-1")
	for _, name := range []string{"metadata.json", "input.json", "output.json"} {
		if _, err := os.Stat(filepath.Join(itemDir, name)); err != nil {
			t.Fatalf("step item %s stat: %v", name, err)
		}
	}
	item, err := repo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if item.Input["prompt"] != "hello" || item.Output["answer"] != "world" {
		t.Fatalf("step payload = %+v/%+v, want split input/output", item.Input, item.Output)
	}
}

func TestStepRepositoryListByRunUsesSequenceScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "step.json")
	repo := NewStepRepository(root)
	for _, item := range []*runmodel.Step{
		{
			ID:     "run-a/step-1",
			RunID:  "run-a",
			Kind:   "shell",
			State:  common.StepStateCompleted,
			Output: map[string]any{"value": "a1"},
		},
		{
			ID:     "run-a/step-2",
			RunID:  "run-a",
			Kind:   "shell",
			State:  common.StepStateCompleted,
			Output: map[string]any{"value": "a2"},
		},
		{
			ID:     "run-b/step-1",
			RunID:  "run-b",
			Kind:   "shell",
			State:  common.StepStateCompleted,
			Output: map[string]any{"value": "b1"},
		},
	} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("create step %s: %v", item.ID, err)
		}
	}

	items, err := repo.ListByRun(ctx, "run-a")
	if err != nil {
		t.Fatalf("list run-a steps: %v", err)
	}
	if len(items) != 2 || items[0].ID != "run-a/step-1" || items[1].ID != "run-a/step-2" {
		t.Fatalf("run-a steps = %+v, want only run-a steps in order", items)
	}
	for _, item := range items {
		if item.RunID != "run-a" {
			t.Fatalf("listed step run = %q, want run-a", item.RunID)
		}
	}
}

func TestToolCallRepositoryStoresItemsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool_call.json")
	repo := NewToolCallRepository(root)
	toolCallID := common.ToolCallID("run-item/step-1/tool-1")

	if err := repo.Create(ctx, &runmodel.ToolCall{
		ID:        toolCallID,
		RunID:     common.RunID("run-item"),
		StepID:    common.StepID("run-item/step-1"),
		ToolName:  "shell",
		State:     common.ToolCallStateCompleted,
		Arguments: map[string]any{"command": "true"},
		Output:    map[string]any{"exitCode": float64(0)},
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("legacy tool_call.json stat err = %v, want not exist", err)
	}
	itemDir := filepath.Join(filepath.Dir(root), "tool-calls", "run-item", "001-step-1-tool-1")
	for _, name := range []string{"metadata.json", "arguments.json", "output.json"} {
		if _, err := os.Stat(filepath.Join(itemDir, name)); err != nil {
			t.Fatalf("tool call item %s stat: %v", name, err)
		}
	}
	items, err := repo.ListByStep(ctx, common.StepID("run-item/step-1"))
	if err != nil {
		t.Fatalf("list tool calls by step: %v", err)
	}
	if len(items) != 1 || items[0].Arguments["command"] != "true" || items[0].Output["exitCode"] != float64(0) {
		t.Fatalf("tool calls = %+v, want rehydrated arguments/output", items)
	}
}

func TestRuntimeEvidenceRepositoriesUseRunScopedSequencePathsAndReuseExistingPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := common.RunID("run-evidence")
	stepID := common.StepID("run-evidence/graph-001/001-plan")

	stepRepo := NewStepRepository(filepath.Join(root, "step.json"))
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/planning",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	stepDir := filepath.Join(root, "steps", "run-evidence", "001-graph-001-001-plan")
	if _, err := os.Stat(filepath.Join(stepDir, "metadata.json")); err != nil {
		t.Fatalf("step sequence dir stat: %v", err)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step by logical id: %v", err)
	}
	step.State = common.StepStateCompleted
	step.Output = map[string]any{"status": "done"}
	if err := stepRepo.Update(ctx, step); err != nil {
		t.Fatalf("update step: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stepDir, "output.json")); err != nil {
		t.Fatalf("step update should reuse sequence dir: %v", err)
	}

	validationRepo := NewValidationResultRepository(filepath.Join(root, "validation_result.json"))
	validationID := common.ValidationResultID("run-evidence/graph-001/001-plan/validation/runtime-step")
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:             validationID,
		WorkspaceID:    common.WorkspaceID("ws-evidence"),
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "runtime_step",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation: %v", err)
	}
	validationDir := filepath.Join(root, "validations", "run-evidence", "001-graph-001-001-plan-validation-runtime-step")
	if _, err := os.Stat(filepath.Join(validationDir, "metadata.json")); err != nil {
		t.Fatalf("validation sequence dir stat: %v", err)
	}
	if _, err := validationRepo.Get(ctx, validationID); err != nil {
		t.Fatalf("get validation by logical id: %v", err)
	}
}

func TestRunEvidenceRepositoriesStoreItemsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := common.RunID("run-evidence")
	stepID := common.StepID("run-evidence/step-1")

	patchPath := filepath.Join(root, "patch.json")
	patchRepo := NewPatchRepository(patchPath)
	patchID := common.PatchID("run-evidence/patch-1")
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:      patchID,
		RunID:   runID,
		StepID:  stepID,
		Diff:    "diff --git a/file b/file",
		State:   common.PatchStatePreviewed,
		Summary: "preview",
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	assertLegacyMapAbsent(t, patchPath)
	assertItemFileExists(t, filepath.Join(root, "patches"), string(patchID))
	patches, err := patchRepo.ListByStep(ctx, stepID)
	if err != nil {
		t.Fatalf("list patches by step: %v", err)
	}
	if len(patches) != 1 || patches[0].Diff == "" {
		t.Fatalf("patches = %+v, want rehydrated item", patches)
	}

	validationPath := filepath.Join(root, "validation_result.json")
	validationRepo := NewValidationResultRepository(validationPath)
	validationID := common.ValidationResultID("run-evidence/validation-1")
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:           validationID,
		RunID:        runID,
		StepID:       stepID,
		State:        common.ValidationResultStatePassed,
		Summary:      "passed",
		EvidenceTags: []string{"unit"},
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	assertLegacyMapAbsent(t, validationPath)
	if _, err := os.Stat(filepath.Join(root, "validations", "run-evidence", "001-validation-1", "metadata.json")); err != nil {
		t.Fatalf("expected sequence-scoped validation result: %v", err)
	}

	checkpointPath := filepath.Join(root, "runtime_checkpoint.json")
	checkpointRepo := NewRuntimeCheckpointRepository(checkpointPath)
	checkpointID := common.CheckpointID("run-evidence/checkpoint-1")
	if err := checkpointRepo.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             checkpointID,
		RunID:          runID,
		CheckpointType: common.CheckpointTypeRunState,
		Data:           map[string]any{"state": "running"},
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	assertLegacyMapAbsent(t, checkpointPath)
	assertItemFileExists(t, filepath.Join(root, "checkpoints"), string(checkpointID))
	checkpoint, err := checkpointRepo.Get(ctx, checkpointID)
	if err != nil {
		t.Fatalf("get runtime checkpoint: %v", err)
	}
	if checkpoint.Data["state"] != "running" {
		t.Fatalf("checkpoint data = %+v, want rehydrated data", checkpoint.Data)
	}

	resultPath := filepath.Join(root, "result.json")
	resultRepo := NewResultRepository(resultPath)
	resultID := common.ResultID("run-evidence/result-1")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		RunID:        runID,
		State:        common.ResultStateAccepted,
		Summary:      "accepted",
		EvidenceRefs: []string{"evidence-1"},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	assertLegacyMapAbsent(t, resultPath)
	assertItemFileExists(t, filepath.Join(root, "results"), string(resultID))

	artifactPath := filepath.Join(root, "artifact.json")
	artifactRepo := NewArtifactRepository(artifactPath)
	artifactID := common.ArtifactID("run-evidence/artifact-1")
	if err := artifactRepo.Create(ctx, &resultmodel.Artifact{
		ID:           artifactID,
		RunID:        runID,
		ResultID:     resultID,
		ArtifactType: "text",
		State:        common.ArtifactStateAvailable,
		Metadata:     map[string]any{"bytes": float64(12)},
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	assertLegacyMapAbsent(t, artifactPath)
	assertItemFileExists(t, filepath.Join(root, "artifacts"), string(artifactID))
	artifacts, err := artifactRepo.ListByResult(ctx, resultID)
	if err != nil {
		t.Fatalf("list artifacts by result: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Metadata["bytes"] != float64(12) {
		t.Fatalf("artifacts = %+v, want rehydrated metadata", artifacts)
	}
}

func TestGovernanceRepositoriesStoreItemsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	ticketPath := filepath.Join(root, "ticket.json")
	ticketRepo := NewDecisionTicketRepository(ticketPath)
	ticketID := common.TicketID("run-governance/ticket-1")
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-governance"),
		RunID:       common.RunID("run-governance"),
		Reason:      "need human decision",
		Context:     map[string]any{"field": "value"},
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	assertLegacyMapAbsent(t, ticketPath)
	assertItemFileExists(t, filepath.Join(root, "decisions", "tickets"), string(ticketID))
	tickets, err := ticketRepo.ListByRunID(ctx, common.RunID("run-governance"))
	if err != nil {
		t.Fatalf("list tickets by run: %v", err)
	}
	if len(tickets) != 1 || tickets[0].Context["field"] != "value" {
		t.Fatalf("tickets = %+v, want rehydrated context", tickets)
	}

	resolutionPath := filepath.Join(root, "resolution.json")
	resolutionRepo := NewDecisionResolutionRepository(resolutionPath)
	resolutionID := common.ResolutionID("run-governance/resolution-1")
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "ok"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	assertLegacyMapAbsent(t, resolutionPath)
	assertItemFileExists(t, filepath.Join(root, "decisions", "resolutions"), string(resolutionID))
	resolution, err := resolutionRepo.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get resolution by ticket: %v", err)
	}
	if resolution.Payload["answer"] != "ok" {
		t.Fatalf("resolution payload = %+v, want rehydrated payload", resolution.Payload)
	}

	approvalPath := filepath.Join(root, "approval.json")
	approvalRepo := NewApprovalRepository(approvalPath)
	approvalID := common.ApprovalID("run-governance/approval-1")
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID("ws-governance"),
		RunID:       common.RunID("run-governance"),
		Reason:      "need approval",
		Context:     map[string]any{"risk": "high"},
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	assertLegacyMapAbsent(t, approvalPath)
	assertItemFileExists(t, filepath.Join(root, "approvals"), string(approvalID))
	approvals, err := approvalRepo.ListByWorkspaceID(ctx, common.WorkspaceID("ws-governance"))
	if err != nil {
		t.Fatalf("list approvals by workspace: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Context["risk"] != "high" {
		t.Fatalf("approvals = %+v, want rehydrated context", approvals)
	}
}

func assertLegacyMapAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("legacy map file %s stat err = %v, want not exist", path, err)
	}
}

func assertItemFileExists(t *testing.T, root string, id string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(safeRepositoryItemFileName(id))+".json")); err != nil {
		t.Fatalf("item file for %s under %s stat: %v", id, root, err)
	}
}

func TestDecisionResolutionRepositoryStoresLongReadableIDsWithSafeFileName(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := NewDecisionResolutionRepository(filepath.Join(root, "resolution.json"))
	ticketID := common.TicketID("run-001-" + strings.Repeat("context-gate-retry-", 12) + "context-integrity-decision")
	resolutionID := common.ResolutionID("resolution-ws-real-run-" + string(ticketID) + "-provide_missing_info")

	if err := repo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "continue"},
	}); err != nil {
		t.Fatalf("create long resolution: %v", err)
	}
	resolution, err := repo.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get long resolution by ticket: %v", err)
	}
	if resolution.ID != resolutionID || resolution.Payload["answer"] != "continue" {
		t.Fatalf("resolution = %+v, want stored long ID and payload", resolution)
	}

	paths, err := repositoryItemPaths(filepath.Join(root, "decisions", "resolutions"))
	if err != nil {
		t.Fatalf("list resolution files: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("resolution file count = %d, want 1: %v", len(paths), paths)
	}
	name := filepath.Base(paths[0])
	if len(strings.TrimSuffix(name, filepath.Ext(name))) > 160 {
		t.Fatalf("resolution filename segment too long: len=%d name=%s", len(strings.TrimSuffix(name, filepath.Ext(name))), name)
	}
	if !strings.Contains(name, "-") {
		t.Fatalf("resolution filename = %s, want readable prefix plus hash suffix", name)
	}
}

func TestWorkspaceRepositorySharedInstanceConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-shared-%03d", idx)),
				Title: fmt.Sprintf("shared workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with shared repository instance: %v", err)
		}
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List after concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}

func TestWorkspaceRepositoryIndependentInstancesConcurrentCreateStress(t *testing.T) {
	if os.Getenv("WORKORCH_STRESS_TEST") != "1" {
		t.Skip("set WORKORCH_STRESS_TEST=1 to run file backend cross-instance concurrency stress")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo := NewWorkspaceRepository(path)
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-independent-%03d", idx)),
				Title: fmt.Sprintf("independent workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with independent repository instances: %v", err)
		}
	}

	items, err := NewWorkspaceRepository(path).List(ctx)
	if err != nil {
		t.Fatalf("List after independent concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}
