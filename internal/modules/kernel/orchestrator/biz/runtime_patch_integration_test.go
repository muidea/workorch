package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestApplyWorkspaceFileWriteReportsPathConflict(t *testing.T) {
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "llm-proxy"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("seed executable file: %v", err)
	}

	err := applyWorkspaceFileWrite(deliverableDirectory, "llm-proxy/config.yaml", "config", "")
	if err == nil {
		t.Fatalf("applyWorkspaceFileWrite should fail for file-as-directory path conflict")
	}
	var conflict *workspacePathConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %T %[1]v, want workspacePathConflictError", err)
	}
	if conflict.TargetRef != "llm-proxy/config.yaml" || conflict.ConflictRef != "llm-proxy" || conflict.ConflictKind != "file" {
		t.Fatalf("conflict = %+v, want target and conflict refs", conflict)
	}
	diagnostics := conflict.RuntimeFailureDiagnostics()
	if diagnostics["kind"] != "workspace_path_conflict" || diagnostics["conflictRef"] != "llm-proxy" {
		t.Fatalf("diagnostics = %+v, want structured path conflict diagnostics", diagnostics)
	}
}

func TestApplyWorkspacePatchRejectsBaseHashMismatch(t *testing.T) {
	deliverableDirectory := t.TempDir()
	target := filepath.Join(deliverableDirectory, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(deliverableDirectory, runtimePatchRequest{
		TargetRef: "main.go",
		Content:   "package changed\n",
		PatchType: "file_write",
		BaseHash:  "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil || !strings.Contains(err.Error(), "baseHash mismatch") {
		t.Fatalf("applyWorkspacePatch err = %v, want baseHash mismatch", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if string(content) != "package main\n" {
		t.Fatalf("content = %q, want unchanged", content)
	}
}

func TestApplyWorkspacePatchAppliesTextReplaceWithBaseHash(t *testing.T) {
	deliverableDirectory := t.TempDir()
	target := filepath.Join(deliverableDirectory, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(deliverableDirectory, runtimePatchRequest{
		TargetRef: "main.go",
		PatchType: "text_replace",
		BaseHash:  sha256HexBytes([]byte(original)),
		OldText:   "\tprintln(\"old\")",
		NewText:   "\tprintln(\"new\")",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch text_replace err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if got, want := string(content), "package main\n\nfunc main() {\n\tprintln(\"new\")\n}\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestApplyWorkspacePatchRejectsRangeLessStandardUnifiedDiff(t *testing.T) {
	deliverableDirectory := t.TempDir()
	target := filepath.Join(deliverableDirectory, "docs", "sample.txt")
	original := "alpha\n\nsection one\nold value\nend section\n\nomega\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(deliverableDirectory, runtimePatchRequest{
		TargetRef: "docs/sample.txt",
		PatchType: "unified_diff",
		Diff:      "--- a/docs/sample.txt\n+++ b/docs/sample.txt\n@@\n section one\n-old value\n+new value\n end section\n",
	})
	if err == nil || !strings.Contains(err.Error(), "explicit line ranges") {
		t.Fatalf("applyWorkspacePatch range-less standard diff err = %v, want explicit line range rejection", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("content = %q, want unchanged", string(content))
	}
}

func TestApplyWorkspacePatchAcceptsUnifiedDiffWithDriftedLineNumberWhenContextIsUnique(t *testing.T) {
	deliverableDirectory := t.TempDir()
	target := filepath.Join(deliverableDirectory, "handler", "handler_test.go")
	original := "package handler\n\nimport (\n\t\"encoding/json\"\n\t\"net/http\"\n\t\"net/http/httptest\"\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n\t\"time\"\n\n\t\"ai-gateway/config\"\n)\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(deliverableDirectory, runtimePatchRequest{
		TargetRef: "handler/handler_test.go",
		PatchType: "unified_diff",
		Diff: "--- handler/handler_test.go\n+++ handler/handler_test.go\n@@ -8,7 +8,7 @@\n" +
			" \t\"time\"\n" +
			" \n" +
			"-\t\"ai-gateway/config\"\n" +
			"+\t\"openai-gateway/config\"\n" +
			" )\n",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch drifted line diff err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "\"openai-gateway/config\"") || strings.Contains(string(content), "\"ai-gateway/config\"") {
		t.Fatalf("content = %q, want drifted line diff applied", string(content))
	}
}

func TestApplyWorkspacePatchAcceptsSingleFileApplyPatchStyleDiff(t *testing.T) {
	deliverableDirectory := t.TempDir()
	target := filepath.Join(deliverableDirectory, "docs", "sample.txt")
	original := "alpha\n\nsection one\nold value\nend section\n\nomega\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(deliverableDirectory, runtimePatchRequest{
		TargetRef: "docs/sample.txt",
		PatchType: "unified_diff",
		Diff:      "*** Begin Patch\n*** Update File: docs/sample.txt\n@@\n section one\n-old value\n+new value\n end section\n*** End Patch",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch apply-patch style diff err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "new value") || strings.Contains(string(content), "old value") {
		t.Fatalf("content = %q, want apply-patch style diff applied", string(content))
	}
}

func TestPatchPreviewDoesNotWriteFile(t *testing.T) {
	deliverableDirectory := t.TempDir()
	result, err := NewPatchRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          llmcommon.RuntimeBlockKindPatchPreview,
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "preview.txt",
			"content":   "preview only\n",
		},
	})
	if err != nil {
		t.Fatalf("patch preview: %v", err)
	}
	if result.Output["stage"] != "patch_preview" || result.Output["patches"] != nil {
		t.Fatalf("patch preview output = %+v, want preview without patches side effect", result.Output)
	}
	if _, err := os.Stat(filepath.Join(deliverableDirectory, "preview.txt")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote target file, stat err = %v", err)
	}
}

func TestRuntimeOrchestratorPublishesPatchPreviewEvent(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: memory.NewPatchRepository()},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-preview-event", WorkspaceID: "ws-patch-preview-event"}
	step := &runmodel.Step{ID: "run-patch-preview-event/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-preview-event/step-1/tool-call"}

	err := orch.patchManager().publishPreviewFromOutput(ctx, run, step, toolCall, map[string]any{
		"patchPreview": map[string]any{
			"patchType":   "file_write",
			"targetRef":   "preview.txt",
			"summary":     "preview write",
			"addedLines":  1,
			"contentHash": "content-sha",
		},
	})
	if err != nil {
		t.Fatalf("publish patch preview: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != orchestratorevents.TopicPatchPreviewed || published[0].Payload[events.PayloadKeyTargetRef] != "preview.txt" {
		t.Fatalf("published = %+v, want patch preview event", published)
	}
}

func TestRuntimeOrchestratorApplyRuntimePatchesPublishesAppliedAndFailedEvents(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	hub := events.NewMemoryHub()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-event", WorkspaceID: "ws-patch-event"}
	step := &runmodel.Step{ID: "run-patch-event/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-event/step-1/tool-call"}

	if err := orch.patchManager().apply(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "applied.txt",
			"content":   "applied\n",
			"summary":   "write applied file",
		}},
	}, deliverableDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if !publishedTopicForTarget(hub.Published(), orchestratorevents.TopicPatchApplied, "applied.txt") {
		t.Fatalf("published = %+v, want patch applied event", hub.Published())
	}

	failedStep := &runmodel.Step{ID: "run-patch-event/step-2", RunID: run.ID}
	err := orch.patchManager().apply(ctx, run, failedStep, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "unified_diff",
			"targetRef": "main.go",
			"diff":      "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-wrong\n+after\n",
			"summary":   "fail context",
		}},
	}, deliverableDirectory)
	if err == nil {
		t.Fatalf("apply invalid diff should fail")
	}
	if !publishedTopicForTarget(hub.Published(), orchestratorevents.TopicPatchFailed, "main.go") {
		t.Fatalf("published = %+v, want patch failed event", hub.Published())
	}
}

func TestRuntimeOrchestratorPublishesArtifactContinuationMetadataEvent(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	orch := &RuntimeOrchestrator{eventHub: hub}
	run := &runmodel.Run{ID: "run-artifact-continuation-event", WorkspaceID: "ws-artifact-continuation-event"}
	step := &runmodel.Step{ID: "run-artifact-continuation-event/artifact-assemble-001", RunID: run.ID}
	item := &orchestratormodel.ArtifactContinuation{
		ID:                "artifact-continuation-event-001",
		WorkspaceID:       run.WorkspaceID,
		RunID:             run.ID,
		ArtifactID:        "artifact-001",
		TargetRef:         "deliverable/target.txt",
		State:             common.ArtifactContinuationStateApplying,
		ExpectedSequence:  3,
		ReceivedSequences: []int{1, 2},
		RollingHash:       "hash-001",
		Metadata:          map[string]any{"content": "must not be published"},
	}

	if err := orch.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationAssembled, nil); err != nil {
		t.Fatalf("publish artifact continuation event: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != orchestratorevents.TopicArtifactContinuationAssembled {
		t.Fatalf("published = %+v, want artifact continuation assembled event", published)
	}
	payload := published[0].Payload
	if payload["targetRef"] != item.TargetRef || payload["rollingHash"] != item.RollingHash {
		t.Fatalf("payload = %+v, want metadata", payload)
	}
	if payload["content"] != nil || payload["body"] != nil || payload["diff"] != nil || payload["metadata"] != nil {
		t.Fatalf("payload = %+v, want no body content or raw metadata", payload)
	}
}

func TestRuntimeOrchestratorRevertsAppliedPatch(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	hub := events.NewMemoryHub()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-revert", WorkspaceID: "ws-patch-revert"}
	step := &runmodel.Step{ID: "run-patch-revert/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-revert/step-1/tool-call"}
	if err := orch.patchManager().apply(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "main.go",
			"content":   "after\n",
			"summary":   "write after",
		}},
	}, deliverableDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	patchID := common.PatchID("run-patch-revert/step-1/patch/1")
	patch, err := patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch.BeforeHash == "" || patch.AfterHash == "" || !patch.OriginalExists || patch.OriginalContent != "before\n" {
		t.Fatalf("patch metadata = %+v, want reversible metadata", patch)
	}
	revertStep := &runmodel.Step{ID: "run-patch-revert/step-2", RunID: run.ID}
	output := map[string]any{"patchRevert": map[string]any{"patchId": string(patchID)}}
	if err := orch.patchManager().revert(ctx, run, revertStep, toolCall, output, deliverableDirectory); err != nil {
		t.Fatalf("revert patch: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(deliverableDirectory, "main.go"))
	if err != nil {
		t.Fatalf("read reverted file: %v", err)
	}
	if string(content) != "before\n" {
		t.Fatalf("content = %q, want reverted content", string(content))
	}
	patch, err = patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get reverted patch: %v", err)
	}
	if patch.State != common.PatchStateReverted || patch.RevertedAt == "" {
		t.Fatalf("patch = %+v, want reverted state", patch)
	}
	if !publishedTopicForTarget(hub.Published(), orchestratorevents.TopicPatchReverted, "main.go") {
		t.Fatalf("published = %+v, want patch reverted event", hub.Published())
	}
}

func TestRuntimeOrchestratorRevertRejectsHashDrift(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		events.NewMemoryHub(),
	)
	run := &runmodel.Run{ID: "run-patch-revert-conflict", WorkspaceID: "ws-patch-revert-conflict"}
	step := &runmodel.Step{ID: "run-patch-revert-conflict/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-revert-conflict/step-1/tool-call"}
	if err := orch.patchManager().apply(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "main.go",
			"content":   "after\n",
			"summary":   "write after",
		}},
	}, deliverableDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("user changed\n"), 0o644); err != nil {
		t.Fatalf("drift file: %v", err)
	}
	patchID := common.PatchID("run-patch-revert-conflict/step-1/patch/1")
	err := orch.patchManager().revert(ctx, run, &runmodel.Step{ID: "run-patch-revert-conflict/step-2", RunID: run.ID}, toolCall, map[string]any{
		"patchRevert": map[string]any{"patchId": string(patchID)},
	}, deliverableDirectory)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("revert error = %v, want conflict", err)
	}
	content, err := os.ReadFile(filepath.Join(deliverableDirectory, "main.go"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "user changed\n" {
		t.Fatalf("content = %q, want drift preserved", string(content))
	}
	patch, err := patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch.State != common.PatchStateApplied {
		t.Fatalf("patch state = %s, want applied after failed revert", patch.State)
	}
}

func TestRuntimeOrchestratorBlocksSensitivePatchUntilApproval(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	workspaceID := common.WorkspaceID("ws-sensitive-patch-approval")
	runID := common.RunID("run-sensitive-patch-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "sensitive patch", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": ".env",
			"content":   "TOKEN=secret\n",
			"summary":   "write env",
		}},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{Patches: patchRepo},
		events.NewMemoryHub(),
	)

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID == "" {
		t.Fatalf("run = %+v, want blocked on approval", runItem)
	}
	if _, err := os.Stat(filepath.Join(deliverableDirectory, ".env")); !os.IsNotExist(err) {
		t.Fatalf("sensitive patch wrote .env before approval, stat err = %v", err)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 0 {
		t.Fatalf("patches = %+v, want no patch record before approval", patches)
	}
	approval, err := approvalRepo.Get(ctx, runItem.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Context["kind"] != "patch_approval_required" {
		t.Fatalf("approval context = %+v, want patch approval context", approval.Context)
	}
}

func TestRuntimeOrchestratorAllowsSensitivePatchWithApprovedApprovalID(t *testing.T) {
	ctx := context.Background()
	approvalRepo := memory.NewApprovalRepository()
	runID := common.RunID("run-sensitive-patch-approved")
	approvalID := common.ApprovalID("approval-sensitive-patch-approved")
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, RunID: runID, State: common.ApprovalStateApproved}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{Patches: memory.NewPatchRepository()},
		events.NewMemoryHub(),
	)

	spec, err := orch.patchManager().approvalBlockSpec(ctx, runID, "", map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType":  "file_write",
			"targetRef":  ".env",
			"content":    "TOKEN=secret\n",
			"approvalId": string(approvalID),
		}},
	})
	if err != nil {
		t.Fatalf("patch approval gate: %v", err)
	}
	if spec.Reason != "" {
		t.Fatalf("approval spec = %+v, want no block with approved approval id", spec)
	}
}

func publishedTopicForTarget(items []events.Envelope, topic string, targetRef string) bool {
	for _, item := range items {
		if item.Topic == topic && item.Payload[events.PayloadKeyTargetRef] == targetRef {
			return true
		}
	}
	return false
}

func TestWorkspaceTargetPathRejectsSymlinkEscape(t *testing.T) {
	deliverableDirectory := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(deliverableDirectory, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := workspaceTargetPath(deliverableDirectory, "outside/owned.txt"); err == nil {
		t.Fatalf("workspaceTargetPath should reject symlink escape")
	}
}

func TestRuntimePatchRequestRejectsReservedDeliverableTarget(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "delivery/weather.md",
		"content":   "weather",
	})
	if err == nil || !strings.Contains(err.Error(), "reserved for WorkOrch-managed artifacts") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want reserved target error", err)
	}
}

func TestRuntimePatchRequestRejectsReservedGovernanceReportTarget(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "decision_record.md",
		"content":   "decision",
	})
	if err == nil || !strings.Contains(err.Error(), "reserved for WorkOrch-managed artifacts") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want reserved governance target error", err)
	}
}

func TestRuntimePatchRequestRejectsUnresolvedTemplateContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "docs/weather.md",
		"content":   "{{ get_weather.stdout }}",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved template error", err)
	}
}

func TestRuntimePatchRequestAllowsSourceCompositeLiteralBraces(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "main_test.go",
		"content": `package main

var response = map[string]interface{}{
	"choices": []map[string]interface{}{{
		"message": map[string]string{"content": "ok"},
	}},
}
`,
	})
	if err != nil {
		t.Fatalf("runtimePatchRequestFromMap err = %v, want Go composite literal braces allowed", err)
	}
}

func TestRuntimePatchRequestAcceptsTextReplace(t *testing.T) {
	request, err := runtimePatchRequestFromMap(map[string]any{
		"patchType":  "text_replace",
		"targetRef":  "main.go",
		"oldText":    "old",
		"newText":    "new",
		"baseHash":   "hash-main",
		"occurrence": float64(2),
	})
	if err != nil {
		t.Fatalf("runtimePatchRequestFromMap text_replace err = %v", err)
	}
	if request.PatchType != "text_replace" || request.OldText != "old" || request.NewText != "new" || request.Occurrence != 2 {
		t.Fatalf("request = %+v, want text_replace fields", request)
	}
}

func TestRuntimePatchRequestRejectsNoOpTextReplace(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "text_replace",
		"targetRef": "main.go",
		"oldText":   "same",
		"newText":   "same",
	})
	if err == nil || !strings.Contains(err.Error(), "substantive change") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want no-op text_replace rejection", err)
	}
}

func TestRuntimePatchRequestRejectsUnresolvedSymbolicPlaceholderContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "config.go",
		"content":   "__FROM_READ_REWRITE_CONFIG_GO_WITH_CONFIG_YAML_AND_ENV_OVERRIDE__",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved symbolic placeholder error", err)
	}
}

func TestRuntimePatchRequestRejectsRedactedMarkerContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "main_test.go",
		"content":   `if auth != "Bearer [REDACTED]" { panic("bad auth") }`,
	})
	if err == nil || !strings.Contains(err.Error(), "redacted runtime markers") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want redacted marker error", err)
	}
}

func TestRuntimePatchRequestRejectsAnglePlaceholderDiff(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main.go",
		"diff":      "<<FROM_IMPLEMENTATION_STEP_MAIN_DIFF>>",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved angle placeholder error", err)
	}
}

func TestRuntimePatchRequestRejectsEmptyFileWriteContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "config.json",
		"content":   "   ",
	})
	if err == nil || !strings.Contains(err.Error(), "patch content is required") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want empty content error", err)
	}
}

func TestRuntimePatchRequestRejectsNonSubstantivePatchPlaceholder(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "logger.go",
		"summary":   "添加注释以确保补丁节点存在",
		"content": `package main

import "fmt"

// LogMessage prints a message to stdout
func LogMessage(msg string) {
	fmt.Println(msg)
}
`,
	})
	if err == nil || !strings.Contains(err.Error(), "non-substantive state-change placeholder") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want non-substantive patch error", err)
	}
}

func TestRuntimePatchRequestRejectsContractConvergenceMarkerPatch(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "unified_diff",
		"targetRef": "config.yaml",
		"summary":   "为满足修复收敛契约执行最小状态变更，新增注释且不改变运行语义",
		"diff":      "--- a/config.yaml\n+++ b/config.yaml\n@@ -1,2 +1,3 @@\n+# runtime-repair: bounded state change for strategy reassessment convergence\n listen_addr: \":8080\"\n",
	})
	if err == nil || !strings.Contains(err.Error(), "non-substantive state-change placeholder") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want contract marker patch rejection", err)
	}
}

func TestRuntimePatchRequestRejectsVerificationAnchorPatch(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main.go",
		"summary":   "在失败目标 main.go 上做最小状态变更，并为严格收敛修复提供有界验证锚点",
		"diff":      "--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n package main\n+\n+// state-change anchor for verification\n",
	})
	if err == nil || !strings.Contains(err.Error(), "non-substantive state-change placeholder") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want verification anchor patch rejection", err)
	}
}
