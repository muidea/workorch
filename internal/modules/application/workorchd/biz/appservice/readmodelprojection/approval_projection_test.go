package readmodelprojection

import (
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMapApprovalProjectsResolutionFields(t *testing.T) {
	item := &approvalmodel.Approval{
		ID:          "approval-resolved",
		WorkspaceID: "ws-approval-resolved",
		RunID:       "run-approval-resolved",
		StepID:      "step-approval-resolved",
		Reason:      "approval resolved",
		Action:      "approve",
		State:       common.ApprovalStateApproved,
		ResolvedAt:  "2026-07-06T02:03:04Z",
		ResolvedBy:  "operator",
		CreatedAt:   "2026-07-06T02:00:00Z",
		UpdatedAt:   "2026-07-06T02:03:04Z",
		Context:     mustProjectionPayload(t, map[string]any{"decision": "approved"}),
	}

	got := MapApproval(item, nil)
	if got.ResolvedAt != item.ResolvedAt || got.ResolvedBy != item.ResolvedBy {
		t.Fatalf("approval resolution fields = %+v, want %s/%s", got, item.ResolvedAt, item.ResolvedBy)
	}
	view := ProjectApprovalView(got)
	if len(view.Context.Fields) != 0 {
		t.Fatalf("approval view context = %+v, want compact view without context", view.Context)
	}
	if view.ResolvedAt != got.ResolvedAt || view.ResolvedBy != got.ResolvedBy {
		t.Fatalf("approval view lost resolution fields: %+v", view)
	}
}
