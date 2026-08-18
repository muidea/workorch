package governance

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

var errGovernanceTraceStoreUnavailable = errors.New("governance trace store unavailable")

type faultingAutoGovernanceTraceRepository struct {
	inner        repository.AutoGovernanceTraceRepository
	failListAt   int32
	failUpsertAt int32
	listCalls    atomic.Int32
	upsertCalls  atomic.Int32
}

func (r *faultingAutoGovernanceTraceRepository) Create(ctx context.Context, item *governancemodel.AutoGovernanceTrace) error {
	return r.inner.Create(ctx, item)
}

func (r *faultingAutoGovernanceTraceRepository) Upsert(ctx context.Context, item *governancemodel.AutoGovernanceTrace) error {
	call := r.upsertCalls.Add(1)
	if r.failUpsertAt > 0 && call == r.failUpsertAt {
		return errGovernanceTraceStoreUnavailable
	}
	return r.inner.Upsert(ctx, item)
}

func (r *faultingAutoGovernanceTraceRepository) Get(ctx context.Context, id common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error) {
	return r.inner.Get(ctx, id)
}

func (r *faultingAutoGovernanceTraceRepository) List(ctx context.Context, query workorchquerycontract.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	call := r.listCalls.Add(1)
	if r.failListAt > 0 && call == r.failListAt {
		return nil, errGovernanceTraceStoreUnavailable
	}
	return r.inner.List(ctx, query)
}

func (r *faultingAutoGovernanceTraceRepository) Delete(ctx context.Context, id common.AutoGovernanceTraceID) error {
	return r.inner.Delete(ctx, id)
}

type governanceApprovalReliabilityFixture struct {
	service      *Service
	context      context.Context
	approvalRepo repository.ApprovalRepository
	traceRepo    *faultingAutoGovernanceTraceRepository
	workspaceID  common.WorkspaceID
	runID        common.RunID
	approvalID   common.ApprovalID
	resolveCount atomic.Int32
}

func newGovernanceApprovalReliabilityFixture(t *testing.T, traceRepo *faultingAutoGovernanceTraceRepository) *governanceApprovalReliabilityFixture {
	t.Helper()
	baseCtx := context.Background()
	workspaceID := common.WorkspaceID("ws-governance-reliability")
	runID := common.RunID("run-governance-reliability")
	goalID := common.GoalID("goal-governance-reliability")
	dodID := common.DoDID("dod-governance-reliability")
	approvalID := common.ApprovalID("approval-governance-reliability")
	ctx, err := common.BindRuntimeWorkspaceContext(baseCtx, workspaceID)
	if err != nil {
		t.Fatalf("bind workspace context: %v", err)
	}
	approvalRepo := memory.NewApprovalRepository()
	runRepo := memory.NewRunRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	mustNoErr(t, goalRepo.Create(ctx, &workspacemodel.Goal{
		ID: goalID, WorkspaceID: workspaceID, Text: "inspect workspace configuration",
	}))
	mustNoErr(t, dodRepo.Create(ctx, &workspacemodel.DoD{
		ID: dodID, WorkspaceID: workspaceID, Items: []string{"return the inspected configuration"}, State: common.DoDStateReady,
	}))
	mustNoErr(t, runRepo.Create(ctx, &runmodel.Run{
		ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID,
		State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID,
	}))
	mustNoErr(t, approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
		StepID: "run-governance-reliability/step-1", Reason: "read configuration",
		Action: "read configuration and list files", State: common.ApprovalStatePending,
		Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "read_only")),
	}))
	fixture := &governanceApprovalReliabilityFixture{
		context: ctx, approvalRepo: approvalRepo, traceRepo: traceRepo,
		workspaceID: workspaceID, runID: runID, approvalID: approvalID,
	}
	fixture.service = NewService(servicePorts{
		Tickets:              memory.NewDecisionTicketRepository(),
		Approvals:            approvalRepo,
		Runs:                 runRepo,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: goalRepo, dods: dodRepo},
		MemoryItems:          memory.NewMemoryItemRepository(),
		AutoGovernanceTraces: traceRepo,
		ResolveDecision:      func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error { return nil },
		ResolveApproval: func(resolveCtx context.Context, id common.ApprovalID) error {
			fixture.resolveCount.Add(1)
			item, err := approvalRepo.Get(resolveCtx, id)
			if err != nil {
				return err
			}
			item.State = common.ApprovalStateApproved
			item.ResolvedBy = autoGovernanceActor
			return approvalRepo.Update(resolveCtx, item)
		},
		LoadConfig: testGovernanceConfig(true, 0.82, 3),
	})
	return fixture
}

func TestGovernanceTraceQueryFailuresFailClosedBeforeOwnerMutation(t *testing.T) {
	for _, failListAt := range []int32{1, 2, 3} {
		t.Run(fmt.Sprintf("list-%d", failListAt), func(t *testing.T) {
			traces := &faultingAutoGovernanceTraceRepository{
				inner: memory.NewAutoGovernanceTraceRepository(), failListAt: failListAt,
			}
			fixture := newGovernanceApprovalReliabilityFixture(t, traces)
			err := fixture.service.processPendingApproval(fixture.context, fixture.approvalID)
			if !errors.Is(err, errGovernanceTraceStoreUnavailable) {
				t.Fatalf("process approval err = %v, want trace store error", err)
			}
			if got := fixture.resolveCount.Load(); got != 0 {
				t.Fatalf("resolve count = %d, want 0", got)
			}
			item, err := fixture.approvalRepo.Get(fixture.context, fixture.approvalID)
			if err != nil || item.State != common.ApprovalStatePending {
				t.Fatalf("approval after trace query failure = %+v, err=%v", item, err)
			}
		})
	}
}

func TestGovernanceTracePrecommitFailuresFailClosedBeforeOwnerMutation(t *testing.T) {
	for _, failUpsertAt := range []int32{1, 2} {
		t.Run(fmt.Sprintf("upsert-%d", failUpsertAt), func(t *testing.T) {
			traces := &faultingAutoGovernanceTraceRepository{
				inner: memory.NewAutoGovernanceTraceRepository(), failUpsertAt: failUpsertAt,
			}
			fixture := newGovernanceApprovalReliabilityFixture(t, traces)
			err := fixture.service.processPendingApproval(fixture.context, fixture.approvalID)
			if !errors.Is(err, errGovernanceTraceStoreUnavailable) {
				t.Fatalf("process approval err = %v, want trace store error", err)
			}
			if got := fixture.resolveCount.Load(); got != 0 {
				t.Fatalf("resolve count = %d, want 0", got)
			}
			item, err := fixture.approvalRepo.Get(fixture.context, fixture.approvalID)
			if err != nil || item.State != common.ApprovalStatePending {
				t.Fatalf("approval after trace precommit failure = %+v, err=%v", item, err)
			}
		})
	}
}

func TestGovernanceRecoversOwnerCommitAfterFinalTraceWriteFailure(t *testing.T) {
	inner := memory.NewAutoGovernanceTraceRepository()
	traces := &faultingAutoGovernanceTraceRepository{inner: inner, failUpsertAt: 3}
	fixture := newGovernanceApprovalReliabilityFixture(t, traces)

	err := fixture.service.processPendingApproval(fixture.context, fixture.approvalID)
	if !errors.Is(err, errGovernanceTraceStoreUnavailable) {
		t.Fatalf("process approval err = %v, want final trace write error", err)
	}
	if got := fixture.resolveCount.Load(); got != 1 {
		t.Fatalf("resolve count = %d, want 1", got)
	}
	traceID := autoGovernanceTraceID(fixture.workspaceID, "approval", string(fixture.approvalID))
	trace, err := inner.Get(fixture.context, traceID)
	if err != nil || trace.Status != autoGovernanceStatusResolving {
		t.Fatalf("trace before recovery = %+v, err=%v", trace, err)
	}

	if err := fixture.service.reconcileIncompleteTraces(context.Background()); err != nil {
		t.Fatalf("reconcile incomplete traces: %v", err)
	}
	trace, err = inner.Get(fixture.context, traceID)
	if err != nil || trace.Status != autoGovernanceStatusResolved || trace.Action != "approve" {
		t.Fatalf("trace after recovery = %+v, err=%v", trace, err)
	}
	if got := fixture.resolveCount.Load(); got != 1 {
		t.Fatalf("resolve count after recovery = %d, want no duplicate owner mutation", got)
	}
}

func TestGovernanceTraceIdentityAndBudgetAreWorkspaceScoped(t *testing.T) {
	ctx := context.Background()
	traces := memory.NewAutoGovernanceTraceRepository()
	service := NewService(servicePorts{AutoGovernanceTraces: traces})
	workspaceA := common.WorkspaceID("ws-governance-a")
	workspaceB := common.WorkspaceID("ws-governance-b")
	runID := common.RunID("run-workspace-local")
	itemID := "shared-item-id"
	ticketTraceID := autoGovernanceTraceID(workspaceA, "ticket", itemID)
	approvalTraceID := autoGovernanceTraceID(workspaceA, "approval", itemID)
	otherWorkspaceTraceID := autoGovernanceTraceID(workspaceB, "ticket", itemID)
	if ticketTraceID == approvalTraceID || ticketTraceID == otherWorkspaceTraceID || approvalTraceID == otherWorkspaceTraceID {
		t.Fatalf("scoped trace IDs collided: ticket=%s approval=%s other=%s", ticketTraceID, approvalTraceID, otherWorkspaceTraceID)
	}
	mustNoErr(t, traces.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID: ticketTraceID, WorkspaceID: workspaceA, RunID: runID,
		ItemType: "ticket", ItemID: itemID, Action: "answer",
		Status: autoGovernanceStatusResolved, OccurredAt: nowUTC(),
	}))

	countA, err := service.countResolvedTicketTraces(ctx, workspaceA, runID)
	if err != nil {
		t.Fatalf("count workspace A: %v", err)
	}
	countB, err := service.countResolvedTicketTraces(ctx, workspaceB, runID)
	if err != nil {
		t.Fatalf("count workspace B: %v", err)
	}
	if countA != 1 || countB != 0 {
		t.Fatalf("workspace budget counts = %d/%d, want 1/0", countA, countB)
	}
	closedA, err := service.hasClosedTrace(ctx, workspaceA, "ticket", itemID)
	if err != nil {
		t.Fatalf("closed workspace A: %v", err)
	}
	closedB, err := service.hasClosedTrace(ctx, workspaceB, "ticket", itemID)
	if err != nil {
		t.Fatalf("closed workspace B: %v", err)
	}
	if !closedA || closedB {
		t.Fatalf("workspace closed state = %t/%t, want true/false", closedA, closedB)
	}
}
