package approval

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestService_CreateApproval_SeedsPendingState(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewApprovalRepository()
	service := NewService(repo)

	cmd := CreateApprovalCommand{
		ApprovalID:  common.ApprovalID("approval-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need human approval",
	}
	if err := service.CreateApproval(ctx, cmd); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	approval, err := repo.Get(ctx, cmd.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.State != common.ApprovalStatePending {
		t.Fatalf("approval state = %q, want pending", approval.State)
	}
}

func TestService_ResolveApproval_PendingTransitions(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		resolve   func(*Service, context.Context, ResolveApprovalCommand) error
		wantState common.ApprovalState
	}{
		{
			name:      "approve",
			resolve:   (*Service).Approve,
			wantState: common.ApprovalStateApproved,
		},
		{
			name:      "reject",
			resolve:   (*Service).Reject,
			wantState: common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := NewService(repo)

			cmd := CreateApprovalCommand{
				ApprovalID:  common.ApprovalID("approval-" + tc.name),
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}
			if err := service.CreateApproval(ctx, cmd); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			if err := tc.resolve(service, ctx, ResolveApprovalCommand{ApprovalID: cmd.ApprovalID}); err != nil {
				t.Fatalf("resolve approval: %v", err)
			}

			approval, err := repo.Get(ctx, cmd.ApprovalID)
			if err != nil {
				t.Fatalf("get approval: %v", err)
			}
			if approval.State != tc.wantState {
				t.Fatalf("approval state = %q, want %q", approval.State, tc.wantState)
			}
		})
	}
}

func TestService_ResolveApproval_RejectsWhenAlreadyResolved(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		firstResolve  func(*Service, context.Context, ResolveApprovalCommand) error
		secondResolve func(*Service, context.Context, ResolveApprovalCommand) error
		wantState     common.ApprovalState
	}{
		{
			name:          "approved cannot reject",
			firstResolve:  (*Service).Approve,
			secondResolve: (*Service).Reject,
			wantState:     common.ApprovalStateApproved,
		},
		{
			name:          "rejected cannot approve",
			firstResolve:  (*Service).Reject,
			secondResolve: (*Service).Approve,
			wantState:     common.ApprovalStateRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewApprovalRepository()
			service := NewService(repo)
			approvalID := common.ApprovalID("approval-1")

			if err := service.CreateApproval(ctx, CreateApprovalCommand{
				ApprovalID:  approvalID,
				WorkspaceID: common.WorkspaceID("ws-1"),
				RunID:       common.RunID("run-1"),
				StepID:      common.StepID("run-1/step-1"),
				Reason:      "need human approval",
			}); err != nil {
				t.Fatalf("create approval: %v", err)
			}

			if err := tc.firstResolve(service, ctx, ResolveApprovalCommand{ApprovalID: approvalID}); err != nil {
				t.Fatalf("first resolve: %v", err)
			}
			if err := tc.secondResolve(service, ctx, ResolveApprovalCommand{ApprovalID: approvalID}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("second resolve err = %v, want conflict", err)
			}

			approval, err := repo.Get(ctx, approvalID)
			if err != nil {
				t.Fatalf("get approval: %v", err)
			}
			if approval.State != tc.wantState {
				t.Fatalf("approval state = %q, want %q", approval.State, tc.wantState)
			}
		})
	}
}
