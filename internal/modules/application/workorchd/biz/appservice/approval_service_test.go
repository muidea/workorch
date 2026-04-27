package appservice

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadApprovalWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadApprovalWithSession(context.Background(), nil, "approval-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestListApprovalsWithBackendReturnsFilteredApprovals(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-list-a", "run-approval-list-a")
	second := prepareApprovalRun(t, dataRoot, "ws-approval-list-b", "run-approval-list-b")

	items, err := ListApprovalsWithBackend(context.Background(), "file", dataRoot, "", "")
	if err != nil {
		t.Fatalf("ListApprovalsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != first.BlockingApprovalID || items[1].ID != second.BlockingApprovalID {
		t.Fatalf("approval ids = %q/%q, want %q/%q", items[0].ID, items[1].ID, first.BlockingApprovalID, second.BlockingApprovalID)
	}

	filteredByRun, err := ListApprovalsWithBackend(context.Background(), "file", dataRoot, "", "run-approval-list-b")
	if err != nil {
		t.Fatalf("ListApprovalsWithBackend filtered by run: %v", err)
	}
	if len(filteredByRun) != 1 || filteredByRun[0].ID != second.BlockingApprovalID {
		t.Fatalf("filteredByRun = %+v, want approval %s", filteredByRun, second.BlockingApprovalID)
	}

	filteredByWorkspace, err := ListApprovalsWithBackend(context.Background(), "file", dataRoot, "ws-approval-list-a", "")
	if err != nil {
		t.Fatalf("ListApprovalsWithBackend filtered by workspace: %v", err)
	}
	if len(filteredByWorkspace) != 1 || filteredByWorkspace[0].ID != first.BlockingApprovalID {
		t.Fatalf("filteredByWorkspace = %+v, want approval %s", filteredByWorkspace, first.BlockingApprovalID)
	}
}

func TestReadApprovalWithBackendReturnsRejectedApproval(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-read", "run-approval-read")
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}

	item, err := ReadApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ReadApprovalWithBackend: %v", err)
	}
	if item.ID != started.BlockingApprovalID || item.State != "rejected" {
		t.Fatalf("approval = %+v, want rejected approval", item)
	}
}

func TestApproveApprovalWithBackendReturnsApprovedApproval(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-approve", "run-approval-approve")

	item, err := ApproveApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ApproveApprovalWithBackend: %v", err)
	}
	if item.State != "approved" {
		t.Fatalf("approval = %+v, want approved", item)
	}
}

func TestRejectApprovalWithBackendReturnsRejectedApproval(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-reject", "run-approval-reject")

	item, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID)
	if err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if item.State != "rejected" {
		t.Fatalf("approval = %+v, want rejected", item)
	}
}

func TestRejectApprovalWithBackendReturnsConflictAfterApproved(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-conflict", "run-approval-conflict")
	if _, err := ApproveApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("ApproveApprovalWithBackend: %v", err)
	}

	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestBatchResolveApprovalsWithBackendApprovesInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-batch-a", "run-approval-batch-a")
	second := prepareApprovalRun(t, dataRoot, "ws-approval-batch-b", "run-approval-batch-b")

	items, err := BatchResolveApprovalsWithBackend(context.Background(), "file", dataRoot, BatchResolveApprovalCommand{
		ApprovalIDs: []string{second.BlockingApprovalID, first.BlockingApprovalID},
		Action:      "approve",
	})
	if err != nil {
		t.Fatalf("BatchResolveApprovalsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.BlockingApprovalID || items[1].ID != first.BlockingApprovalID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "approved" {
			t.Fatalf("item = %+v, want approved", item)
		}
	}
}

func TestBatchResolveApprovalsWithBackendRejectsInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-batch-reject-a", "run-approval-batch-reject-a")
	second := prepareApprovalRun(t, dataRoot, "ws-approval-batch-reject-b", "run-approval-batch-reject-b")

	items, err := BatchResolveApprovalsWithBackend(context.Background(), "file", dataRoot, BatchResolveApprovalCommand{
		ApprovalIDs: []string{first.BlockingApprovalID, second.BlockingApprovalID},
		Action:      "reject",
	})
	if err != nil {
		t.Fatalf("BatchResolveApprovalsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.BlockingApprovalID || items[1].ID != second.BlockingApprovalID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "rejected" {
			t.Fatalf("item = %+v, want rejected", item)
		}
	}
}

func TestBatchResolveApprovalsWithBackendRejectsDuplicateApprovalIDs(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-batch-dup", "run-approval-batch-dup")

	if _, err := BatchResolveApprovalsWithBackend(context.Background(), "file", dataRoot, BatchResolveApprovalCommand{
		ApprovalIDs: []string{started.BlockingApprovalID, started.BlockingApprovalID},
		Action:      "approve",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveApprovalsWithBackendRejectsAlreadyResolvedBeforeMutation(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-batch-conflict-a", "run-approval-batch-conflict-a")
	second := prepareApprovalRun(t, dataRoot, "ws-approval-batch-conflict-b", "run-approval-batch-conflict-b")
	if _, err := ApproveApprovalWithBackend(context.Background(), "file", dataRoot, second.BlockingApprovalID); err != nil {
		t.Fatalf("ApproveApprovalWithBackend: %v", err)
	}

	if _, err := BatchResolveApprovalsWithBackend(context.Background(), "file", dataRoot, BatchResolveApprovalCommand{
		ApprovalIDs: []string{first.BlockingApprovalID, second.BlockingApprovalID},
		Action:      "reject",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
	firstApproval, err := ReadApprovalWithBackend(context.Background(), "file", dataRoot, first.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ReadApprovalWithBackend: %v", err)
	}
	if firstApproval.State != "pending" {
		t.Fatalf("first approval = %+v, want still pending after prevalidation conflict", firstApproval)
	}
}
