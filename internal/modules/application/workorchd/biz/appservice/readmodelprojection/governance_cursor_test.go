package readmodelprojection

import (
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSelectDecisionPageWindowUsesWorkspaceScopedCursor(t *testing.T) {
	items := []*decisionmodel.Ticket{
		{ID: "ticket-shared", WorkspaceID: "ws-a"},
		{ID: "ticket-shared", WorkspaceID: "ws-b"},
		{ID: "ticket-next", WorkspaceID: "ws-b"},
	}
	first, cursor, hasMore := selectDecisionPageWindow(items, 1, "")
	if len(first) != 1 || first[0].WorkspaceID != "ws-a" || !hasMore {
		t.Fatalf("first page = %+v hasMore=%v, want ws-a shared with more", first, hasMore)
	}
	if cursor != governanceScopedCursor("ws-a", "ticket-shared") {
		t.Fatalf("cursor = %q, want scoped ws-a ticket", cursor)
	}

	second, cursor, hasMore := selectDecisionPageWindow(items, 1, cursor)
	if len(second) != 1 || second[0].WorkspaceID != "ws-b" || second[0].ID != "ticket-shared" || !hasMore {
		t.Fatalf("second page = %+v hasMore=%v, want ws-b shared with more", second, hasMore)
	}
	if cursor != governanceScopedCursor("ws-b", "ticket-shared") {
		t.Fatalf("second cursor = %q, want scoped ws-b ticket", cursor)
	}

	third, _, hasMore := selectDecisionPageWindow(items, 1, cursor)
	if len(third) != 1 || third[0].ID != "ticket-next" || hasMore {
		t.Fatalf("third page = %+v hasMore=%v, want next item and no more", third, hasMore)
	}
}

func TestSelectApprovalPageWindowUsesWorkspaceScopedCursor(t *testing.T) {
	items := []*approvalmodel.Approval{
		{ID: "approval-shared", WorkspaceID: common.WorkspaceID("ws-a")},
		{ID: "approval-shared", WorkspaceID: common.WorkspaceID("ws-b")},
		{ID: "approval-next", WorkspaceID: common.WorkspaceID("ws-b")},
	}
	first, cursor, hasMore := selectApprovalPageWindow(items, 1, "")
	if len(first) != 1 || first[0].WorkspaceID != "ws-a" || !hasMore {
		t.Fatalf("first page = %+v hasMore=%v, want ws-a shared with more", first, hasMore)
	}
	if cursor != governanceScopedCursor("ws-a", "approval-shared") {
		t.Fatalf("cursor = %q, want scoped ws-a approval", cursor)
	}

	second, cursor, hasMore := selectApprovalPageWindow(items, 1, cursor)
	if len(second) != 1 || second[0].WorkspaceID != "ws-b" || second[0].ID != "approval-shared" || !hasMore {
		t.Fatalf("second page = %+v hasMore=%v, want ws-b shared with more", second, hasMore)
	}
	if cursor != governanceScopedCursor("ws-b", "approval-shared") {
		t.Fatalf("second cursor = %q, want scoped ws-b approval", cursor)
	}

	third, _, hasMore := selectApprovalPageWindow(items, 1, cursor)
	if len(third) != 1 || third[0].ID != "approval-next" || hasMore {
		t.Fatalf("third page = %+v hasMore=%v, want next item and no more", third, hasMore)
	}
}
