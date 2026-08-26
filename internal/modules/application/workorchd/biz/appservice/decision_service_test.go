package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type countingAppDecisionTicketRepository struct {
	base     *memory.DecisionTicketRepository
	getCount atomic.Int64
}

func newCountingAppDecisionTicketRepository() *countingAppDecisionTicketRepository {
	return &countingAppDecisionTicketRepository{base: memory.NewDecisionTicketRepository()}
}

func (r *countingAppDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Create(ctx, item)
}

func (r *countingAppDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	r.getCount.Add(1)
	return r.base.Get(ctx, id)
}

func (r *countingAppDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	return r.base.List(ctx)
}

func (r *countingAppDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return r.base.ListByRunID(ctx, runID)
}

func (r *countingAppDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingAppDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Update(ctx, item)
}

func (r *countingAppDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	return r.base.Delete(ctx, id)
}

type countingAppDecisionResolutionRepository struct {
	base               *memory.DecisionResolutionRepository
	getByTicketIDCount atomic.Int64
}

func newCountingAppDecisionResolutionRepository() *countingAppDecisionResolutionRepository {
	return &countingAppDecisionResolutionRepository{base: memory.NewDecisionResolutionRepository()}
}

func (r *countingAppDecisionResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	return r.base.Create(ctx, item)
}

func (r *countingAppDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	r.getByTicketIDCount.Add(1)
	return r.base.GetByTicketID(ctx, ticketID)
}

func (r *countingAppDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	return r.base.DeleteByTicketID(ctx, ticketID)
}

func newCountingDecisionRuntimeSession(t *testing.T) (*runtimecore.Session, *countingAppDecisionTicketRepository, *countingAppDecisionResolutionRepository) {
	t.Helper()

	tickets := newCountingAppDecisionTicketRepository()
	resolutions := newCountingAppDecisionResolutionRepository()
	rawHub := magicevent.NewHub(16)
	subscribeGovernanceEvidenceAcks(t, rawHub)
	subscribeTestCanonicalGovernanceOwnerReads(t, rawHub, func(ctx context.Context, workspaceID common.WorkspaceID) (*runmodel.Run, error) {
		items, err := tickets.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item != nil && item.State == common.DecisionTicketStateOpen {
				return &runmodel.Run{
					ID: item.RunID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: item.ID,
				}, nil
			}
		}
		return nil, common.ErrNotFound
	})
	decisionModule := decisionmodule.New()
	if err := decisionModule.Configure(decisionbiz.Dependencies{OwnerRepositories: appserviceTestDecisionOwnerRepositories(tickets, resolutions, memory.NewDecisionWorkspaceDeletionRepository()), EventHub: rawHub}); err != nil {
		t.Fatalf("setup decision module: %v", err)
	}
	if err := decisionModule.Start(); err != nil {
		t.Fatalf("run decision module: %v", err)
	}
	t.Cleanup(func() {
		if err := decisionModule.Stop(context.Background()); err != nil {
			t.Errorf("teardown decision module: %v", err)
		}
	})
	approvalModule := approvalmodule.New()
	if err := approvalModule.Configure(approvalbiz.Dependencies{OwnerRepositories: appserviceTestApprovalOwnerRepositories(memory.NewApprovalRepository(), memory.NewApprovalWorkspaceDeletionRepository()), EventHub: rawHub}); err != nil {
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

	exports := completeAppserviceTestExports(bootstrap.Exports{
		Infrastructure: bootstrap.InfrastructureExports{
			CommandHub: rawHub,
		},
	})
	startFocusedGovernanceModule(t, exports, rawHub)
	session, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	return session, tickets, resolutions
}

func TestReadDecisionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadDecisionWithSession(context.Background(), nil, "ticket-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestListDecisionsWithBackendReturnsFilteredTickets(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-list-a", "run-decision-list-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-list-b", "run-decision-list-b")

	items, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != first.BlockingTicketID || items[1].ID != second.BlockingTicketID {
		t.Fatalf("ticket ids = %q/%q, want %q/%q", items[0].ID, items[1].ID, first.BlockingTicketID, second.BlockingTicketID)
	}

	filteredByRun, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, second.WorkspaceID, "run-decision-list-b")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by run: %v", err)
	}
	if len(filteredByRun) != 1 || filteredByRun[0].ID != second.BlockingTicketID {
		t.Fatalf("filteredByRun = %+v, want ticket %s", filteredByRun, second.BlockingTicketID)
	}

	filteredByWorkspace, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "ws-decision-list-a", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by workspace: %v", err)
	}
	if len(filteredByWorkspace) != 1 || filteredByWorkspace[0].ID != first.BlockingTicketID {
		t.Fatalf("filteredByWorkspace = %+v, want ticket %s", filteredByWorkspace, first.BlockingTicketID)
	}
}

func TestListDecisionPageWithBackendProjectsAuditPayloads(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-page-a", "run-decision-page-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-page-b", "run-decision-page-b")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"answer": "first decision payload"}),
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	for id, createdAt := range map[string]string{
		first.BlockingTicketID:  "2026-05-20T00:00:00Z",
		second.BlockingTicketID: "2026-05-20T00:01:00Z",
	} {
		ticket, err := exports.Repositories.Tickets.Get(ctx, common.TicketID(id))
		if err != nil {
			t.Fatalf("get ticket %s: %v", id, err)
		}
		ticket.CreatedAt = createdAt
		ticket.UpdatedAt = createdAt
		if err := exports.Repositories.Tickets.Update(ctx, ticket); err != nil {
			t.Fatalf("update ticket %s: %v", id, err)
		}
	}

	page, err := ListDecisionPageWithBackend(context.Background(), "file", dataRoot, workorchquerycontract.RunResourcePageQuery{
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListDecisionPageWithBackend: %v", err)
	}
	if len(page.Decisions) != 1 || page.Decisions[0].ID != first.BlockingTicketID || page.Decisions[0].State != string(common.DecisionTicketStateResolved) {
		t.Fatalf("page decisions = %+v, want first composite-key ticket", page.Decisions)
	}
	wantCursor := first.WorkspaceID + "\x00" + first.ID + "\x00" + first.BlockingTicketID
	if !page.HasMore || page.NextCursorAfterID != wantCursor {
		t.Fatalf("page cursor = %+v, want hasMore with second cursor", page)
	}
	if len(page.Decisions[0].Context.Fields) != 0 || len(page.Decisions[0].ResolutionPayload.Fields) == 0 {
		t.Fatalf("projected decision must hide raw context and retain safe resolution audit payload: %+v", page.Decisions[0])
	}

	next, err := ListDecisionPageWithBackend(context.Background(), "file", dataRoot, workorchquerycontract.RunResourcePageQuery{
		CursorAfterID: page.NextCursorAfterID,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("ListDecisionPageWithBackend next: %v", err)
	}
	if len(next.Decisions) != 1 || next.Decisions[0].ID != second.BlockingTicketID || next.HasMore {
		t.Fatalf("next page = %+v, want second composite-key ticket without more", next)
	}

	detail, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if len(detail.Context.Fields) == 0 || len(detail.ResolutionPayload.Fields) == 0 {
		t.Fatalf("detail decision lost full payloads: %+v", detail)
	}
}

func TestListDecisionPageWithBackendScopesResolvedDuplicateTicketIDs(t *testing.T) {
	dataRoot := t.TempDir()
	const runID = "run-decision-page-shared"
	first := prepareDecisionRun(t, dataRoot, "ws-decision-page-shared-a", runID)
	second := prepareDecisionRun(t, dataRoot, "ws-decision-page-shared-b", runID)
	if first.BlockingTicketID != second.BlockingTicketID {
		t.Fatalf("test setup ticket ids = %q/%q, want duplicate IDs", first.BlockingTicketID, second.BlockingTicketID)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	for _, workspaceID := range []common.WorkspaceID{"ws-decision-page-shared-a", "ws-decision-page-shared-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		ticket, err := exports.Repositories.Tickets.Get(scoped, common.TicketID(first.BlockingTicketID))
		if err != nil {
			t.Fatalf("get ticket %s/%s: %v", workspaceID, first.BlockingTicketID, err)
		}
		ticket.State = common.DecisionTicketStateResolved
		if err := exports.Repositories.Tickets.Update(scoped, ticket); err != nil {
			t.Fatalf("update ticket %s/%s: %v", workspaceID, first.BlockingTicketID, err)
		}
		if err := exports.Repositories.Resolutions.Create(scoped, &decisionmodel.Resolution{
			ID:       common.ResolutionID("resolution-" + string(workspaceID)),
			TicketID: common.TicketID(first.BlockingTicketID),
			Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
			Payload:  mustAppserviceGovernancePayload(t, map[string]any{"workspaceId": string(workspaceID)}),
		}); err != nil {
			t.Fatalf("create resolution %s/%s: %v", workspaceID, first.BlockingTicketID, err)
		}
	}

	page, err := ListDecisionPageWithBackend(context.Background(), "file", dataRoot, workorchquerycontract.RunResourcePageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListDecisionPageWithBackend: %v", err)
	}
	if len(page.Decisions) != 2 {
		t.Fatalf("page decisions = %+v, want two scoped duplicate-ticket decisions", page.Decisions)
	}
	got := map[string]projectioncontract.Decision{}
	for _, item := range page.Decisions {
		got[item.WorkspaceID] = item
	}
	for _, workspaceID := range []string{"ws-decision-page-shared-a", "ws-decision-page-shared-b"} {
		item, ok := got[workspaceID]
		if !ok {
			t.Fatalf("page decisions = %+v, missing workspace %s", page.Decisions, workspaceID)
		}
		itemWorkspaceID, _ := item.ResolutionPayload.String("workspaceId")
		if item.ID != first.BlockingTicketID || itemWorkspaceID != string(workspaceID) {
			t.Fatalf("decision for %s = %+v, want scoped resolution payload", workspaceID, item)
		}
	}
}

func TestReadDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-read", "run-decision-read")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	item, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.ID != started.BlockingTicketID || item.State != "resolved" {
		t.Fatalf("decision = %+v, want resolved decision", item)
	}
	if item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionType = %q, want provide_missing_info", item.ResolutionType)
	}
}

func TestDecisionOperationsHonorRuntimeWorkspaceScope(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-scope-a", "run-decision-scope")

	readSession, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new read session: %v", err)
	}
	wrongCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-decision-scope-b")
	if _, err := ReadDecisionWithSession(wrongCtx, readSession, started.BlockingTicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadDecisionWithSession err = %v, want not found", err)
	}

	resolveSession, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new resolve session: %v", err)
	}
	if _, err := ResolveDecisionWithSession(wrongCtx, resolveSession, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ResolveDecisionWithSession err = %v, want not found", err)
	}

	item, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.State != string(common.DecisionTicketStateOpen) {
		t.Fatalf("decision state = %q, want open after wrong-scope resolve", item.State)
	}
}

func TestReadDecisionWithBackendIncludesAutoGovernanceSummary(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-decision-autogov"
	runID := "run-decision-autogov"
	started := prepareDecisionRun(t, dataRoot, workspaceID, runID)

	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	ctx := context.Background()
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("session exports: %v", err)
	}
	if err := exports.Repositories.AutoGovernanceTraces.Upsert(ctx, &governancemodel.AutoGovernanceTrace{
		ID:                common.AutoGovernanceTraceID("trace-decision-autogov"),
		WorkspaceID:       common.WorkspaceID(workspaceID),
		RunID:             common.RunID(runID),
		ItemType:          "ticket",
		ItemID:            started.BlockingTicketID,
		Action:            "answer",
		RiskLevel:         "L1",
		Confidence:        0.92,
		PolicyMatched:     "goal_context",
		Reason:            "derived from structured context",
		EvidenceRefs:      []string{"memory-recall-1"},
		PromptFingerprint: "fingerprint-1",
		Status:            "resolved",
		OccurredAt:        "2026-05-06T10:00:00Z",
	}); err != nil {
		t.Fatalf("upsert auto governance trace: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	item, err := ReadDecisionWithBackend(ctx, "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.AutoGovernance == nil ||
		item.AutoGovernance.Action != "answer" ||
		item.AutoGovernance.Status != "resolved" ||
		item.AutoGovernance.RiskLevel != "L1" {
		t.Fatalf("auto governance summary = %+v, want resolved ticket summary", item.AutoGovernance)
	}
}

func TestResolveDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-resolve", "run-decision-resolve")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "tester"}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	wantResolutionID := "resolution-" + started.WorkspaceID + "-" + started.BlockingTicketID + "-" + decisioncommon.ResolutionTypeProvideMissingInfo
	if item.ResolutionID != wantResolutionID {
		t.Fatalf("resolutionID = %q, want %s", item.ResolutionID, wantResolutionID)
	}
	operator, _ := item.ResolutionPayload.String("operator")
	if operator != "tester" {
		t.Fatalf("resolutionPayload = %+v, want operator tester", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionWithoutScopeConflictsOnDuplicateFileTicket(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "file", t.TempDir())
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	ticketID := common.TicketID("ticket-duplicate-resolution")
	runID := common.RunID("run-duplicate-resolution")
	oldCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-duplicate-old")
	currentCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-duplicate-current")
	if err := exports.Repositories.Tickets.Create(oldCtx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-duplicate-old",
		RunID:       runID,
		StepID:      "run-duplicate-resolution/step-1",
		Reason:      "old resolved ticket",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("create old ticket: %v", err)
	}
	if err := exports.Repositories.Resolutions.Create(oldCtx, &decisionmodel.Resolution{
		ID:       "resolution-ws-duplicate-old-ticket-duplicate-resolution-replan",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeReplan,
		Payload:  mustAppserviceGovernancePayload(t, map[string]any{"operator": "old"}),
	}); err != nil {
		t.Fatalf("create old resolution: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(currentCtx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-duplicate-current",
		RunID:       runID,
		StepID:      "run-duplicate-resolution/step-1",
		Reason:      "current open ticket",
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create current ticket: %v", err)
	}

	_, err = ResolveDecisionWithSession(context.Background(), session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "current"}),
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveDecisionWithSession err = %v, want conflict", err)
	}
	current, err := exports.Repositories.Tickets.Get(currentCtx, ticketID)
	if err != nil || current.State != common.DecisionTicketStateOpen {
		t.Fatalf("current duplicate ticket changed = %+v err=%v", current, err)
	}
	if _, err := exports.Repositories.Resolutions.GetByTicketID(currentCtx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("current duplicate resolution err = %v, want not found", err)
	}
}

func TestResolveDecisionWithBackendNormalizesSemanticIntent(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-semantic", "run-decision-semantic")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: "auto",
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer":   "请重新规划执行方案",
			"operator": "tui",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("decision = %+v, want resolved replan", item)
	}
	semantic, _ := item.ResolutionPayload.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	source, _ := semantic.String("source")
	rawInput, _ := semantic.String("rawInput")
	if intent != "request_replan" || source != "tui" || rawInput != "请重新规划执行方案" {
		t.Fatalf("semantic payload = %+v, want normalized intent/source/rawInput", semantic)
	}
}

func TestResolveDecisionWithSessionSelectsContinuationCandidateFromApproveIntent(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-candidate-continue")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  common.WorkspaceID("ws-candidate-continue"),
		RunID:        common.RunID("run-candidate-continue"),
		StepID:       common.StepID("run-candidate-continue/step-1"),
		Reason:       "execution target choice is missing",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Continue in the current workspace or provide another target?",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			"candidateValues": map[string]any{
				"executionTargetMode": []any{"init_from_scratch", "use_existing_project"},
			},
			"missingFields":        []any{"executionTargetMode"},
			"requiresHumanFields":  []any{"executionTargetMode"},
			"confidenceByField":    map[string]any{"executionTargetMode": 0.98},
			"knownValues":          map[string]any{"workspaceState": "empty"},
			"decisionContextScope": "target_workspace",
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer":   "继续处理",
			"operator": "tui",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	executionTargetMode, _ := item.ResolutionPayload.String("executionTargetMode")
	candidateSelectionMode, _ := item.ResolutionPayload.String("candidateSelectionMode")
	if executionTargetMode != "init_from_scratch" || candidateSelectionMode != "semantic_resume" {
		t.Fatalf("resolutionPayload = %+v, want selected continuation candidate", item.ResolutionPayload)
	}
	fields, ok := item.ResolutionPayload.StringSlice("selectedCandidateFields")
	if !ok || len(fields) != 1 || fields[0] != "executionTargetMode" {
		t.Fatalf("selectedCandidateFields = %+v, want executionTargetMode", fields)
	}
}

func TestResolveDecisionWithSessionRejectsInlineProviderConfigForProviderRecoveryTicket(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-provider-recovery")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-provider-recovery"),
		RunID:       common.RunID("run-provider-recovery"),
		StepID:      common.StepID("run-provider-recovery/step-1"),
		Reason:      "LLM provider retry attempts exhausted after 2 attempts",
		Question:    "Fix or switch the LLM provider in system configuration first.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			"failureClass":                                  "provider_timeout",
			governancecore.DecisionContextKeyScope:          governancecore.DecisionContextScopeWorkorchRuntime,
			governancecore.DecisionContextKeyDependency:     governancecore.DecisionContextDependencyLLMProvider,
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyConfigOnly,
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	_, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer": "provider = compatible\nbaseURL = https://api.example.com\napiKey = sk-secret\nmodel = model-a",
		}),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ResolveDecisionWithSession err = %v, want invalid argument", err)
	}
	item, getErr := tickets.Get(ctx, ticketID)
	if getErr != nil {
		t.Fatalf("get ticket: %v", getErr)
	}
	if item.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", item.State)
	}
	if _, getErr = resolutions.GetByTicketID(ctx, ticketID); !errors.Is(getErr, common.ErrNotFound) {
		t.Fatalf("resolution err = %v, want not found", getErr)
	}
}

func TestResolveDecisionWithSessionRejectsEmptyProvideMissingInfo(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-empty-provide-missing-info")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-empty-provide-missing-info"),
		RunID:       common.RunID("run-empty-provide-missing-info"),
		StepID:      common.StepID("run-empty-provide-missing-info/step-1"),
		Reason:      "runtime needs a valid decision resolution",
		Question:    "Provide missing information, replan, or reject current path.",
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	_, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer":   "/",
			"operator": "tui",
		}),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ResolveDecisionWithSession err = %v, want invalid argument", err)
	}
	item, getErr := tickets.Get(ctx, ticketID)
	if getErr != nil {
		t.Fatalf("get ticket: %v", getErr)
	}
	if item.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", item.State)
	}
	if _, getErr = resolutions.GetByTicketID(ctx, ticketID); !errors.Is(getErr, common.ErrNotFound) {
		t.Fatalf("resolution err = %v, want not found", getErr)
	}
}

func TestResolveDecisionWithSessionAllowsProviderRecoveryConfirmationWithoutSecrets(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-provider-recovery-confirmed")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-provider-recovery-confirmed"),
		RunID:       common.RunID("run-provider-recovery-confirmed"),
		StepID:      common.StepID("run-provider-recovery-confirmed/step-1"),
		Reason:      "LLM provider retry attempts exhausted after 2 attempts",
		Question:    "Fix or switch the LLM provider in system configuration first.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			"failureClass":                                  "provider_timeout",
			governancecore.DecisionContextKeyScope:          governancecore.DecisionContextScopeWorkorchRuntime,
			governancecore.DecisionContextKeyDependency:     governancecore.DecisionContextDependencyLLMProvider,
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyConfigOnly,
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer":   "已在系统配置中切换默认 LLM provider，继续执行",
			"operator": "console",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provider recovery confirmation", item)
	}
	recoveryAction, _ := item.ResolutionPayload.String("recoveryAction")
	if recoveryAction != "retry_same_step" {
		t.Fatalf("resolutionPayload = %+v, want retry_same_step recovery action", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionConvertsProviderRecoveryReplanToRetry(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-provider-recovery-replan")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:                     ticketID,
		WorkspaceID:            common.WorkspaceID("ws-provider-recovery-replan"),
		RunID:                  common.RunID("run-provider-recovery-replan"),
		StepID:                 common.StepID("run-provider-recovery-replan/step-1"),
		Reason:                 "LLM provider retry attempts exhausted after 2 attempts",
		Question:               "Fix or switch the LLM provider in system configuration first.",
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo, decisioncommon.ResolutionTypeRejectCurrentPath},
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			"failureClass":                                  "provider_timeout",
			governancecore.DecisionContextKeyScope:          governancecore.DecisionContextScopeWorkorchRuntime,
			governancecore.DecisionContextKeyDependency:     governancecore.DecisionContextDependencyLLMProvider,
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyConfigOnly,
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "console"}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provider retry", item)
	}
	if value, _ := item.ResolutionPayload.String("requestedResolutionType"); value != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("requestedResolutionType = %q, want replan", value)
	}
	if value, _ := item.ResolutionPayload.String("recoveryAction"); value != "retry_same_step" {
		t.Fatalf("recoveryAction = %q, want retry_same_step", value)
	}
	if value, _ := item.ResolutionPayload.String("resolutionNotice"); value == "" {
		t.Fatalf("resolutionNotice is empty")
	}
}

func TestResolveDecisionWithSessionAllowsTargetWorkspaceExternalDependencyConfig(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-target-workspace-test-config")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-target-workspace-test-config"),
		RunID:       common.RunID("run-target-workspace-test-config"),
		StepID:      common.StepID("run-target-workspace-test-config/step-1"),
		Reason:      "integration test configuration is missing",
		Question:    "Provide the target workspace external dependency test configuration.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			governancecore.DecisionContextKeyScope: governancecore.DecisionContextScopeTargetWorkspace,
			"dependencyKind":                       "external_service",
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer": "provider = compatible\nbaseURL = https://api.example.com\napiKey = sk-secret\nmodel = model-a",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want target workspace config accepted", item)
	}
	if rendered := fmt.Sprintf("%#v", item.ResolutionPayload); strings.Contains(strings.ToLower(rendered), "sk-secret") {
		t.Fatalf("resolutionPayload = %+v, want secret redacted", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionInfersRepairExhaustionReplanFromContinueUnderPolicy(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-repair-exhaustion-continue")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-continue"),
		RunID:       common.RunID("run-repair-exhaustion-continue"),
		StepID:      common.StepID("run-repair-exhaustion-continue/step-1"),
		Reason:      "runtime repair attempts exhausted after 3 attempts",
		Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyRepairPlan,
			"allowedRecoveryActions":                        []any{"replan"},
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: "auto",
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"answer":   "继续",
			"operator": "tui",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("decision = %+v, want resolved replan", item)
	}
	recoveryAction, _ := item.ResolutionPayload.String("recoveryAction")
	if recoveryAction != "replan" {
		t.Fatalf("resolutionPayload = %+v, want inferred replan", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionKeepsExplicitRepairExhaustionTerminateUnderReplanPolicy(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-repair-exhaustion-terminate-replan-policy")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-terminate-replan-policy"),
		RunID:       common.RunID("run-repair-exhaustion-terminate-replan-policy"),
		StepID:      common.StepID("run-repair-exhaustion-terminate-replan-policy/step-1"),
		Reason:      "runtime repair attempts exhausted after 3 attempts",
		Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyRepairPlan,
			"allowedRecoveryActions":                        []any{"replan"},
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"recoveryAction": "terminate",
			"operator":       "e2e",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", item)
	}
	recoveryAction, _ := item.ResolutionPayload.String("recoveryAction")
	if recoveryAction != "terminate" {
		t.Fatalf("resolutionPayload = %+v, want explicit terminate preserved", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithSessionAllowsExplicitRepairExhaustionRecoveryAction(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()
	ticketID := common.TicketID("ticket-repair-exhaustion-explicit")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-explicit"),
		RunID:       common.RunID("run-repair-exhaustion-explicit"),
		StepID:      common.StepID("run-repair-exhaustion-explicit/step-1"),
		Reason:      "runtime repair attempts exhausted after 3 attempts",
		Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: mustAppserviceGovernancePayload(t, map[string]any{
			governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyRepairPlan,
		}),
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
			"recoveryAction":    "retry_same_step",
			"resumeInstruction": "retry the failed verification after applying the clarified constraint",
			"operator":          "tui",
		}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved explicit recovery action", item)
	}
}

func TestResolveDecisionWithSessionMapsRepairExhaustionReplanAndTerminate(t *testing.T) {
	for _, tc := range []struct {
		name               string
		answer             string
		wantResolutionType string
		wantAction         string
	}{
		{name: "replan", answer: "请重新规划修复路径", wantResolutionType: decisioncommon.ResolutionTypeReplan, wantAction: "replan"},
		{name: "terminate", answer: "不继续，终止当前路径", wantResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath, wantAction: "terminate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, tickets, _ := newCountingDecisionRuntimeSession(t)
			ctx := context.Background()
			ticketID := common.TicketID("ticket-repair-exhaustion-" + tc.name)
			if err := tickets.Create(ctx, &decisionmodel.Ticket{
				ID:          ticketID,
				WorkspaceID: common.WorkspaceID("ws-repair-exhaustion-" + tc.name),
				RunID:       common.RunID("run-repair-exhaustion-" + tc.name),
				StepID:      common.StepID("run-repair-exhaustion-" + tc.name + "/step-1"),
				Reason:      "runtime repair attempts exhausted after 3 attempts",
				Question:    "Provide missing implementation guidance, choose replan, or reject the current path.",
				Context: mustAppserviceGovernancePayload(t, map[string]any{
					governancecore.DecisionContextKeyRecoveryPolicy: governancecore.DecisionContextRecoveryPolicyRepairPlan,
				}),
				State: common.DecisionTicketStateOpen,
			}); err != nil {
				t.Fatalf("create ticket: %v", err)
			}

			item, err := ResolveDecisionWithSession(ctx, session, string(ticketID), appmodelcore.ResolveDecisionCommand{
				ResolutionType: "auto",
				ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{
					"answer":   tc.answer,
					"operator": "tui",
				}),
			})
			if err != nil {
				t.Fatalf("ResolveDecisionWithSession: %v", err)
			}
			action, _ := item.ResolutionPayload.String("recoveryAction")
			if item.ResolutionType != tc.wantResolutionType || action != tc.wantAction {
				t.Fatalf("decision = %+v, want type=%s action=%s", item, tc.wantResolutionType, tc.wantAction)
			}
		})
	}
}

func TestResolveDecisionWithSessionReusesReturnedEntitiesForResponse(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()

	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-app-return-loaded"),
		WorkspaceID: common.WorkspaceID("ws-app-return-loaded"),
		RunID:       common.RunID("run-app-return-loaded"),
		StepID:      common.StepID("run-app-return-loaded/step-1"),
		Reason:      "need operator decision",
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	tickets.getCount.Store(0)
	resolutions.getByTicketIDCount.Store(0)

	item, err := ResolveDecisionWithSession(ctx, session, "ticket-app-return-loaded", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "tester"}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	operator, _ := item.ResolutionPayload.String("operator")
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo || operator != "tester" {
		t.Fatalf("decision = %+v, want resolved response from returned entities", item)
	}
	if got := tickets.getCount.Load(); got != 3 {
		t.Fatalf("ticket Get count = %d, want lookup + resolve + durable notification marker without app response reread", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 1 {
		t.Fatalf("resolution GetByTicketID count = %d, want one kernel orphan-check read on first resolve", got)
	}
}

func TestResolveDecisionWithBackendSupportsRejectCurrentPath(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-reject", "run-decision-reject")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeRejectCurrentPath,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "tester", "reason": "not viable"}),
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", item)
	}
	wantResolutionID := "resolution-" + started.WorkspaceID + "-" + started.BlockingTicketID + "-" + decisioncommon.ResolutionTypeRejectCurrentPath
	if item.ResolutionID != wantResolutionID {
		t.Fatalf("resolutionID = %q, want %s", item.ResolutionID, wantResolutionID)
	}
	reason, _ := item.ResolutionPayload.String("reason")
	if reason != "not viable" {
		t.Fatalf("resolutionPayload = %+v, want reject reason", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendReturnsConflictAfterResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-conflict", "run-decision-conflict")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResolveDecisionWithBackendReturnsExactOwnerReadbackAfterResponseLoss(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-idempotent", "run-decision-idempotent")
	payload := mustAppserviceGovernancePayload(t, map[string]any{"operator": "idempotent-tester", "note": "retry-safe"})
	first, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: payload,
	})
	if err != nil {
		t.Fatalf("first ResolveDecisionWithBackend: %v", err)
	}
	if first.State != "resolved" {
		t.Fatalf("first decision = %+v, want resolved", first)
	}

	second, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: payload,
	})
	if err != nil {
		t.Fatalf("response-loss retry ResolveDecisionWithBackend: %v", err)
	}
	if second.State != "resolved" || second.ResolutionID != first.ResolutionID {
		t.Fatalf("response-loss readback = %+v, want exact resolution %q", second, first.ResolutionID)
	}
}

func TestResolveDecisionWithBackendReturnsConflictForDifferentPayloadAfterResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-diffpayload", "run-decision-diffpayload")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "first"}),
	}); err != nil {
		t.Fatalf("first ResolveDecisionWithBackend: %v", err)
	}

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "second"}),
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict for payload mismatch", err)
	}
}

func TestResolveDecisionWithBackendRejectsUnsupportedResolutionType(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-invalid", "run-decision-invalid")

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: "unsupported_resolution",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendResolvesTicketsInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-a", "run-decision-batch-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-b", "run-decision-batch-b")

	items, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         []string{second.BlockingTicketID, first.BlockingTicketID},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "batch-tester"}),
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.BlockingTicketID || items[1].ID != first.BlockingTicketID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		operator, _ := item.ResolutionPayload.String("operator")
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || operator != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan with payload", item)
		}
	}
}

func TestBatchResolveDecisionsWithSessionReusesReturnedEntitiesAfterPrevalidation(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()

	for idx, ticketID := range []common.TicketID{"ticket-app-batch-a", "ticket-app-batch-b"} {
		workspaceID := common.WorkspaceID(fmt.Sprintf("ws-app-batch-%d", idx))
		runID := common.RunID(fmt.Sprintf("run-app-batch-%d", idx))
		if err := tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      common.StepID(string(runID) + "/step"),
			Reason:      "need operator decision",
			State:       common.DecisionTicketStateOpen,
		}); err != nil {
			t.Fatalf("create ticket %s: %v", ticketID, err)
		}
	}
	tickets.getCount.Store(0)
	resolutions.getByTicketIDCount.Store(0)

	items, err := BatchResolveDecisionsWithSession(ctx, session, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         []string{"ticket-app-batch-b", "ticket-app-batch-a"},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "batch-tester"}),
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithSession: %v", err)
	}
	if len(items) != 2 || items[0].ID != "ticket-app-batch-b" || items[1].ID != "ticket-app-batch-a" {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		operator, _ := item.ResolutionPayload.String("operator")
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || operator != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan response", item)
		}
	}
	if got := tickets.getCount.Load(); got != 6 {
		t.Fatalf("ticket Get count = %d, want lookup + resolve + durable notification marker per ticket", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 2 {
		t.Fatalf("resolution GetByTicketID count = %d, want one orphan-check read per ticket on first batch resolve", got)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsDuplicateTicketIDs(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-batch-dup", "run-decision-batch-dup")

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:      []string{started.BlockingTicketID, started.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendAcceptsExactResponseLossDuplicate(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-a", "run-decision-batch-conflict-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-b", "run-decision-batch-conflict-b")
	// Seed second with the same resolution type/payload the batch will use.
	// Its terminal owner state is a response-loss candidate; Decision owns the
	// complete identity/payload comparison and returns the exact readback.
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "batch-tester"}),
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	items, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         []string{first.BlockingTicketID, second.BlockingTicketID},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustAppserviceGovernancePayload(t, map[string]any{"operator": "batch-tester"}),
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.BlockingTicketID || items[1].ID != second.BlockingTicketID {
		t.Fatalf("items = %+v, want request order with exact duplicate readback", items)
	}

	firstDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if firstDecision.State != "resolved" {
		t.Fatalf("first decision = %+v, want resolved", firstDecision)
	}
	secondDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if secondDecision.State != "resolved" || secondDecision.ResolutionID != items[1].ResolutionID {
		t.Fatalf("second decision = %+v, want exact pre-existing resolution readback", secondDecision)
	}
}

func TestBatchResolveDecisionsWithBackendReturnsConflictForDifferentPayloadInBatch(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-diffpayload-a", "run-decision-batch-diffpayload-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-diffpayload-b", "run-decision-batch-diffpayload-b")
	// Seed second with a different type. Decision rejects the non-identical
	// terminal retry; the earlier open target retains the historical sequential
	// batch behavior and is already resolved.
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID, appmodelcore.ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:      []string{first.BlockingTicketID, second.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeReplan,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
	firstDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if firstDecision.State != "resolved" || firstDecision.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("first decision = %+v, want resolved with replan before owner duplicate conflict", firstDecision)
	}
}
