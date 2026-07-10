package appservice

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/kernel/approval"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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

func TestListApprovalPageWithBackendProjectsAuditPayloads(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-page-a", "run-approval-page-a")
	second := prepareApprovalRun(t, dataRoot, "ws-approval-page-b", "run-approval-page-b")
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	for id, createdAt := range map[string]string{
		first.BlockingApprovalID:  "2026-05-20T00:00:00Z",
		second.BlockingApprovalID: "2026-05-20T00:01:00Z",
	} {
		item, err := exports.Repositories.Approvals.Get(ctx, common.ApprovalID(id))
		if err != nil {
			t.Fatalf("get approval %s: %v", id, err)
		}
		item.CreatedAt = createdAt
		item.UpdatedAt = createdAt
		if err := exports.Repositories.Approvals.Update(ctx, item); err != nil {
			t.Fatalf("update approval %s: %v", id, err)
		}
	}
	approval, err := exports.Repositories.Approvals.Get(ctx, common.ApprovalID(second.BlockingApprovalID))
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.Context = map[string]any{"large": "approval context"}
	if err := exports.Repositories.Approvals.Update(ctx, approval); err != nil {
		t.Fatalf("update approval context: %v", err)
	}

	page, err := ListApprovalPageWithBackend(context.Background(), "file", dataRoot, ApprovalQuery{
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListApprovalPageWithBackend: %v", err)
	}
	if len(page.Approvals) != 1 || page.Approvals[0].ID != second.BlockingApprovalID {
		t.Fatalf("page approvals = %+v, want newest second approval", page.Approvals)
	}
	wantCursor := second.WorkspaceID + "\x00" + second.BlockingApprovalID
	if !page.HasMore || page.NextCursorAfterID != wantCursor {
		t.Fatalf("page cursor = %+v, want hasMore with second cursor", page)
	}
	if page.Approvals[0].Context != nil {
		t.Fatalf("projected approval carries large context: %+v", page.Approvals[0])
	}

	next, err := ListApprovalPageWithBackend(context.Background(), "file", dataRoot, ApprovalQuery{
		CursorAfterID: page.NextCursorAfterID,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("ListApprovalPageWithBackend next: %v", err)
	}
	if len(next.Approvals) != 1 || next.Approvals[0].ID != first.BlockingApprovalID || next.HasMore {
		t.Fatalf("next page = %+v, want older first approval without more", next)
	}

	detail, err := ReadApprovalWithBackend(context.Background(), "file", dataRoot, second.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ReadApprovalWithBackend: %v", err)
	}
	if detail.Context == nil {
		t.Fatalf("detail approval lost full context: %+v", detail)
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

func TestApprovalOperationsHonorRuntimeWorkspaceScope(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-scope-a", "run-approval-scope")

	readSession, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new read session: %v", err)
	}
	wrongCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-approval-scope-b")
	if _, err := ReadApprovalWithSession(wrongCtx, readSession, started.BlockingApprovalID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadApprovalWithSession err = %v, want not found", err)
	}

	approveSession, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new approve session: %v", err)
	}
	if _, err := ApproveApprovalWithSession(wrongCtx, approveSession, started.BlockingApprovalID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ApproveApprovalWithSession err = %v, want not found", err)
	}

	item, err := ReadApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ReadApprovalWithBackend: %v", err)
	}
	if item.State != string(common.ApprovalStatePending) {
		t.Fatalf("approval state = %q, want pending after wrong-scope approve", item.State)
	}
}

func TestReadApprovalWithBackendIncludesAutoGovernanceSummary(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-approval-autogov"
	runID := "run-approval-autogov"
	started := prepareApprovalRun(t, dataRoot, workspaceID, runID)

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	ctx := context.Background()
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("session exports: %v", err)
	}
	if err := exports.Repositories.AutoGovernanceTraces.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID:                common.AutoGovernanceTraceID("trace-approval-autogov"),
		WorkspaceID:       common.WorkspaceID(workspaceID),
		RunID:             common.RunID(runID),
		ItemType:          "approval",
		ItemID:            started.BlockingApprovalID,
		Action:            "approve",
		RiskLevel:         "L1",
		Confidence:        0.95,
		PolicyMatched:     "read_only",
		Reason:            "action classified as read-only",
		EvidenceRefs:      []string{"step-1"},
		PromptFingerprint: "fingerprint-a",
		Status:            "resolved",
		OccurredAt:        "2026-05-06T10:10:00Z",
	}); err != nil {
		t.Fatalf("upsert auto governance trace: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	item, err := ReadApprovalWithBackend(ctx, "file", dataRoot, started.BlockingApprovalID)
	if err != nil {
		t.Fatalf("ReadApprovalWithBackend: %v", err)
	}
	if item.AutoGovernance == nil ||
		item.AutoGovernance.Action != "approve" ||
		item.AutoGovernance.Status != "resolved" ||
		item.AutoGovernance.PolicyMatched != "read_only" {
		t.Fatalf("auto governance summary = %+v, want resolved approval summary", item.AutoGovernance)
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

func TestResolveApprovalWithBackendPersistsGovernanceSemanticPayload(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareApprovalRun(t, dataRoot, "ws-approval-semantic", "run-approval-semantic")

	item, err := ResolveApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID, ResolveApprovalCommand{
		Action: "approve",
		Payload: map[string]any{
			"rawInput": "同意继续",
			"source":   "tui",
		},
	})
	if err != nil {
		t.Fatalf("ResolveApprovalWithBackend: %v", err)
	}
	if item.State != "approved" {
		t.Fatalf("approval = %+v, want approved", item)
	}
	semantic, ok := item.Context["governanceSemantic"].(map[string]any)
	if !ok || semantic["intent"] != "approve_execution" || semantic["source"] != "tui" {
		t.Fatalf("governanceSemantic = %#v, want approve from tui", item.Context["governanceSemantic"])
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
	session := newApprovalOnlySession(t, repo)
	before := repo.getCount.Load()

	item, err := ApproveApprovalWithSession(ctx, session, string(approvalID))
	if err != nil {
		t.Fatalf("ApproveApprovalWithSession: %v", err)
	}
	if item.ID != string(approvalID) || item.State != string(common.ApprovalStateApproved) {
		t.Fatalf("approval = %+v, want approved", item)
	}
	if got := repo.getCount.Load() - before; got != 2 {
		t.Fatalf("approval Get count during approve = %d, want transition and governance reads", got)
	}
}

func TestBatchResolveApprovalsWithSessionReusesReturnedApprovals(t *testing.T) {
	ctx := context.Background()
	repo := newAppserviceCountingApprovalRepository()
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
	session := newApprovalOnlySession(t, repo)
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
	if got := repo.getCount.Load() - before; got != 6 {
		t.Fatalf("approval Get count during batch approve = %d, want prevalidation, transition, and governance reads", got)
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

func TestBatchResolveApprovalsWithBackendNormalizesSemanticAction(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareApprovalRun(t, dataRoot, "ws-approval-batch-semantic", "run-approval-batch-semantic")

	items, err := BatchResolveApprovalsWithBackend(context.Background(), "file", dataRoot, BatchResolveApprovalCommand{
		ApprovalIDs: []string{first.BlockingApprovalID},
		Action:      "同意继续执行",
	})
	if err != nil {
		t.Fatalf("BatchResolveApprovalsWithBackend: %v", err)
	}
	if len(items) != 1 || items[0].ID != first.BlockingApprovalID || items[0].State != "approved" {
		t.Fatalf("items = %+v, want semantic approve resolution", items)
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

func newApprovalOnlySession(t *testing.T, repo *appserviceCountingApprovalRepository) *RuntimeSession {
	t.Helper()

	rawHub := magicevent.NewHub(16)
	eventHub := events.NewMagicHubWithHubOwner(rawHub, "appservice/approval_service_test")
	approvalModule := approvalmodule.New()
	if err := approvalModule.Configure(approvalmodule.Dependencies{Approvals: repo, EventHub: rawHub}); err != nil {
		t.Fatalf("setup approval module: %v", err)
	}
	if err := approvalModule.Start(); err != nil {
		t.Fatalf("run approval module: %v", err)
	}
	t.Cleanup(func() {
		if err := approvalModule.Stop(context.Background()); err != nil {
			t.Errorf("teardown approval module: %v", err)
		}
	})
	exports := bootstrap.Exports{
		Repositories: bootstrap.RepositoryExports{
			Workspaces:            memory.NewWorkspaceRepository(),
			Runs:                  memory.NewRunRepository(),
			Steps:                 memory.NewStepRepository(),
			Results:               memory.NewResultRepository(),
			Artifacts:             memory.NewArtifactRepository(),
			ArtifactContinuations: memory.NewArtifactContinuationRepository(),
			EventRecords:          memory.NewEventRecordRepository(),
			MemoryRecallTraces:    memory.NewMemoryRecallTraceRepository(),
			MCPCapabilities:       memory.NewMCPCapabilityRepository(),
			ToolCalls:             memory.NewToolCallRepository(),
			Patches:               memory.NewPatchRepository(),
			ValidationResults:     memory.NewValidationResultRepository(),
			RuntimeCheckpoints:    memory.NewRuntimeCheckpointRepository(),
			Tickets:               memory.NewDecisionTicketRepository(),
			Resolutions:           memory.NewDecisionResolutionRepository(),
			Approvals:             repo,
		},
		Infrastructure: bootstrap.InfrastructureExports{
			EventHub:   eventHub,
			CommandHub: rawHub,
		},
	}
	exports = completeAppserviceTestExports(exports)
	startFocusedGovernanceModule(t, exports, rawHub)
	session, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	return session
}
