package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestAssembleArtifactContinuationStoresContentInSequenceOrder(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	content1 := "hello "
	content2 := "world\n"
	item.FinalReceived = true
	item.ReceivedSequences = []int{2, 1}
	item.BytesWritten = int64(len([]byte(content1 + content2)))
	item.RollingHash = "sha256:" + sha256HexBytes([]byte(content1+content2))
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	saveArtifactChunkForAssembleTest(t, ctx, repo, item, 1, content1, item.TargetRef)
	saveArtifactChunkForAssembleTest(t, ctx, repo, item, 2, content2, item.TargetRef)

	metadata, content, err := assembleArtifactContinuation(ctx, repo, item)
	if err != nil {
		t.Fatalf("assembleArtifactContinuation() error = %v", err)
	}
	if content != content1+content2 {
		t.Fatalf("assembled content = %q, want %q", content, content1+content2)
	}
	if metadata.Bytes != item.BytesWritten || !artifactChunkHashesEqual(metadata.RollingHash, item.RollingHash) {
		t.Fatalf("assembled metadata = %+v, want bytes/hash from content", metadata)
	}
	if got := strings.Join(metadata.SourceChunkRefs, ","); got != "001,002" {
		t.Fatalf("SourceChunkRefs = %v, want 001,002", metadata.SourceChunkRefs)
	}
	stored, storedContent, err := repo.GetAssembled(ctx, item.ID)
	if err != nil {
		t.Fatalf("get assembled: %v", err)
	}
	if storedContent != content || stored.RollingHash != metadata.RollingHash {
		t.Fatalf("stored assembled = %+v/%q, want saved metadata/content", stored, storedContent)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated continuation: %v", err)
	}
	if updated.State != common.ArtifactContinuationStateApplying || updated.RollingHash != metadata.RollingHash || updated.BytesWritten != metadata.Bytes {
		t.Fatalf("updated continuation = %+v, want applying state with assembled bytes/hash", updated)
	}
}

func TestAssembleArtifactContinuationRejectsMissingFinalChunk(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.ReceivedSequences = []int{1}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	saveArtifactChunkForAssembleTest(t, ctx, repo, item, 1, "content", item.TargetRef)

	_, _, err := assembleArtifactContinuation(ctx, repo, item)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "final chunk") {
		t.Fatalf("assembleArtifactContinuation() error = %v, want final chunk rejection", err)
	}
}

func TestAssembleArtifactContinuationRejectsSequenceGap(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.FinalReceived = true
	item.ReceivedSequences = []int{1, 3}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}

	_, _, err := assembleArtifactContinuation(ctx, repo, item)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "contiguous") {
		t.Fatalf("assembleArtifactContinuation() error = %v, want sequence gap rejection", err)
	}
}

func TestAssembleArtifactContinuationRejectsTargetRefMismatch(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.FinalReceived = true
	item.ReceivedSequences = []int{1}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	saveArtifactChunkForAssembleTest(t, ctx, repo, item, 1, "content", "deliverable/other.txt")

	_, _, err := assembleArtifactContinuation(ctx, repo, item)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "targetRef") {
		t.Fatalf("assembleArtifactContinuation() error = %v, want targetRef rejection", err)
	}
}

func TestAssembleArtifactContinuationRejectsChunkHashMismatch(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.FinalReceived = true
	item.ReceivedSequences = []int{1}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	if err := repo.SaveChunk(ctx, &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID: item.ID,
		Sequence:       1,
		ArtifactID:     item.ArtifactID,
		TargetRef:      item.TargetRef,
		ContentHash:    sha256HexBytes([]byte("different")),
		Bytes:          int64(len([]byte("content"))),
	}, "content"); err != nil {
		t.Fatalf("save chunk: %v", err)
	}

	_, _, err := assembleArtifactContinuation(ctx, repo, item)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "contentHash") {
		t.Fatalf("assembleArtifactContinuation() error = %v, want hash rejection", err)
	}
}

func TestAssembleArtifactContinuationRejectsRollingHashMismatch(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.FinalReceived = true
	item.ReceivedSequences = []int{1}
	item.RollingHash = sha256HexBytes([]byte("different"))
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	saveArtifactChunkForAssembleTest(t, ctx, repo, item, 1, "content", item.TargetRef)

	_, _, err := assembleArtifactContinuation(ctx, repo, item)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "rollingHash") {
		t.Fatalf("assembleArtifactContinuation() error = %v, want rollingHash rejection", err)
	}
}

func TestApplyAssembledArtifactContinuationWritesDeliverableAndEvidence(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	patches := memory.NewPatchRepository()
	item := newArtifactContinuationForAssembleTest()
	item.State = common.ArtifactContinuationStateApplying
	content := "assembled content\n"
	metadata := &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID:  item.ID,
		TargetRef:       item.TargetRef,
		Bytes:           int64(len([]byte(content))),
		RollingHash:     sha256HexBytes([]byte(content)),
		SourceChunkRefs: []string{"001"},
	}
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	if err := continuations.SaveAssembled(ctx, metadata, content); err != nil {
		t.Fatalf("save assembled: %v", err)
	}
	run := &runmodel.Run{
		ID:          item.RunID,
		WorkspaceID: item.WorkspaceID,
	}
	step := &runmodel.Step{
		ID:     common.StepID("run-assemble/002-artifact-apply"),
		RunID:  item.RunID,
		Kind:   "artifact/apply",
		Output: map[string]any{},
	}
	toolCall := &runmodel.ToolCall{
		ID:          common.ToolCallID("run-assemble/002-artifact-apply/tool-call"),
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      step.ID,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ArtifactContinuations: continuations,
			Patches:               patches,
		},
	}

	if err := orch.applyAssembledArtifactContinuation(ctx, run, step, toolCall, item, t.TempDir()); err != nil {
		t.Fatalf("applyAssembledArtifactContinuation() error = %v", err)
	}
	updated, err := continuations.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get continuation: %v", err)
	}
	if updated.State != common.ArtifactContinuationStateApplied {
		t.Fatalf("continuation state = %s, want applied", updated.State)
	}
	if len(updated.EvidenceRefs) != 1 || updated.EvidenceRefs[0] != string(step.ID)+"/patch/1" {
		t.Fatalf("evidenceRefs = %+v, want patch evidence ref", updated.EvidenceRefs)
	}
	storedMeta, storedContent, err := continuations.GetAssembled(ctx, item.ID)
	if err != nil {
		t.Fatalf("get assembled: %v", err)
	}
	if storedContent != content || len(storedMeta.ApplyEvidenceRefs) != 1 || storedMeta.ApplyEvidenceRefs[0] != updated.EvidenceRefs[0] {
		t.Fatalf("assembled apply metadata/content = %+v/%q, want apply evidence ref", storedMeta, storedContent)
	}
	appliedPatches, err := patches.ListByStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(appliedPatches) != 1 || appliedPatches[0].State != common.PatchStateApplied || appliedPatches[0].TargetRef != item.TargetRef {
		t.Fatalf("applied patches = %+v, want one applied target patch", appliedPatches)
	}
}

func TestApplyAssembledArtifactContinuationRejectsHashMismatch(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.State = common.ArtifactContinuationStateApplying
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	if err := continuations.SaveAssembled(ctx, &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID: item.ID,
		TargetRef:      item.TargetRef,
		Bytes:          int64(len([]byte("content"))),
		RollingHash:    sha256HexBytes([]byte("different")),
	}, "content"); err != nil {
		t.Fatalf("save assembled: %v", err)
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ArtifactContinuations: continuations,
			Patches:               memory.NewPatchRepository(),
		},
	}

	err := orch.applyAssembledArtifactContinuation(ctx, &runmodel.Run{ID: item.RunID, WorkspaceID: item.WorkspaceID}, &runmodel.Step{ID: "run-assemble/002-apply"}, &runmodel.ToolCall{ID: "run-assemble/002-apply/tool-call"}, item, t.TempDir())
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "rollingHash") {
		t.Fatalf("applyAssembledArtifactContinuation() error = %v, want rollingHash rejection", err)
	}
}

func TestApplyAssembledArtifactContinuationRecordsApplyFailureDiagnostics(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.State = common.ArtifactContinuationStateApplying
	content := "assembled content\n"
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	if err := continuations.SaveAssembled(ctx, &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID: item.ID,
		TargetRef:      item.TargetRef,
		Bytes:          int64(len([]byte(content))),
		RollingHash:    sha256HexBytes([]byte(content)),
	}, content); err != nil {
		t.Fatalf("save assembled: %v", err)
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ArtifactContinuations: continuations,
			Patches:               memory.NewPatchRepository(),
		},
	}
	step := &runmodel.Step{ID: "run-assemble/002-apply", RunID: item.RunID}
	toolCall := &runmodel.ToolCall{
		ID:          common.ToolCallID("run-assemble/002-apply/tool-call"),
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      step.ID,
	}

	err := orch.applyAssembledArtifactContinuation(ctx, &runmodel.Run{ID: item.RunID, WorkspaceID: item.WorkspaceID}, step, toolCall, item, "")
	if err == nil {
		t.Fatal("applyAssembledArtifactContinuation() error = nil, want apply failure")
	}
	updated, getErr := continuations.Get(ctx, item.ID)
	if getErr != nil {
		t.Fatalf("get continuation: %v", getErr)
	}
	if updated.State != common.ArtifactContinuationStateFailed {
		t.Fatalf("continuation state = %s, want failed", updated.State)
	}
	diag := mapFromValue(updated.Metadata["applyFailure"])
	if diag["targetRef"] != item.TargetRef || diag["failedStepId"] != string(step.ID) || strings.TrimSpace(stringFromMap(diag, "failureError")) == "" {
		t.Fatalf("applyFailure metadata = %+v, want target and failure diagnostics", diag)
	}
}

func TestArtifactContinuationRecoverySpecsDispatchByState(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		state    common.ArtifactContinuationState
		wantKind string
	}{
		{name: "planning", state: common.ArtifactContinuationStatePlanning, wantKind: llmWorkspaceExecutionStepKind},
		{name: "chunking", state: common.ArtifactContinuationStateChunking, wantKind: llmWorkspaceExecutionStepKind},
		{name: "assembling", state: common.ArtifactContinuationStateAssembling, wantKind: artifactAssembleStepKind},
		{name: "applying", state: common.ArtifactContinuationStateApplying, wantKind: artifactApplyStepKind},
	} {
		t.Run(tc.name, func(t *testing.T) {
			continuations := memory.NewArtifactContinuationRepository()
			item := newArtifactContinuationForAssembleTest()
			item.ID = common.ArtifactContinuationID("artifact-continuation-recovery-" + tc.name)
			item.RunID = common.RunID("run-recovery-" + tc.name)
			item.State = tc.state
			if err := continuations.Create(ctx, item); err != nil {
				t.Fatalf("create continuation: %v", err)
			}
			orch := &RuntimeOrchestrator{
				evidence: RuntimeEvidenceRepositories{ArtifactContinuations: continuations},
			}

			specs, ok, err := orch.artifactContinuationRecoverySpecs(ctx, &runmodel.Run{ID: item.RunID, WorkspaceID: item.WorkspaceID, InitialStepKind: llmWorkspaceExecutionStepKind})
			if err != nil {
				t.Fatalf("artifactContinuationRecoverySpecs() error = %v", err)
			}
			if !ok || len(specs) != 1 || specs[0].Kind != tc.wantKind {
				t.Fatalf("specs = %+v ok=%v, want one %s spec", specs, ok, tc.wantKind)
			}
			context := mapFromValue(specs[0].Input["artifactContinuationContext"])
			if context["id"] != string(item.ID) || context["state"] != string(tc.state) {
				t.Fatalf("artifactContinuationContext = %+v, want recovered context", context)
			}
		})
	}
}

func TestArtifactContinuationRecoverySpecsIgnoreTerminalState(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	item := newArtifactContinuationForAssembleTest()
	item.ID = "artifact-continuation-recovery-applied"
	item.RunID = "run-recovery-applied"
	item.State = common.ArtifactContinuationStateApplied
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("create continuation: %v", err)
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{ArtifactContinuations: continuations},
	}

	specs, ok, err := orch.artifactContinuationRecoverySpecs(ctx, &runmodel.Run{ID: item.RunID, WorkspaceID: item.WorkspaceID, InitialStepKind: llmWorkspaceExecutionStepKind})
	if err != nil {
		t.Fatalf("artifactContinuationRecoverySpecs() error = %v", err)
	}
	if ok || len(specs) != 0 {
		t.Fatalf("specs = %+v ok=%v, want terminal continuation ignored", specs, ok)
	}
}

func TestHandleArtifactOutputContinuationCreatesPlanAndDispatchesChunk(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	stepRepo := memory.NewStepRepository()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := &runmodel.Run{ID: "run-artifact-plan-dispatch", WorkspaceID: "ws-artifact-plan-dispatch", InitialStepKind: llmWorkspaceExecutionStepKind, State: common.RunStateRunning}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-dispatch/001-plan",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": artifactOutputPhasePlan,
			},
		},
	}
	workspace := &workspacemodel.Workspace{ID: run.WorkspaceID, Title: "ws", State: common.WorkspaceStateRunning, DeliverableDirectory: t.TempDir()}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{ArtifactContinuations: continuations, Patches: memory.NewPatchRepository()},
		eventHub:   events.NewMemoryHub(),
	}
	output := map[string]any{
		"completionState": "continue",
		"outputMode":      outputModeIncrementalArtifact,
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable.txt",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1"},
				"estimatedChunks": 2,
				"estimatedBytes":  128,
			}},
		},
	}

	handled, err := orch.handleArtifactOutputContinuation(ctx, run, step, output, workspace)
	if err != nil {
		t.Fatalf("handleArtifactOutputContinuation() error = %v", err)
	}
	if !handled {
		t.Fatal("handleArtifactOutputContinuation() handled = false")
	}
	items, err := continuations.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list continuations: %v", err)
	}
	if len(items) != 1 || items[0].State != common.ArtifactContinuationStateChunking || items[0].ExpectedSequence != 1 {
		t.Fatalf("continuations = %+v, want one chunking continuation", items)
	}
	dispatched, err := stepRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(dispatched) != 2 {
		t.Fatalf("steps = %+v, want original step plus chunk step", dispatched)
	}
	chunkStep := artifactContinuationTestStepWithKind(dispatched, llmWorkspaceExecutionStepKind, step.ID)
	if chunkStep == nil {
		t.Fatalf("steps = %+v, want dispatched chunk step", dispatched)
	}
	if chunkStep.Kind != llmWorkspaceExecutionStepKind || outputModeFromInput(chunkStep.Input) != outputModeIncrementalArtifact {
		t.Fatalf("chunk step = %+v, want incremental artifact LLM step", chunkStep)
	}
	context := mapFromValue(chunkStep.Input["artifactContinuationContext"])
	if context["id"] != string(items[0].ID) || context["expectedSequence"] != 1 {
		t.Fatalf("artifactContinuationContext = %+v, want created continuation cursor", context)
	}
}

func TestHandleArtifactOutputContinuationRecordsChunkAndDispatchesAssemble(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	stepRepo := memory.NewStepRepository()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := &runmodel.Run{ID: "run-artifact-chunk-dispatch", WorkspaceID: "ws-artifact-chunk-dispatch", InitialStepKind: llmWorkspaceExecutionStepKind, State: common.RunStateRunning}
	item := newArtifactContinuationForAssembleTest()
	item.ID = "artifact-continuation-chunk-dispatch"
	item.WorkspaceID = run.WorkspaceID
	item.RunID = run.ID
	item.State = common.ArtifactContinuationStateChunking
	item.ExpectedSequence = 1
	item.ReceivedSequences = nil
	item.FinalReceived = false
	item.BytesWritten = 0
	item.RollingHash = ""
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("seed continuation: %v", err)
	}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-dispatch/001-chunk",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: artifactContinuationNextStepInput(nil, item),
	}
	deliverableDir := t.TempDir()
	workspace := &workspacemodel.Workspace{ID: run.WorkspaceID, Title: "ws", State: common.WorkspaceStateRunning, DeliverableDirectory: deliverableDir}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{ArtifactContinuations: continuations, Patches: memory.NewPatchRepository()},
		eventHub:   events.NewMemoryHub(),
	}
	content := "assembled content\n"
	output := map[string]any{
		"completionState": "continue",
		"outputMode":      outputModeIncrementalArtifact,
		"artifactChunk": map[string]any{
			"artifactId": item.ArtifactID,
			"targetRef":  item.TargetRef,
			"sequence":   1,
			"content":    content,
			"chunkHash":  "sha256:" + sha256HexBytes([]byte(content)),
			"isFinal":    true,
		},
	}

	handled, err := orch.handleArtifactOutputContinuation(ctx, run, step, output, workspace)
	if err != nil {
		t.Fatalf("handleArtifactOutputContinuation() error = %v", err)
	}
	if !handled {
		t.Fatal("handleArtifactOutputContinuation() handled = false")
	}
	updated, err := continuations.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get continuation: %v", err)
	}
	if updated.State != common.ArtifactContinuationStateApplied || !updated.FinalReceived || updated.ExpectedSequence != 2 {
		t.Fatalf("continuation = %+v, want applied final continuation", updated)
	}
	_, stored, err := continuations.GetChunk(ctx, item.ID, 1)
	if err != nil {
		t.Fatalf("get chunk: %v", err)
	}
	if stored != content {
		t.Fatalf("stored chunk = %q, want %q", stored, content)
	}
	dispatched, err := stepRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if artifactContinuationTestStepWithKind(dispatched, artifactAssembleStepKind, step.ID) == nil ||
		artifactContinuationTestStepWithKind(dispatched, artifactApplyStepKind, step.ID) == nil {
		t.Fatalf("steps = %+v, want assemble and apply steps dispatched", dispatched)
	}
	if chunk := mapFromValue(output["artifactChunk"]); chunk["content"] != nil || chunk["contentOmitted"] != true {
		t.Fatalf("artifactChunk output = %+v, want compact metadata", chunk)
	}
	target, err := os.ReadFile(filepath.Join(deliverableDir, filepath.FromSlash(item.TargetRef)))
	if err != nil {
		t.Fatalf("read applied artifact: %v", err)
	}
	if string(target) != content {
		t.Fatalf("applied artifact = %q, want %q", target, content)
	}
}

func TestHandleArtifactOutputContinuationApplyUpdatesCoverageWorkItems(t *testing.T) {
	ctx := context.Background()
	continuations := memory.NewArtifactContinuationRepository()
	stepRepo := memory.NewStepRepository()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := &runmodel.Run{
		ID:              "run-artifact-coverage-update",
		WorkspaceID:     "ws-artifact-coverage-update",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "produce primary deliverable"},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{{
							"id":       "source_change",
							"required": true,
						}},
					},
				},
			},
		},
	}
	ensureRunCoverageWorkItems(run)
	item := newArtifactContinuationForAssembleTest()
	item.ID = "artifact-continuation-coverage-update"
	item.WorkspaceID = run.WorkspaceID
	item.RunID = run.ID
	item.TargetRef = "deliverable.txt"
	item.State = common.ArtifactContinuationStateChunking
	item.ExpectedSequence = 1
	item.ReceivedSequences = nil
	item.CoverageTargets = []string{"source_change"}
	if err := continuations.Create(ctx, item); err != nil {
		t.Fatalf("seed continuation: %v", err)
	}
	step := &runmodel.Step{
		ID:    "run-artifact-coverage-update/001-chunk",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: artifactContinuationNextStepInput(nil, item),
	}
	deliverableDir := t.TempDir()
	workspace := &workspacemodel.Workspace{ID: run.WorkspaceID, Title: "ws", State: common.WorkspaceStateRunning, DeliverableDirectory: deliverableDir}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{output: map[string]any{"completionState": "completed"}},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{ArtifactContinuations: continuations, Patches: memory.NewPatchRepository()},
		eventHub:   events.NewMemoryHub(),
	}
	content := "deliverable content\n"
	output := map[string]any{
		"completionState": "continue",
		"outputMode":      outputModeIncrementalArtifact,
		"artifactChunk": map[string]any{
			"artifactId": item.ArtifactID,
			"targetRef":  item.TargetRef,
			"sequence":   1,
			"content":    content,
			"chunkHash":  "sha256:" + sha256HexBytes([]byte(content)),
			"isFinal":    true,
		},
	}

	if handled, err := orch.handleArtifactOutputContinuation(ctx, run, step, output, workspace); err != nil || !handled {
		t.Fatalf("handleArtifactOutputContinuation() handled=%v error=%v", handled, err)
	}
	updatedRun, err := runRepo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	var sourceChange map[string]any
	for _, item := range mapSliceFromValue(updatedRun.ExecutionContext[coverageWorkItemsKey]) {
		if stringFromMap(item, "sourceRequirementId") == "source_change" {
			sourceChange = item
			break
		}
	}
	if len(sourceChange) == 0 {
		t.Fatalf("coverageWorkItems = %+v, want source_change item", updatedRun.ExecutionContext[coverageWorkItemsKey])
	}
	missing := stringSet(coverageMissingEvidenceKinds(sourceChange))
	if missing["state_change"] || !missing["validation"] {
		t.Fatalf("source_change missingEvidenceKinds = %+v, want validation only after artifact apply", coverageMissingEvidenceKinds(sourceChange))
	}
}

func artifactContinuationTestStepWithKind(items []*runmodel.Step, kind string, exclude common.StepID) *runmodel.Step {
	for _, item := range items {
		if item != nil && item.ID != exclude && item.Kind == kind {
			return item
		}
	}
	return nil
}

func newArtifactContinuationForAssembleTest() *orchestratormodel.ArtifactContinuation {
	return &orchestratormodel.ArtifactContinuation{
		ID:               common.ArtifactContinuationID("artifact-continuation-assemble-001"),
		WorkspaceID:      common.WorkspaceID("ws-assemble"),
		RunID:            common.RunID("run-assemble"),
		RootStepID:       common.StepID("run-assemble/001-step"),
		ArtifactID:       "artifact-001",
		TargetRef:        "deliverable/example.txt",
		Operation:        "replace",
		CoverageTargets:  []string{"coverage-001"},
		State:            common.ArtifactContinuationStateAssembling,
		ExpectedSequence: 2,
	}
}

func saveArtifactChunkForAssembleTest(t *testing.T, ctx context.Context, repo *memory.ArtifactContinuationRepository, item *orchestratormodel.ArtifactContinuation, sequence int, content string, targetRef string) {
	t.Helper()
	if err := repo.SaveChunk(ctx, &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID: item.ID,
		Sequence:       sequence,
		ArtifactID:     item.ArtifactID,
		TargetRef:      targetRef,
		ContentHash:    sha256HexBytes([]byte(content)),
		Bytes:          int64(len([]byte(content))),
	}, content); err != nil {
		t.Fatalf("save chunk %d: %v", sequence, err)
	}
}
