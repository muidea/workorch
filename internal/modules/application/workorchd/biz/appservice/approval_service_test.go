package appservice

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
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

func TestApproveApprovalWithSessionReusesReturnedApproval(t *testing.T) {
	ctx := context.Background()
	repo := newAppserviceCountingApprovalRepository()
	service := newReturningApprovalService(repo)
	approvalID := common.ApprovalID("approval-appservice-approve-loaded")
	if err := repo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID("ws-appservice-approval"),
		RunID:       common.RunID("run-appservice-approval"),
		StepID:      common.StepID("run-appservice-approval/approval"),
		Reason:      "need human approval",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	session := newApprovalOnlySession(t, service, repo)
	before := repo.getCount.Load()

	item, err := ApproveApprovalWithSession(ctx, session, string(approvalID))
	if err != nil {
		t.Fatalf("ApproveApprovalWithSession: %v", err)
	}
	if item.ID != string(approvalID) || item.State != string(common.ApprovalStateApproved) {
		t.Fatalf("approval = %+v, want approved", item)
	}
	if got := repo.getCount.Load() - before; got != 1 {
		t.Fatalf("approval Get count during approve = %d, want only service transition read", got)
	}
}

func TestBatchResolveApprovalsWithSessionReusesReturnedApprovals(t *testing.T) {
	ctx := context.Background()
	repo := newAppserviceCountingApprovalRepository()
	service := newReturningApprovalService(repo)
	firstID := common.ApprovalID("approval-appservice-batch-a")
	secondID := common.ApprovalID("approval-appservice-batch-b")
	for _, id := range []common.ApprovalID{firstID, secondID} {
		if err := repo.Create(ctx, &approvalmodel.Approval{
			ID:          id,
			WorkspaceID: common.WorkspaceID("ws-appservice-approval"),
			RunID:       common.RunID("run-appservice-approval"),
			StepID:      common.StepID("run-appservice-approval/approval"),
			Reason:      "need human approval",
			State:       common.ApprovalStatePending,
		}); err != nil {
			t.Fatalf("seed approval %s: %v", id, err)
		}
	}
	session := newApprovalOnlySession(t, service, repo)
	before := repo.getCount.Load()

	items, err := BatchResolveApprovalsWithSession(ctx, session, BatchResolveApprovalCommand{
		ApprovalIDs: []string{string(secondID), string(firstID)},
		Action:      "approve",
	})
	if err != nil {
		t.Fatalf("BatchResolveApprovalsWithSession: %v", err)
	}
	if len(items) != 2 || items[0].ID != string(secondID) || items[1].ID != string(firstID) {
		t.Fatalf("items = %+v, want request order", items)
	}
	if got := repo.getCount.Load() - before; got != 4 {
		t.Fatalf("approval Get count during batch approve = %d, want two prevalidation reads plus two service transition reads", got)
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

type appserviceCountingApprovalRepository struct {
	inner    *memory.ApprovalRepository
	getCount atomic.Int64
}

func newAppserviceCountingApprovalRepository() *appserviceCountingApprovalRepository {
	return &appserviceCountingApprovalRepository{inner: memory.NewApprovalRepository()}
}

func (r *appserviceCountingApprovalRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	return r.inner.Create(ctx, item)
}

func (r *appserviceCountingApprovalRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *appserviceCountingApprovalRepository) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	return r.inner.List(ctx)
}

func (r *appserviceCountingApprovalRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	return r.inner.ListByRunID(ctx, runID)
}

func (r *appserviceCountingApprovalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *appserviceCountingApprovalRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	return r.inner.Update(ctx, item)
}

func (r *appserviceCountingApprovalRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	return r.inner.Delete(ctx, id)
}

func newReturningApprovalService(repo *appserviceCountingApprovalRepository) *approvalservice.Module {
	resolve := func(ctx context.Context, cmd approvalservice.ResolveApprovalCommand, state common.ApprovalState) (*approvalmodel.Approval, error) {
		if cmd.ApprovalID == "" {
			return nil, common.ErrInvalidArgument
		}
		item, err := repo.Get(ctx, cmd.ApprovalID)
		if err != nil {
			return nil, err
		}
		if item.State == state {
			return item, nil
		}
		if item.State != common.ApprovalStatePending {
			return nil, common.ErrConflict
		}
		item.State = state
		if err := repo.Update(ctx, item); err != nil {
			return nil, err
		}
		return item, nil
	}
	return approvalservice.New(approvalservice.Dependencies{
		EnsureApproved: func(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
			_, err := resolve(ctx, cmd, common.ApprovalStateApproved)
			return err
		},
		EnsureApprovedAndReturn: func(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) (*approvalmodel.Approval, error) {
			return resolve(ctx, cmd, common.ApprovalStateApproved)
		},
		EnsureRejected: func(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) error {
			_, err := resolve(ctx, cmd, common.ApprovalStateRejected)
			return err
		},
		EnsureRejectedAndReturn: func(ctx context.Context, cmd approvalservice.ResolveApprovalCommand) (*approvalmodel.Approval, error) {
			return resolve(ctx, cmd, common.ApprovalStateRejected)
		},
	})
}

func newApprovalOnlySession(t *testing.T, service *approvalservice.Module, repo *appserviceCountingApprovalRepository) *RuntimeSession {
	t.Helper()

	bootstrapRuntime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := bootstrapRuntime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown bootstrap runtime: %v", err)
		}
	})
	exports := bootstrapRuntime.Exports()
	exports.Services.ApprovalService = service
	exports.Repositories.Approvals = repo
	session, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	return session
}
