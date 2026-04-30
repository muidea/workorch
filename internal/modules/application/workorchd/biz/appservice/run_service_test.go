package appservice

import (
	"context"
	"errors"
	"testing"

	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run created workspace", "", "workspaces/run created workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "created goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, "ws-run-created", []string{"created dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-created", "run-created", goal.ID, dod.ID)
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if item.ID != "run-created" || item.WorkspaceID != "ws-run-created" {
		t.Fatalf("run = %+v, want created run", item)
	}
	if item.State != "created" || item.GoalID != goal.ID || item.DoDID != dod.ID {
		t.Fatalf("run fields = %+v, want created state with goal/dod", item)
	}
	goalContext, _ := item.ExecutionContext["goal"].(map[string]any)
	dodContext, _ := item.ExecutionContext["dod"].(map[string]any)
	if goalContext["text"] != "created goal" {
		t.Fatalf("execution goal context = %+v, want created goal", goalContext)
	}
	if dodContext["state"] != "ready" {
		t.Fatalf("execution dod context = %+v, want ready dod", dodContext)
	}
	dodItems := normalizeStringIDsFromAny(dodContext["items"])
	if len(dodItems) != 1 || dodItems[0] != "created dod" {
		t.Fatalf("execution dod items = %+v, want created dod", dodContext["items"])
	}
}

func normalizeStringIDsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func runCompressedContext(t *testing.T, item Run) map[string]any {
	t.Helper()
	compressed, ok := item.ExecutionContext["compressedContext"].(map[string]any)
	if !ok || len(compressed) == 0 {
		t.Fatalf("executionContext = %+v, want compressedContext map", item.ExecutionContext)
	}
	return compressed
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
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-read", "run read workspace", "", "workspaces/run read workspace"); err != nil {
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
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-a", "run workspace a", "", "workspaces/run workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-run-a: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-b", "run workspace b", "", "workspaces/run workspace b"); err != nil {
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

func TestStartRunWithBackendFailsWhenDefaultLLMProviderUnavailable(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run start workspace", "", "workspaces/run start workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start", "run-start", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	item, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start")
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if item.ID != "run-start" || item.State != "failed" || item.FailureReason == "" {
		t.Fatalf("run = %+v, want run-start/failed with provider failure reason", item)
	}
	compressed := runCompressedContext(t, item)
	if compressed["version"] != "workorch.compressedContext.v1" || compressed["mode"] != "deterministic_recovery_summary" {
		t.Fatalf("compressedContext = %+v, want deterministic recovery summary", compressed)
	}
	if compressed["runId"] != "run-start" || compressed["workspaceId"] != "ws-run-start" {
		t.Fatalf("compressedContext identity = %+v, want run-start/ws-run-start", compressed)
	}
}

func TestStartRunWithBackendReturnsFailedRunForFailedRunAfterReload(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run start restart workspace", "", "workspaces/run start restart workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-start-restart", "run-start-restart", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart"); err != nil {
		t.Fatalf("first StartRunWithBackend: %v", err)
	}

	if _, err := StartRunWithBackend(context.Background(), "file", dataRoot, "run-start-restart"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second StartRunWithBackend err = %v, want conflict for failed run", err)
	}
}

func TestStartRunWithBackendReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	item := prepareDecisionRun(t, dataRoot, "ws-run-start-blocked-decision", "run-start-blocked-decision")
	if item.State != "blocked_on_decision" || item.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", item)
	}
}

func TestResumeRunWithBackendReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-rejected"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-approval-rejected", runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}

	item, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}
	if item.State != "failed" || item.BlockingApprovalID != "" || item.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason and cleared approval", item)
	}
	compressed := runCompressedContext(t, item)
	if compressed["runId"] != runID || compressed["workspaceId"] != "ws-run-resume-approval-rejected" {
		t.Fatalf("compressedContext identity = %+v, want resumed approval run", compressed)
	}
	if _, ok := compressed["recentEvidence"].([]any); !ok {
		t.Fatalf("compressedContext = %+v, want recent evidence for resumed run", compressed)
	}
}

func TestResumeRunWithBackendReturnsConflictForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-rejected-restart"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-failed", runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("first ResumeRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second ResumeRunWithBackend err = %v, want conflict for failed run", err)
	}
}

func TestStartRunWithBackendReturnsConflictForFailedRunAfterReload(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-failed-start-conflict"
	started := prepareApprovalRun(t, dataRoot, "ws-run-start-failed", runID)
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
	runID := "run-approval-pending"
	started := prepareApprovalRun(t, dataRoot, "ws-run-resume-pending", runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("err = %v, want blocked", err)
	}
}

func TestResumeRunWithBackendReturnsConflictForCreatedRun(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run resume conflict workspace", "", "workspaces/run resume conflict workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, "ws-run-resume-conflict", "run-resume-conflict", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, "run-resume-conflict"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestRecoverStuckRunningRunsWithBackendFailsRunningRunWithoutContinuationCheckpoint(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-stuck-running", "stuck running workspace", "", "workspaces/stuck-running"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend stuck: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-continuation-running", "continuation running workspace", "", "workspaces/continuation-running"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend continuation: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-stuck-running", "run-stuck-running", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend stuck: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, "ws-continuation-running", "run-continuation-running", "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend continuation: %v", err)
	}

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	for _, runID := range []common.RunID{"run-stuck-running", "run-continuation-running"} {
		item, err := exports.Repositories.Runs.Get(ctx, runID)
		if err != nil {
			t.Fatalf("get run %s: %v", runID, err)
		}
		item.State = common.RunStateRunning
		if err := exports.Repositories.Runs.Update(ctx, item); err != nil {
			t.Fatalf("update run %s: %v", runID, err)
		}
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               "checkpoint-continuation-running",
		WorkspaceID:      "ws-continuation-running",
		RunID:            "run-continuation-running",
		CheckpointType:   common.CheckpointTypeRecovery,
		RunStateSnapshot: common.RunStateRunning,
		OccurredAt:       "2026-04-30T13:00:00Z",
		Data:             map[string]any{"continuationAction": "resume_from_checkpoint"},
	}); err != nil {
		t.Fatalf("create continuation checkpoint: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close seed session: %v", err)
	}

	recovered, err := RecoverStuckRunningRunsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("RecoverStuckRunningRunsWithBackend: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != "run-stuck-running" || recovered[0].State != "failed" {
		t.Fatalf("recovered = %+v, want only run-stuck-running failed", recovered)
	}
	if recovered[0].FailureReason != stuckRunningRunFailureReason {
		t.Fatalf("failure reason = %q, want %q", recovered[0].FailureReason, stuckRunningRunFailureReason)
	}
	continued, err := ReadRunWithBackend(ctx, "file", dataRoot, "run-continuation-running")
	if err != nil {
		t.Fatalf("ReadRunWithBackend continuation: %v", err)
	}
	if continued.State != "running" {
		t.Fatalf("continued run = %+v, want running", continued)
	}
}

func TestReadRunWithSessionReturnsFailureReason(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	if err := session.Bootstrap.Exports().Services.WorkspaceService.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:      "ws-run-failure",
		Title:            "run failure workspace",
		WorkingDirectory: "workspaces/ws-run-failure",
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
