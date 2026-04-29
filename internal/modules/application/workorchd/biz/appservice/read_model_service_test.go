package appservice

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestReadRunSnapshotWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadRunSnapshotWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestReadRunSnapshotWithBackendReturnsBlockedDecisionReadModel(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-decision-snapshot"
	started := prepareDecisionRun(t, dataRoot, "ws-run-snapshot", runID)

	item, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if item.Workspace.ID != "ws-run-snapshot" {
		t.Fatalf("workspace = %+v, want ws-run-snapshot", item.Workspace)
	}
	if item.Run.ID != runID || item.Run.State != "blocked_on_decision" {
		t.Fatalf("run = %+v, want blocked decision run", item.Run)
	}
	if item.Step == nil || item.Step.ID == "" || item.Step.State == "" {
		t.Fatalf("step = %+v, want current step", item.Step)
	}
	if len(item.Steps) == 0 {
		t.Fatalf("steps = %+v, want non-empty", item.Steps)
	}
	if len(item.Decisions) != 1 || item.Decisions[0].ID != started.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", item.Decisions, started.BlockingTicketID)
	}
}

func TestReadRunSnapshotWithBackendReturnsExecutionEvidence(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-run-evidence-snapshot")
	runID := common.RunID("run-evidence-snapshot")
	stepID := common.StepID("run-evidence-snapshot/step-1")
	toolCallID := common.ToolCallID("tool-call-evidence-snapshot")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "run evidence snapshot",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   "validation",
		State:  common.StepStateCompleted,
		Output: map[string]any{"result": "done"},
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		Arguments:   map[string]any{"command": "go test ./..."},
		State:       common.ToolCallStateCompleted,
		Output:      map[string]any{"exitCode": 0},
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-evidence-snapshot"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-evidence-snapshot"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "go_test",
		State:          common.ValidationResultStatePassed,
		Summary:        "passed",
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-evidence-snapshot"),
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             map[string]any{"thread": "main"},
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	resultID := common.ResultID("result-evidence-snapshot")
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		DoDID:        common.DoDID("dod-evidence-snapshot"),
		State:        common.ResultStateAccepted,
		Title:        "Accepted Result",
		Summary:      "formal outcome",
		SourceType:   "dod_assessment",
		SourceRef:    string(stepID),
		EvidenceRefs: []string{string(stepID), "validation-evidence-snapshot"},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           common.ArtifactID("artifact-evidence-snapshot"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     resultID,
		StepID:       stepID,
		ArtifactType: "report",
		Name:         "outcome.md",
		Summary:      "formal artifact",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: []string{string(resultID)},
		Metadata:     map[string]any{"format": "markdown"},
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	first, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("first ReadRunSnapshotWithBackend: %v", err)
	}
	second, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("second ReadRunSnapshotWithBackend: %v", err)
	}

	if len(first.Steps) != 1 || first.Steps[0].ID != string(stepID) {
		t.Fatalf("steps = %+v, want %s", first.Steps, stepID)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != string(toolCallID) {
		t.Fatalf("toolCalls = %+v, want %s", first.ToolCalls, toolCallID)
	}
	if len(first.Patches) != 1 || first.Patches[0].ToolCallID != string(toolCallID) {
		t.Fatalf("patches = %+v, want tool call ref %s", first.Patches, toolCallID)
	}
	if len(first.ValidationResults) != 1 || first.ValidationResults[0].State != "passed" {
		t.Fatalf("validationResults = %+v, want passed", first.ValidationResults)
	}
	if len(first.RuntimeCheckpoints) != 1 || first.RuntimeCheckpoints[0].Data["thread"] != "main" {
		t.Fatalf("runtimeCheckpoints = %+v, want checkpoint data", first.RuntimeCheckpoints)
	}
	if len(first.Results) != 1 || first.Results[0].Summary != "formal outcome" {
		t.Fatalf("results = %+v, want formal outcome", first.Results)
	}
	if len(first.Artifacts) != 1 || first.Artifacts[0].ResultID != string(resultID) || first.Artifacts[0].Metadata["format"] != "markdown" {
		t.Fatalf("artifacts = %+v, want result-linked artifact", first.Artifacts)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("snapshot mismatch after reload: first=%+v second=%+v", first, second)
	}
}

func TestReadRunStatusWithBackendReturnsBlockedApprovalSummary(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-status"
	started := prepareApprovalRun(t, dataRoot, "ws-run-status", runID)

	item, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunStatusWithBackend: %v", err)
	}
	if item.ID != runID || item.WorkspaceID != "ws-run-status" || item.State != "blocked_on_approval" {
		t.Fatalf("status = %+v, want blocked approval status", item)
	}
	if item.StepCount == 0 || item.CurrentStepID == "" || item.CurrentStepState == "" {
		t.Fatalf("status = %+v, want populated step summary", item)
	}
	if item.ApprovalCount != 1 || item.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("status = %+v, want approval count 1 and blocking approval %s", item, started.BlockingApprovalID)
	}
}

func TestReadRunObservabilityWithSessionReturnsDerivedRuntimeProgress(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "memory", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-run-observability")
	runID := common.RunID("run-observability")
	stepID := common.StepID("run-observability/step-1")
	toolCallID := common.ToolCallID("tool-call-observability")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "observability", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": `{"completionState":"continue"`},
				{"type": "done"},
			},
		},
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "graph/mock",
		State:       common.ToolCallStateCompleted,
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-observability"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		TargetRef:   "README.md",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-observability"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "runtime_step",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-observability"),
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateCompleted,
		OccurredAt:       "2026-04-25T10:00:00Z",
		Data:             map[string]any{"event": "step_completed"},
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: "test",
		SubjectType:  "run",
		SubjectID:    string(runID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      events.RunPayload("", "", common.RunStateCompleted, "", "", ""),
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	item, err := ReadRunObservabilityWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunObservabilityWithSession: %v", err)
	}
	if item.Progress.StepTotal != 1 || item.Progress.StepCompleted != 1 || !item.Progress.Terminal {
		t.Fatalf("progress = %+v, want terminal completed progress", item.Progress)
	}
	if item.Evidence.ToolCallCompleted != 1 || item.Evidence.PatchApplied != 1 || item.Evidence.ValidationPassed != 1 || item.Evidence.RuntimeCheckpointTotal != 1 || item.Evidence.EventTotal == 0 {
		t.Fatalf("evidence = %+v, want derived evidence counters", item.Evidence)
	}
	if item.LatestCheckpoint == nil || item.LatestCheckpoint.ID != "checkpoint-observability" {
		t.Fatalf("latest checkpoint = %+v, want checkpoint-observability", item.LatestCheckpoint)
	}
	if len(item.Timeline) < 2 {
		t.Fatalf("timeline = %+v, want checkpoint and events", item.Timeline)
	}
	if !timelineContainsStepOutput(item.Timeline, string(stepID), "llm_execution_graph_plan", "completed") {
		t.Fatalf("timeline = %+v, want step output timeline item", item.Timeline)
	}
	if !timelineContainsStreamDelta(item.Timeline, string(stepID), `{"completionState":"continue"`, "completed") {
		t.Fatalf("timeline = %+v, want stream delta timeline item", item.Timeline)
	}
	if !timelineContainsStepOccurredAt(item.Timeline, "step", string(stepID), "2026-04-25T10:00:00Z") {
		t.Fatalf("timeline = %+v, want step occurrence derived from checkpoint", item.Timeline)
	}
	if !timelineContainsStepOccurredAt(item.Timeline, "step_stream", string(stepID), "2026-04-25T10:00:00Z") {
		t.Fatalf("timeline = %+v, want stream occurrence derived from step checkpoint", item.Timeline)
	}
}

func TestReadRunTaskProgressWithBackendReturnsFormalResultProgress(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-task-progress-result")
	runID := common.RunID("run-task-progress-result")
	stepID := common.StepID("run-task-progress-result/step-1")
	resultID := common.ResultID("result-task-progress")
	artifactID := common.ArtifactID("artifact-task-progress")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "task progress", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateCompleted}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:          common.ValidationResultID("validation-task-progress"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ValidationResultStatePassed,
		Summary:     "validation passed",
	}); err != nil {
		t.Fatalf("create validation: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		State:        common.ResultStateAccepted,
		Title:        "Accepted Result",
		Summary:      "formal task outcome",
		EvidenceRefs: []string{string(stepID), "validation-task-progress"},
		ArtifactRefs: []common.ArtifactID{artifactID},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           artifactID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     resultID,
		ArtifactType: "report",
		State:        common.ArtifactStateAvailable,
		Summary:      "formal artifact",
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	progress, err := ReadRunTaskProgressWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunTaskProgressWithBackend: %v", err)
	}
	if progress.WorkspaceID != string(workspaceID) || progress.RunID != string(runID) || progress.State != "completed" {
		t.Fatalf("progress identity = %+v, want completed run progress", progress)
	}
	if progress.Recovery.Mode != "none" || progress.Recovery.NextAction != "inspect_result" {
		t.Fatalf("recovery = %+v, want inspect completed result", progress.Recovery)
	}
	if progress.Summary != "formal task outcome" {
		t.Fatalf("summary = %q, want formal task outcome", progress.Summary)
	}
	if !progressContainsMilestone(progress, "formal_result", "completed") {
		t.Fatalf("milestones = %+v, want completed formal result", progress.Milestones)
	}
	if !progressContainsItem(progress, string(resultID), "result", "accepted") || !progressContainsItem(progress, string(artifactID), "artifact", "available") {
		t.Fatalf("items = %+v, want result and artifact items", progress.Items)
	}
}

func TestReadRunTaskProgressWithBackendReturnsDecisionRecovery(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-task-progress-decision"
	started := prepareDecisionRun(t, dataRoot, "ws-task-progress-decision", runID)

	progress, err := ReadRunTaskProgressWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunTaskProgressWithBackend: %v", err)
	}
	if progress.Recovery.Mode != "wait_decision" || progress.Recovery.BlockingTicketID != started.BlockingTicketID {
		t.Fatalf("recovery = %+v, want decision blocker %s", progress.Recovery, started.BlockingTicketID)
	}
	if len(progress.Blockers) != 1 || progress.Blockers[0].Kind != "decision" || progress.Blockers[0].ID != started.BlockingTicketID {
		t.Fatalf("blockers = %+v, want decision blocker %s", progress.Blockers, started.BlockingTicketID)
	}
	if !progressContainsMilestone(progress, "governance", "blocked") {
		t.Fatalf("milestones = %+v, want blocked governance", progress.Milestones)
	}
}

func TestReadWorkspaceTaskProgressWithBackendReturnsActiveRunProgress(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-task-progress-approval"
	runID := "run-task-progress-approval"
	started := prepareApprovalRun(t, dataRoot, workspaceID, runID)

	progress, err := ReadWorkspaceTaskProgressWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceTaskProgressWithBackend: %v", err)
	}
	if progress.WorkspaceID != workspaceID || progress.RunID != runID {
		t.Fatalf("progress identity = %+v, want workspace active run progress", progress)
	}
	if progress.Recovery.Mode != "wait_approval" || progress.Recovery.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("recovery = %+v, want approval blocker %s", progress.Recovery, started.BlockingApprovalID)
	}
}

func TestReadRunContextBundleWithBackendReturnsRecoveryContextAndCompressionCandidates(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-context-bundle")
	runID := common.RunID("run-context-bundle")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:               workspaceID,
		Title:            "context bundle workspace",
		State:            common.WorkspaceStateActive,
		WorkingDirectory: "workspaces/context-bundle",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{"id": "goal-context-bundle", "text": "deliver context bundle"},
			"dod":  map[string]any{"id": "dod-context-bundle", "state": "ready", "items": []any{"bundle exposes recovery context"}},
			"skills": []any{
				map[string]any{"id": "skill-context", "name": "context"},
			},
			"mcpServers": []any{
				map[string]any{"id": "mcp-context", "name": "context mcp"},
			},
			"memories": []any{
				map[string]any{"id": "memory-context", "summary": "prior context"},
			},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 0; idx < 11; idx++ {
		stepID := common.StepID("run-context-bundle/step-" + strconv.Itoa(idx+1))
		if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
			ID:     stepID,
			RunID:  runID,
			Kind:   "llm/workspace_execution",
			State:  common.StepStateCompleted,
			Output: map[string]any{"stage": "context_bundle_stage"},
		}); err != nil {
			t.Fatalf("create step: %v", err)
		}
		if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
			ID:               common.CheckpointID("checkpoint-context-bundle-" + strconv.Itoa(idx+1)),
			WorkspaceID:      workspaceID,
			RunID:            runID,
			CheckpointType:   common.CheckpointTypeRunState,
			CurrentStepID:    stepID,
			RunStateSnapshot: common.RunStateRunning,
			OccurredAt:       "2026-04-29T10:00:" + twoDigitSecond(idx) + "Z",
			Data:             map[string]any{"event": "step_completed"},
		}); err != nil {
			t.Fatalf("create checkpoint: %v", err)
		}
	}

	bundle, err := ReadRunContextBundleWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunContextBundleWithBackend: %v", err)
	}
	if bundle.WorkspaceID != string(workspaceID) || bundle.RunID != string(runID) || bundle.Mode != "run_recovery" {
		t.Fatalf("bundle identity = %+v, want run recovery bundle", bundle)
	}
	if bundle.TaskProgress.RunID != string(runID) || bundle.Recovery.Mode != "continue" {
		t.Fatalf("bundle progress/recovery = %+v/%+v, want continue run progress", bundle.TaskProgress, bundle.Recovery)
	}
	if bundle.CompressedContext.Version != "workorch.compressedContext.v1" ||
		bundle.CompressedContext.Mode != "deterministic_recovery_summary" ||
		bundle.CompressedContext.RunID != string(runID) {
		t.Fatalf("compressedContext = %+v, want deterministic compressed context for run", bundle.CompressedContext)
	}
	if bundle.LatestCheckpoint == nil || bundle.LatestCheckpoint.ID != "checkpoint-context-bundle-11" {
		t.Fatalf("latest checkpoint = %+v, want checkpoint-context-bundle-11", bundle.LatestCheckpoint)
	}
	if !contextBundleContainsSection(bundle, "goal") || !contextBundleContainsSection(bundle, "dod") || !contextBundleContainsSection(bundle, "task_progress") {
		t.Fatalf("sections = %+v, want goal/dod/task_progress", bundle.Sections)
	}
	if len(bundle.RecentEvidence) != 12 {
		t.Fatalf("recentEvidence = %d, want 12", len(bundle.RecentEvidence))
	}
	if len(bundle.CompressedContext.RecentEvidence) != 6 {
		t.Fatalf("compressed recentEvidence = %d, want 6", len(bundle.CompressedContext.RecentEvidence))
	}
	if !contextBundleContainsCompressionCandidate(bundle, "recent_evidence") || !contextBundleContainsCompressionCandidate(bundle, "task_progress") {
		t.Fatalf("compressionCandidates = %+v, want recent evidence and task progress candidates", bundle.CompressionCandidates)
	}
}

func TestReadRunTaskDecompositionsWithBackendReturnsExecutionGraphProjection(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-task-decomposition")
	runID := common.RunID("run-task-decomposition")
	rootStepID := common.StepID("run-task-decomposition/step-1")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "task decomposition", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"executionGraph": map[string]any{
				"nodes": []map[string]any{
					{"id": "inspect", "kind": "shell/command", "input": map[string]any{"command": "hostname"}},
					{"id": "verify", "kind": "shell/command", "dependsOn": []string{"inspect"}, "input": map[string]any{"command": "true"}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("create root step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-task-decomposition/block/inspect"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"graphNodeId":     "inspect",
			"graphRootStepId": string(rootStepID),
		},
		Output: map[string]any{"exitCode": 0},
	}); err != nil {
		t.Fatalf("create inspect step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-task-decomposition/block/verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"graphNodeId":     "verify",
			"graphRootStepId": string(rootStepID),
		},
	}); err != nil {
		t.Fatalf("create verify step: %v", err)
	}

	items, err := ReadRunTaskDecompositionsWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunTaskDecompositionsWithBackend: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("task decompositions = %+v, want one graph projection", items)
	}
	item := items[0]
	if item.RootStepID != string(rootStepID) || item.State != "running" || item.NodeTotal != 2 || item.NodeCompleted != 1 || item.NodeRunning != 1 {
		t.Fatalf("task decomposition = %+v, want running 1/2 completed", item)
	}
	if len(item.Nodes) != 2 || item.Nodes[0].ID != "inspect" || item.Nodes[0].State != "completed" || item.Nodes[1].ID != "verify" || item.Nodes[1].State != "running" {
		t.Fatalf("nodes = %+v, want inspect completed and verify running", item.Nodes)
	}
	if len(item.Nodes[1].DependsOn) != 1 || item.Nodes[1].DependsOn[0] != "inspect" {
		t.Fatalf("verify dependsOn = %+v, want inspect", item.Nodes[1].DependsOn)
	}

	bundle, err := ReadRunContextBundleWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunContextBundleWithBackend: %v", err)
	}
	if !contextBundleContainsSection(bundle, "task_decomposition") {
		t.Fatalf("context bundle sections = %+v, want task_decomposition", bundle.Sections)
	}
}

func progressContainsMilestone(progress TaskProgress, id string, state string) bool {
	for _, item := range progress.Milestones {
		if item.ID == id && item.State == state {
			return true
		}
	}
	return false
}

func contextBundleContainsSection(bundle ContextBundle, id string) bool {
	for _, item := range bundle.Sections {
		if item.ID == id {
			return true
		}
	}
	return false
}

func contextBundleContainsCompressionCandidate(bundle ContextBundle, sectionID string) bool {
	for _, item := range bundle.CompressionCandidates {
		if item.SectionID == sectionID {
			return true
		}
	}
	return false
}

func twoDigitSecond(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func progressContainsItem(progress TaskProgress, id string, kind string, state string) bool {
	for _, item := range progress.Items {
		if item.ID == id && item.Kind == kind && item.State == state {
			return true
		}
	}
	return false
}

func timelineContainsStepOutput(items []RuntimeTimelineItem, stepID string, stage string, runState string) bool {
	for _, item := range items {
		if item.Source != "step" || item.StepID != stepID || item.Output == nil {
			continue
		}
		if item.Output["stage"] == stage && item.RunState == runState {
			return true
		}
	}
	return false
}

func timelineContainsStreamDelta(items []RuntimeTimelineItem, stepID string, delta string, runState string) bool {
	for _, item := range items {
		if item.Source != "step_stream" || item.StepID != stepID {
			continue
		}
		if item.Type == "delta" && item.Data["delta"] == delta && item.RunState == runState {
			return true
		}
	}
	return false
}

func timelineContainsStepOccurredAt(items []RuntimeTimelineItem, source string, stepID string, occurredAt string) bool {
	for _, item := range items {
		if item.Source == source && item.StepID == stepID && item.OccurredAt == occurredAt {
			return true
		}
	}
	return false
}

func TestReadWorkspaceSnapshotWithBackendReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-workspace-snapshot"
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, workspaceID, "workspace snapshot", "", "workspaces/workspace snapshot"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          "run-workspace-snapshot-completed",
		WorkspaceID: common.WorkspaceID(workspaceID),
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          "result-workspace-snapshot",
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       "run-workspace-snapshot-completed",
		State:       common.ResultStateAccepted,
		Summary:     "workspace formal outcome",
	}); err != nil {
		t.Fatalf("create workspace result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           "artifact-workspace-snapshot",
		WorkspaceID:  common.WorkspaceID(workspaceID),
		RunID:        "run-workspace-snapshot-completed",
		ResultID:     "result-workspace-snapshot",
		ArtifactType: "report",
		State:        common.ArtifactStateAvailable,
	}); err != nil {
		t.Fatalf("create workspace artifact: %v", err)
	}

	decisionRunID := "run-decision-workspace-snapshot"
	decisionRun := prepareDecisionRun(t, dataRoot, workspaceID, decisionRunID)
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, decisionRun.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	approvalRunID := "run-approval-workspace-snapshot"
	approvalRun := prepareApprovalRun(t, dataRoot, workspaceID, approvalRunID)
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(approvalRunID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if item.Workspace.ID != workspaceID {
		t.Fatalf("workspace = %+v, want %s", item.Workspace, workspaceID)
	}
	if item.Workspace.State != "blocked" || item.Workspace.ActiveRunID != approvalRunID {
		t.Fatalf("workspace projection = %+v, want blocked active run %s", item.Workspace, approvalRunID)
	}
	if len(item.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", item.Runs)
	}
	if len(item.Results) != 1 || item.Results[0].Summary != "workspace formal outcome" {
		t.Fatalf("results = %+v, want workspace formal outcome", item.Results)
	}
	if len(item.Artifacts) != 1 || item.Artifacts[0].ResultID != "result-workspace-snapshot" {
		t.Fatalf("artifacts = %+v, want workspace artifact", item.Artifacts)
	}
	if len(item.Decisions) != 1 || item.Decisions[0].ID != decisionRun.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", item.Decisions, decisionRun.BlockingTicketID)
	}
	if len(item.Approvals) != 1 || item.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", item.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestReadWorkspaceSnapshotWithBackendReturnsPersistedEventRecords(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-event-record-snapshot"
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, workspaceID, "event record snapshot", "", "workspaces/event record snapshot"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if len(item.EventRecords) == 0 {
		t.Fatalf("eventRecords = %+v, want persisted workspace event", item.EventRecords)
	}
	if item.EventRecords[0].WorkspaceID != workspaceID || item.EventRecords[0].Topic != events.TopicWorkspaceCreated {
		t.Fatalf("eventRecords = %+v, want workspace-created event", item.EventRecords)
	}
}

func TestReadWorkspaceSnapshotProjectsWorkspaceStateFromRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-projection")
	runID := common.RunID("run-projection")
	workspace := &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "projection",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	tests := []struct {
		runState       common.RunState
		workspaceState string
	}{
		{runState: common.RunStateRunning, workspaceState: "running"},
		{runState: common.RunStateBlockedOnDecision, workspaceState: "waiting_decision"},
		{runState: common.RunStateBlockedOnApproval, workspaceState: "blocked"},
		{runState: common.RunStateValidating, workspaceState: "validating"},
		{runState: common.RunStateCompleted, workspaceState: "completed"},
		{runState: common.RunStateFailed, workspaceState: "failed"},
		{runState: common.RunStateCancelled, workspaceState: "failed"},
		{runState: common.RunStateSuperseded, workspaceState: "failed"},
	}
	for _, tc := range tests {
		t.Run(string(tc.runState), func(t *testing.T) {
			run.State = tc.runState
			if err := exports.Repositories.Runs.Update(ctx, run); err != nil {
				t.Fatalf("update run: %v", err)
			}
			snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
			if err != nil {
				t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
			}
			if snapshot.Workspace.State != tc.workspaceState {
				t.Fatalf("workspace state = %q, want %q", snapshot.Workspace.State, tc.workspaceState)
			}
			expectedActiveRunID := string(runID)
			if tc.runState == common.RunStateCompleted ||
				tc.runState == common.RunStateFailed ||
				tc.runState == common.RunStateCancelled ||
				tc.runState == common.RunStateSuperseded {
				expectedActiveRunID = ""
			}
			if snapshot.Workspace.ActiveRunID != expectedActiveRunID {
				t.Fatalf("activeRunId = %q, want %q", snapshot.Workspace.ActiveRunID, expectedActiveRunID)
			}
		})
	}
}

func TestReadWorkspaceSnapshotProjectsAcceptedResultOverStaleTerminalActiveRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-stale-active-result")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-result-completed")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "stale projection",
		State:       common.WorkspaceStateFailed,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:            staleRunID,
		WorkspaceID:   workspaceID,
		State:         common.RunStateFailed,
		FailureReason: "old provider failure",
	}); err != nil {
		t.Fatalf("create stale run: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          completedRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(completedRunID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       completedRunID,
		State:       common.ResultStateAccepted,
		Summary:     "formal result",
		CreatedAt:   "2026-04-29T10:00:00Z",
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateCompleted) {
		t.Fatalf("workspace state = %q, want completed", snapshot.Workspace.State)
	}
	if snapshot.Workspace.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty for terminal projection", snapshot.Workspace.ActiveRunID)
	}
	if len(snapshot.Results) != 1 || snapshot.Results[0].RunID != string(completedRunID) || snapshot.Results[0].Summary != "formal result" {
		t.Fatalf("results = %+v, want accepted formal result for completed run", snapshot.Results)
	}
}

func TestReadTerminalRunModelsWithBackendRemainStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-run-terminal-reload"
	runID := "run-approval-terminal-reload"
	started := prepareApprovalRun(t, dataRoot, workspaceID, runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}

	firstRun, err := ReadRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunWithBackend: %v", err)
	}
	firstSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunSnapshotWithBackend: %v", err)
	}
	firstStatus, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunStatusWithBackend: %v", err)
	}
	firstWorkspace, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("first ReadWorkspaceSnapshotWithBackend: %v", err)
	}

	secondRun, err := ReadRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunWithBackend: %v", err)
	}
	secondSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunSnapshotWithBackend: %v", err)
	}
	secondStatus, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunStatusWithBackend: %v", err)
	}
	secondWorkspace, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("second ReadWorkspaceSnapshotWithBackend: %v", err)
	}

	if firstRun.State != "failed" || firstRun.FailureReason == "" {
		t.Fatalf("first run = %+v, want failed with failure reason", firstRun)
	}
	if !reflect.DeepEqual(firstRun, secondRun) {
		t.Fatalf("run mismatch after reload: first=%+v second=%+v", firstRun, secondRun)
	}
	if !reflect.DeepEqual(firstSnapshot, secondSnapshot) {
		t.Fatalf("snapshot mismatch after reload: first=%+v second=%+v", firstSnapshot, secondSnapshot)
	}
	if !reflect.DeepEqual(firstStatus, secondStatus) {
		t.Fatalf("status mismatch after reload: first=%+v second=%+v", firstStatus, secondStatus)
	}
	if !reflect.DeepEqual(firstWorkspace, secondWorkspace) {
		t.Fatalf("workspace snapshot mismatch after reload: first=%+v second=%+v", firstWorkspace, secondWorkspace)
	}
}
