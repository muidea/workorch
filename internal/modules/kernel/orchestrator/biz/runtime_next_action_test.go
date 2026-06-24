package orchestrator

import (
	"context"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNextActionForRunPendingApprovalRequestsApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-approval")
	runID := common.RunID("run-next-action-approval")
	approvalID := common.ApprovalID("approval-next-action")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Action:      "review pending runtime action",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRequestApproval || action.Stage != "approval" || action.StageMode != "governance" {
		t.Fatalf("next action = %+v, want request approval", action)
	}
	if action.SubjectType != "approval" || action.SubjectID != string(approvalID) {
		t.Fatalf("next action subject = %s/%s, want approval %s", action.SubjectType, action.SubjectID, approvalID)
	}
	if action.RequiredOutput["approvalId"] != string(approvalID) {
		t.Fatalf("required output = %+v, want approval id", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden", action.ForbiddenActions)
	}
}

func TestDerivePendingApprovalNextActionIgnoresResolvedOrUnreferencedApproval(t *testing.T) {
	run := &runmodel.Run{
		ID:                 "run-pending-approval-rule",
		WorkspaceID:        "ws-pending-approval-rule",
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-blocking",
	}
	approved := &approvalmodel.Approval{
		ID:          "approval-blocking",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.ApprovalStateApproved,
	}
	if action, ok := derivePendingApprovalNextAction(run, approved); ok || action.Type != "" {
		t.Fatalf("derive with approved approval = %+v/%v, want no request_approval", action, ok)
	}

	unreferencedPending := &approvalmodel.Approval{
		ID:          "approval-other",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.ApprovalStatePending,
	}
	if action, ok := derivePendingApprovalNextAction(run, unreferencedPending); ok || action.Type != "" {
		t.Fatalf("derive with unreferenced approval = %+v/%v, want no request_approval", action, ok)
	}
}

func mustCreateWorkspaceForNextAction(t *testing.T, ctx context.Context, repo *memory.WorkspaceRepository, workspaceID common.WorkspaceID, runID common.RunID) {
	t.Helper()
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
}
