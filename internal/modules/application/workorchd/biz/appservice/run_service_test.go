package appservice

import (
	"context"
	"errors"
	"testing"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateRunWithSession(context.Background(), nil, "ws-1", "run-1", "", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateRunWithBackendReturnsCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run created workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run-created", "goal-created", "dod-created")
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if item.ID != "run-created" || item.WorkspaceID != "ws-run-created" {
		t.Fatalf("run = %+v, want created run", item)
	}
	if item.State != "created" || item.GoalID != "goal-created" || item.DoDID != "dod-created" {
		t.Fatalf("run fields = %+v, want created state with goal/dod", item)
	}
}

func TestStartRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := StartRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestResumeRunWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ResumeRunWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateRunWithBackendReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	if _, err := CreateRunWithBackend(context.Background(), "file", t.TempDir(), "ws-missing", "run-created", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestReadRunWithBackendReturnsPersistedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run read workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run-read", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := ReadRunWithBackend(context.Background(), "file", dataRoot, "run-read")
	if err != nil {
		t.Fatalf("ReadRunWithBackend: %v", err)
	}
	if item.ID != "run-read" || item.WorkspaceID != "ws-run-read" || item.State != "created" {
		t.Fatalf("run = %+v, want persisted run", item)
	}
}

func TestListRunsWithBackendReturnsFilteredRuns(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-a: %v", err)
	}
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run-b", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run-a", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend run-a: %v", err)
	}

	items, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "")
	if err != nil {
		t.Fatalf("ListRunsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "run-a" || items[1].ID != "run-b" {
		t.Fatalf("run ids = %q/%q, want run-a/run-b", items[0].ID, items[1].ID)
	}

	filtered, err := ListRunsWithBackend(context.Background(), "file", dataRoot, "ws-run-b")
	if err != nil {
		t.Fatalf("ListRunsWithBackend filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "run-b" {
		t.Fatalf("filtered runs = %+v, want run-b only", filtered)
	}
}

func TestStartRunWithBackendReturnsCompletedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run start workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run-start", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start")
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if item.ID != "run-start" || item.State != "completed" {
		t.Fatalf("run = %+v, want run-start/completed", item)
	}
	if item.BlockingTicketID != "" || item.BlockingApprovalID != "" || item.FailureReason != "" {
		t.Fatalf("run blocking/failure = %+v, want empty", item)
	}
}

func TestStartRunWithBackendReturnsCompletedRunForCompletedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run start restart workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run-start-restart", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart"); err != nil {
		t.Fatalf("first StartRunWithBackend: %v", err)
	}

	item, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart")
	if err != nil {
		t.Fatalf("second StartRunWithBackend: %v", err)
	}
	if item.ID != "run-start-restart" || item.State != "completed" || item.FailureReason != "" {
		t.Fatalf("run = %+v, want completed without failure reason", item)
	}
}

func TestStartRunWithBackendReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-start-blocked-decision", "run start blocked decision workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-blocked-decision", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if item.State != "blocked_on_decision" || item.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", item)
	}
}

func TestResumeRunWithBackendReturnsCompletedRunAfterDecisionResolved(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-decision", "run resume decision workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-decision", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	if err := session.Bootstrap.Exports().Services.DecisionService.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: "resolution-run-resume-decision",
		TicketID:     common.TicketID(started.BlockingTicketID),
		Type:         decisioncommon.ResolutionTypeApprovePath,
	}); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}
	if item.State != "completed" || item.BlockingTicketID != "" || item.FailureReason != "" {
		t.Fatalf("run = %+v, want completed with cleared block", item)
	}
}

func TestResumeRunWithBackendReturnsCompletedRunAfterApprovalApproved(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-approval", "run resume approval workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-approval", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{
		ApprovalID: common.ApprovalID(started.BlockingApprovalID),
	}); err != nil {
		t.Fatalf("EnsureApproved: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}
	if item.State != "completed" || item.BlockingApprovalID != "" || item.FailureReason != "" {
		t.Fatalf("run = %+v, want completed with cleared approval", item)
	}
}

func TestResumeRunWithBackendReturnsCompletedRunForCompletedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-resume-completed"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-completed", "run resume completed workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-completed", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("first ResumeRunWithBackend: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ResumeRunWithBackend: %v", err)
	}
	if item.ID != runID || item.State != "completed" || item.FailureReason != "" {
		t.Fatalf("run = %+v, want completed without failure reason", item)
	}
}

func TestResumeRunWithBackendReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-rejected"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-approval-rejected", "run resume approval rejected workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-approval-rejected", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureRejected(ctx, approvalmodule.ResolveApprovalCommand{
		ApprovalID: common.ApprovalID(started.BlockingApprovalID),
	}); err != nil {
		t.Fatalf("EnsureRejected: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}
	if item.State != "failed" || item.BlockingApprovalID != "" || item.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason and cleared approval", item)
	}
}

func TestResumeRunWithBackendReturnsFailedRunForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-rejected-restart"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-failed", "run resume failed workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-failed", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("first ResumeRunWithBackend: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ResumeRunWithBackend: %v", err)
	}
	if item.ID != runID || item.State != "failed" || item.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with failure reason", item)
	}
}

func TestStartRunWithBackendReturnsConflictForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-failed-start-conflict"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-start-failed", "run start failed workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-failed", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}

	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResumeRunWithBackendReturnsBlockedWhenDependencyPending(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-pending"
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-pending", "run resume pending workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-pending", runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("err = %v, want blocked", err)
	}
}

func TestResumeRunWithBackendReturnsConflictForCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run resume conflict workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run-resume-conflict", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, "run-resume-conflict"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestReadRunWithSessionReturnsFailureReason(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	if err := session.Bootstrap.Exports().Services.WorkspaceService.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: "ws-run-failure",
		Title:       "run failure workspace",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := session.Bootstrap.Exports().Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:       "run-failure",
		WorkspaceID: "ws-run-failure",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := session.Bootstrap.Exports().Services.RunService.FailRun(ctx, runmodule.FailRunCommand{
		RunID:  "run-failure",
		Reason: "manual fail",
	}); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	item, err := ReadRunWithSession(ctx, session, "run-failure")
	if err != nil {
		t.Fatalf("ReadRunWithSession: %v", err)
	}
	if item.State != "failed" || item.FailureReason != "manual fail" {
		t.Fatalf("run = %+v, want failed/manual fail", item)
	}
}
