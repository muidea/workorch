package approval

import (
	"context"
	"errors"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestResolveUniqueApprovalForUnscopedCommandConflictsOnMixedStateDuplicates(t *testing.T) {
	ctx := context.Background()
	repository := memory.NewApprovalRepository()
	approvalID := common.ApprovalID("approval-mixed-state-duplicate")
	for _, item := range []*approvalmodel.Approval{
		{
			ID: approvalID, WorkspaceID: "ws-approval-mixed-a", RunID: "run-approval-mixed-a",
			State: common.ApprovalStatePending, Reason: "pending approval",
		},
		{
			ID: approvalID, WorkspaceID: "ws-approval-mixed-b", RunID: "run-approval-mixed-b",
			State: common.ApprovalStateApproved, Reason: "resolved approval",
		},
	} {
		if err := repository.Create(ctx, item); err != nil {
			t.Fatalf("create approval %s: %v", item.WorkspaceID, err)
		}
	}

	owner := &Biz{approvals: repository}
	if _, err := owner.resolveUniqueApprovalForUnscopedCommand(ctx, approvalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mixed-state duplicate discovery err = %v, want conflict", err)
	}
}
