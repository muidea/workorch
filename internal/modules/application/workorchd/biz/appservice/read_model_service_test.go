package appservice

import (
	"context"
	"errors"
	"reflect"
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
	if !timelineContainsStepOutput(item.Timeline, string(stepID), "llm_execution_graph_plan") {
		t.Fatalf("timeline = %+v, want step output timeline item", item.Timeline)
	}
	if !timelineContainsStreamDelta(item.Timeline, string(stepID), `{"completionState":"continue"`) {
		t.Fatalf("timeline = %+v, want stream delta timeline item", item.Timeline)
	}
}

func timelineContainsStepOutput(items []RuntimeTimelineItem, stepID string, stage string) bool {
	for _, item := range items {
		if item.Source != "step" || item.StepID != stepID || item.Output == nil {
			continue
		}
		if item.Output["stage"] == stage {
			return true
		}
	}
	return false
}

func timelineContainsStreamDelta(items []RuntimeTimelineItem, stepID string, delta string) bool {
	for _, item := range items {
		if item.Source != "step_stream" || item.StepID != stepID {
			continue
		}
		if item.Type == "delta" && item.Data["delta"] == delta {
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
			if snapshot.Workspace.ActiveRunID != string(runID) {
				t.Fatalf("activeRunId = %q, want %s", snapshot.Workspace.ActiveRunID, runID)
			}
		})
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
