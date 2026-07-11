package governancecore

import (
	"errors"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEnsureApprovalWorkspaceScope(t *testing.T) {
	item := &approvalmodel.Approval{WorkspaceID: "ws-001"}
	if err := EnsureApprovalWorkspaceScope("", item); err != nil {
		t.Fatalf("unscoped approval err = %v", err)
	}
	if err := EnsureApprovalWorkspaceScope("ws-001", item); err != nil {
		t.Fatalf("matching approval err = %v", err)
	}
	if err := EnsureApprovalWorkspaceScope("ws-other", item); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong-scope approval err = %v, want not found", err)
	}
	if err := EnsureApprovalWorkspaceScope("ws-001", nil); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("nil approval err = %v, want not found", err)
	}
}

func TestEnsureDecisionTicketWorkspaceScope(t *testing.T) {
	item := &decisionmodel.Ticket{WorkspaceID: "ws-001"}
	if err := EnsureDecisionTicketWorkspaceScope("", item); err != nil {
		t.Fatalf("unscoped decision err = %v", err)
	}
	if err := EnsureDecisionTicketWorkspaceScope("ws-001", item); err != nil {
		t.Fatalf("matching decision err = %v", err)
	}
	if err := EnsureDecisionTicketWorkspaceScope("ws-other", item); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong-scope decision err = %v, want not found", err)
	}
	if err := EnsureDecisionTicketWorkspaceScope("ws-001", nil); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("nil decision err = %v, want not found", err)
	}
}
