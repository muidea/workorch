package appservice

import (
	"context"
	"errors"
	"reflect"
	"testing"

	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadRunSnapshotWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadRunSnapshotWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestReadRunSnapshotWithBackendReturnsBlockedDecisionReadModel(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-snapshot"
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
	if len(item.Observed) == 0 {
		t.Fatalf("observed = %+v, want non-empty", item.Observed)
	}
}

func TestReadRunStatusWithBackendReturnsBlockedApprovalSummary(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-status"
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

func TestReadWorkspaceSnapshotWithBackendReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-workspace-snapshot"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, workspaceID, "workspace snapshot"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceID, "run-workspace-snapshot-completed", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend completed: %v", err)
	}
	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-workspace-snapshot-completed"); err != nil {
		t.Fatalf("StartRunWithBackend completed: %v", err)
	}

	decisionRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-workspace-snapshot"
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceID, decisionRunID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend decision: %v", err)
	}
	decisionRun, err := StartRunWithBackend(context.Background(), "file", dataRoot, decisionRunID)
	if err != nil {
		t.Fatalf("StartRunWithBackend decision: %v", err)
	}
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, decisionRun.BlockingTicketID); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, decisionRunID); err != nil {
		t.Fatalf("ResumeRunWithBackend decision: %v", err)
	}

	approvalRunID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-workspace-snapshot"
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceID, approvalRunID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend approval: %v", err)
	}
	approvalRun, err := StartRunWithBackend(context.Background(), "file", dataRoot, approvalRunID)
	if err != nil {
		t.Fatalf("StartRunWithBackend approval: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if item.Workspace.ID != workspaceID {
		t.Fatalf("workspace = %+v, want %s", item.Workspace, workspaceID)
	}
	if len(item.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", item.Runs)
	}
	if len(item.Decisions) != 1 || item.Decisions[0].ID != decisionRun.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", item.Decisions, decisionRun.BlockingTicketID)
	}
	if len(item.Approvals) != 1 || item.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", item.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestReadTerminalRunModelsWithBackendRemainStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-run-terminal-reload"
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-terminal-reload"
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
