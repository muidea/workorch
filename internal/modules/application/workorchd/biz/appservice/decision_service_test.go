package appservice

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
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

func newCountingDecisionRuntimeSession(t *testing.T) (*RuntimeSession, *countingAppDecisionTicketRepository, *countingAppDecisionResolutionRepository) {
	t.Helper()

	tickets := newCountingAppDecisionTicketRepository()
	resolutions := newCountingAppDecisionResolutionRepository()
	decisionModule := decisionmodule.New()
	if err := decisionModule.Setup(decisionmodule.Dependencies{Tickets: tickets, Resolutions: resolutions}); err != nil {
		t.Fatalf("setup decision module: %v", err)
	}
	workspaceModule := workspacemodule.New()
	if err := workspaceModule.Setup(workspacemodule.Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
	}); err != nil {
		t.Fatalf("setup workspace module: %v", err)
	}
	approvalModule := approvalmodule.New()
	if err := approvalModule.Setup(approvalmodule.Dependencies{Approvals: memory.NewApprovalRepository()}); err != nil {
		t.Fatalf("setup approval module: %v", err)
	}

	session, err := NewRuntimeSessionFromExports(bootstrap.Exports{
		Repositories: bootstrap.RepositoryExports{
			Workspaces:         memory.NewWorkspaceRepository(),
			Runs:               memory.NewRunRepository(),
			Steps:              memory.NewStepRepository(),
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			EventRecords:       memory.NewEventRecordRepository(),
			MCPCapabilities:    memory.NewMCPCapabilityRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			Tickets:            tickets,
			Resolutions:        resolutions,
			Approvals:          memory.NewApprovalRepository(),
		},
		Services: bootstrap.ServiceExports{
			WorkspaceService: workspaceModule.Service(),
			RunService:       runservice.New(runservice.Dependencies{}),
			DecisionService:  decisionModule.Service(),
			ApprovalService:  approvalModule.Service(),
		},
	})
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

	filteredByRun, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "run-decision-list-b")
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

func TestReadDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-read", "run-decision-read")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
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

func TestResolveDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-resolve", "run-decision-resolve")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{"operator": "tester"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionID = %q, want resolution-%s-%s", item.ResolutionID, started.BlockingTicketID, decisioncommon.ResolutionTypeProvideMissingInfo)
	}
	if item.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("resolutionPayload = %+v, want operator tester", item.ResolutionPayload)
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

	item, err := ResolveDecisionWithSession(ctx, session, "ticket-app-return-loaded", ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{"operator": "tester"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithSession: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo || item.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("decision = %+v, want resolved response from returned entities", item)
	}
	if got := tickets.getCount.Load(); got != 1 {
		t.Fatalf("ticket Get count = %d, want one service read without app response reread", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 0 {
		t.Fatalf("resolution GetByTicketID count = %d, want no app response reread on first resolve", got)
	}
}

func TestResolveDecisionWithBackendSupportsRejectCurrentPath(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-reject", "run-decision-reject")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeRejectCurrentPath,
		ResolutionPayload: map[string]any{"operator": "tester", "reason": "not viable"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("resolutionID = %q, want formal reject resolution id", item.ResolutionID)
	}
	if item.ResolutionPayload["reason"] != "not viable" {
		t.Fatalf("resolutionPayload = %+v, want reject reason", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendReturnsConflictAfterResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-conflict", "run-decision-conflict")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResolveDecisionWithBackendRejectsUnsupportedResolutionType(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-invalid", "run-decision-invalid")

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: "unsupported_resolution",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendResolvesTicketsInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-a", "run-decision-batch-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-b", "run-decision-batch-b")

	items, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:         []string{second.BlockingTicketID, first.BlockingTicketID},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: map[string]any{"operator": "batch-tester"},
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.BlockingTicketID || items[1].ID != first.BlockingTicketID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan with payload", item)
		}
	}
}

func TestBatchResolveDecisionsWithSessionReusesReturnedEntitiesAfterPrevalidation(t *testing.T) {
	session, tickets, resolutions := newCountingDecisionRuntimeSession(t)
	ctx := context.Background()

	for _, ticketID := range []common.TicketID{"ticket-app-batch-a", "ticket-app-batch-b"} {
		if err := tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          ticketID,
			WorkspaceID: common.WorkspaceID("ws-app-batch"),
			RunID:       common.RunID("run-app-batch"),
			StepID:      common.StepID("run-app-batch/step"),
			Reason:      "need operator decision",
			State:       common.DecisionTicketStateOpen,
		}); err != nil {
			t.Fatalf("create ticket %s: %v", ticketID, err)
		}
	}
	tickets.getCount.Store(0)
	resolutions.getByTicketIDCount.Store(0)

	items, err := BatchResolveDecisionsWithSession(ctx, session, BatchResolveDecisionCommand{
		TicketIDs:         []string{"ticket-app-batch-b", "ticket-app-batch-a"},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: map[string]any{"operator": "batch-tester"},
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithSession: %v", err)
	}
	if len(items) != 2 || items[0].ID != "ticket-app-batch-b" || items[1].ID != "ticket-app-batch-a" {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan response", item)
		}
	}
	if got := tickets.getCount.Load(); got != 4 {
		t.Fatalf("ticket Get count = %d, want prevalidation plus one service read per ticket", got)
	}
	if got := resolutions.getByTicketIDCount.Load(); got != 0 {
		t.Fatalf("resolution GetByTicketID count = %d, want no response reread on first batch resolve", got)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsDuplicateTicketIDs(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-batch-dup", "run-decision-batch-dup")

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{started.BlockingTicketID, started.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsAlreadyResolvedTicketBeforeMutation(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-a", "run-decision-batch-conflict-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-b", "run-decision-batch-conflict-b")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{first.BlockingTicketID, second.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeReplan,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
	firstDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if firstDecision.State != "open" {
		t.Fatalf("first decision = %+v, want still open after prevalidation conflict", firstDecision)
	}
}
