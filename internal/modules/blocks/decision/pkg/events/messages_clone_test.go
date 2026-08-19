package events

import (
	"errors"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeListWindowIsBounded(t *testing.T) {
	cursor, limit, err := NormalizeListWindow(" cursor ", 0)
	if err != nil || cursor != "cursor" || limit != DefaultListLimit {
		t.Fatalf("default window = (%q, %d, %v)", cursor, limit, err)
	}
	if _, _, err := NormalizeListWindow("", MaxListLimit+1); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized list err = %v, want invalid argument", err)
	}
}

func TestTicketCursorBindsWorkspaceRunAndTicketIdentity(t *testing.T) {
	left := TicketCursor(&decisionmodel.Ticket{ID: "ticket", WorkspaceID: "ws-a", RunID: "run"})
	right := TicketCursor(&decisionmodel.Ticket{ID: "ticket", WorkspaceID: "ws-b", RunID: "run"})
	if left == "" || right == "" || left == right {
		t.Fatalf("composite cursors must differ across workspaces: %q %q", left, right)
	}
}
