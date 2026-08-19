package decision

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionservice "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionModel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustDecisionPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type recordingDecisionHub struct {
	magicevent.Hub
	mu     sync.Mutex
	posted []magicevent.Event
}

func newRecordingDecisionHub(t *testing.T) *recordingDecisionHub {
	t.Helper()
	rawHub := magicevent.NewHub(32)
	hub := &recordingDecisionHub{Hub: rawHub}
	evidenceObserver := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	evidenceObserver.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(decisionevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	t.Cleanup(func() {
		evidenceObserver.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
		rawHub.Terminate(context.Background())
	})
	return hub
}

func (h *recordingDecisionHub) Post(event magicevent.Event) {
	h.mu.Lock()
	h.posted = append(h.posted, event)
	h.mu.Unlock()
	h.Hub.Post(event)
}

func (h *recordingDecisionHub) Posted() []magicevent.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]magicevent.Event(nil), h.posted...)
}

type countingDecisionTicketRepository struct {
	inner          *memory.DecisionTicketRepository
	getCount       atomic.Int64
	updates        atomic.Int64
	failNextUpdate atomic.Bool
}

func newCountingDecisionTicketRepository() *countingDecisionTicketRepository {
	return &countingDecisionTicketRepository{inner: memory.NewDecisionTicketRepository()}
}

func (r *countingDecisionTicketRepository) Create(ctx context.Context, item *decisionModel.Ticket) error {
	return r.inner.Create(ctx, item)
}

func (r *countingDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionModel.Ticket, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingDecisionTicketRepository) List(ctx context.Context) ([]*decisionModel.Ticket, error) {
	return r.inner.List(ctx)
}

func (r *countingDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionModel.Ticket, error) {
	return r.inner.ListByRunID(ctx, runID)
}

func (r *countingDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionModel.Ticket, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingDecisionTicketRepository) Update(ctx context.Context, item *decisionModel.Ticket) error {
	r.updates.Add(1)
	if r.failNextUpdate.Swap(false) {
		return errors.New("injected ticket update failure")
	}
	return r.inner.Update(ctx, item)
}

func (r *countingDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	return r.inner.Delete(ctx, id)
}

type countingDecisionResolutionRepository struct {
	inner              *memory.DecisionResolutionRepository
	getByTicketIDCount atomic.Int64
}

type failingDeleteDecisionResolutionRepository struct {
	inner      *memory.DecisionResolutionRepository
	failDelete atomic.Bool
}

func newFailingDeleteDecisionResolutionRepository() *failingDeleteDecisionResolutionRepository {
	return &failingDeleteDecisionResolutionRepository{inner: memory.NewDecisionResolutionRepository()}
}

func (r *failingDeleteDecisionResolutionRepository) Create(ctx context.Context, item *decisionModel.Resolution) error {
	return r.inner.Create(ctx, item)
}

func (r *failingDeleteDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionModel.Resolution, error) {
	return r.inner.GetByTicketID(ctx, ticketID)
}

func (r *failingDeleteDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	if r.failDelete.Load() {
		return errors.New("injected resolution delete failure")
	}
	return r.inner.DeleteByTicketID(ctx, ticketID)
}

func newCountingDecisionResolutionRepository() *countingDecisionResolutionRepository {
	return &countingDecisionResolutionRepository{inner: memory.NewDecisionResolutionRepository()}
}

func (r *countingDecisionResolutionRepository) Create(ctx context.Context, item *decisionModel.Resolution) error {
	return r.inner.Create(ctx, item)
}

func (r *countingDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionModel.Resolution, error) {
	r.getByTicketIDCount.Add(1)
	return r.inner.GetByTicketID(ctx, ticketID)
}

func (r *countingDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	return r.inner.DeleteByTicketID(ctx, ticketID)
}

func TestService_CreateTicket_SeedsOpenState(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	cmd := decisionservice.CreateTicketCommand{
		TicketID:               common.TicketID("ticket-1"),
		WorkspaceID:            common.WorkspaceID("ws-1"),
		RunID:                  common.RunID("run-1"),
		StepID:                 common.StepID("run-1/step-1"),
		Reason:                 "need operator decision",
		AutoGovernance:         mustDecisionPayload(t, map[string]any{"policy": "manual"}),
		FailureClass:           "runtime_failed",
		FailureSignature:       "class=runtime_failed",
		AllowedRecoveryActions: []string{"replan"},
	}
	if err := service.CreateTicket(ctx, cmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, cmd.TicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %q, want open", ticket.State)
	}
	policy, _ := ticket.AutoGovernance.String("policy")
	if policy != "manual" ||
		ticket.FailureClass != "runtime_failed" ||
		ticket.FailureSignature != "class=runtime_failed" ||
		len(ticket.AllowedRecoveryActions) != 1 ||
		ticket.AllowedRecoveryActions[0] != "replan" {
		t.Fatalf("ticket governance fields = %+v", ticket)
	}
}

func TestService_RuntimeGovernanceProposalStaysOpenUntilFinalize(t *testing.T) {
	ctx := context.Background()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	service := NewService(tickets, resolutions)
	if err := tickets.Create(ctx, &decisionModel.Ticket{
		ID:                     "ticket-runtime-proposal",
		WorkspaceID:            "ws-runtime-proposal",
		RunID:                  "run-runtime-proposal",
		StepID:                 "runtime/dispatch-1",
		Reason:                 "provider recovery needs confirmation",
		DecisionKind:           "runtime_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: "resolution-runtime-proposal",
		TicketID:     "ticket-runtime-proposal",
		WorkspaceID:  "ws-runtime-proposal",
		RunID:        "run-runtime-proposal",
		StepID:       "runtime/dispatch-1",
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"summary": "provider is ready"}),
	}
	ticket, proposal, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, cmd)
	if err != nil {
		t.Fatalf("persist proposal: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen || proposal == nil || proposal.State != decisionModel.RuntimeGovernanceResolutionProposalPending {
		t.Fatalf("ticket/proposal after pending persist = %+v / %+v", ticket, proposal)
	}
	if _, err := resolutions.GetByTicketID(ctx, cmd.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("formal resolution err = %v, want not found before aggregate commit", err)
	}
	ticket, resolution, err := service.FinalizeRuntimeGovernanceResolutionProposal(ctx, cmd)
	if err != nil {
		t.Fatalf("finalize proposal: %v", err)
	}
	if ticket.State != common.DecisionTicketStateResolved || ticket.RuntimeGovernanceProposal == nil || ticket.RuntimeGovernanceProposal.State != decisionModel.RuntimeGovernanceResolutionProposalCommitted || resolution == nil {
		t.Fatalf("ticket/resolution after finalize = %+v / %+v", ticket, resolution)
	}
}

func TestService_RuntimeGovernanceCommittedProposalReplaysExactResolution(t *testing.T) {
	ctx := context.Background()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	service := NewService(tickets, resolutions)
	if err := tickets.Create(ctx, &decisionModel.Ticket{
		ID:                     "ticket-runtime-committed-replay",
		WorkspaceID:            "ws-runtime-committed-replay",
		RunID:                  "run-runtime-committed-replay",
		StepID:                 "runtime/dispatch-1",
		Reason:                 "provider recovery needs confirmation",
		DecisionKind:           "runtime_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: "resolution-runtime-committed-replay",
		TicketID:     "ticket-runtime-committed-replay",
		WorkspaceID:  "ws-runtime-committed-replay",
		RunID:        "run-runtime-committed-replay",
		StepID:       "runtime/dispatch-1",
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"summary": "provider is ready"}),
	}
	if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, cmd); err != nil {
		t.Fatalf("persist proposal: %v", err)
	}
	if _, _, err := service.FinalizeRuntimeGovernanceResolutionProposal(ctx, cmd); err != nil {
		t.Fatalf("finalize proposal: %v", err)
	}
	replayedTicket, replayedProposal, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, cmd)
	if err != nil {
		t.Fatalf("replay committed proposal: %v", err)
	}
	if replayedTicket.State != common.DecisionTicketStateResolved || replayedProposal == nil || replayedProposal.State != decisionModel.RuntimeGovernanceResolutionProposalCommitted || replayedProposal.ResolutionID != cmd.ResolutionID {
		t.Fatalf("replayed committed proposal = %+v / %+v", replayedTicket, replayedProposal)
	}
	resolution, err := resolutions.GetByTicketID(ctx, cmd.TicketID)
	if err != nil || resolution.ID != cmd.ResolutionID {
		t.Fatalf("replay changed persisted resolution = %+v err=%v", resolution, err)
	}
	changed := cmd
	changed.Payload = mustDecisionPayload(t, map[string]any{"summary": "changed input"})
	if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed replay err = %v, want conflict", err)
	}
}

func TestService_RuntimeGovernanceCommittedFinalizeReplayIsZeroWrite(t *testing.T) {
	ctx := context.Background()
	tickets := newCountingDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	service := NewService(tickets, resolutions)
	if err := tickets.Create(ctx, &decisionModel.Ticket{
		ID:                     "ticket-runtime-zero-write",
		WorkspaceID:            "ws-runtime-zero-write",
		RunID:                  "run-runtime-zero-write",
		StepID:                 "runtime/dispatch-zero-write",
		Reason:                 "confirm recovery",
		DecisionKind:           "runtime_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID:       "resolution-runtime-zero-write",
		TicketID:           "ticket-runtime-zero-write",
		WorkspaceID:        "ws-runtime-zero-write",
		RunID:              "run-runtime-zero-write",
		StepID:             "runtime/dispatch-zero-write",
		Type:               decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:            mustDecisionPayload(t, map[string]any{"summary": "continue", "apiKey": "sk-secret123456"}),
		ContinuationAction: "resume",
		ResumeRunID:        "run-runtime-zero-write",
	}
	if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, cmd); err != nil {
		t.Fatalf("prepare proposal: %v", err)
	}
	first, _, transitioned, err := service.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, cmd)
	if err != nil || !transitioned {
		t.Fatalf("first finalize = transitioned:%v err:%v", transitioned, err)
	}
	updates := tickets.updates.Load()
	updatedAt := first.UpdatedAt
	proposalUpdatedAt := first.RuntimeGovernanceProposal.UpdatedAt
	replayed, _, transitioned, err := service.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, cmd)
	if err != nil {
		t.Fatalf("replay finalize: %v", err)
	}
	if transitioned {
		t.Fatal("exact finalize replay unexpectedly transitioned")
	}
	if got := tickets.updates.Load(); got != updates {
		t.Fatalf("exact finalize replay wrote ticket: updates %d -> %d", updates, got)
	}
	if replayed.UpdatedAt != updatedAt || replayed.RuntimeGovernanceProposal.UpdatedAt != proposalUpdatedAt {
		t.Fatalf("exact finalize replay changed timestamps: ticket %q -> %q proposal %q -> %q", updatedAt, replayed.UpdatedAt, proposalUpdatedAt, replayed.RuntimeGovernanceProposal.UpdatedAt)
	}
}

func TestService_OrdinaryResolutionReplayUsesCanonicalSealedFingerprint(t *testing.T) {
	ctx := context.Background()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	service := NewService(tickets, resolutions)
	if err := service.CreateTicket(ctx, decisionservice.CreateTicketCommand{
		TicketID: "ticket-resolution-fingerprint", WorkspaceID: "ws-resolution-fingerprint", RunID: "run-resolution-fingerprint", Reason: "need input",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID:       "resolution-fingerprint",
		TicketID:           "ticket-resolution-fingerprint",
		WorkspaceID:        "ws-resolution-fingerprint",
		RunID:              "run-resolution-fingerprint",
		Type:               decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:            mustDecisionPayload(t, map[string]any{"answer": "continue", "apiKey": "sk-secret123456"}),
		ContinuationAction: "resume",
		ResumeRunID:        "run-resolution-fingerprint",
		SuccessorRunID:     "run-resolution-successor",
	}
	if _, _, _, err := service.EnsureResolvedTicketAndReturnWithTransition(ctx, cmd); err != nil {
		t.Fatalf("first resolution: %v", err)
	}
	if _, _, transitioned, err := service.EnsureResolvedTicketAndReturnWithTransition(ctx, cmd); err != nil || transitioned {
		t.Fatalf("same raw payload replay = transitioned:%v err:%v", transitioned, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*decisionservice.ResolveTicketCommand)
	}{
		{name: "continuation", change: func(value *decisionservice.ResolveTicketCommand) { value.ContinuationAction = "restart" }},
		{name: "resume run", change: func(value *decisionservice.ResolveTicketCommand) { value.ResumeRunID = "run-other" }},
		{name: "successor run", change: func(value *decisionservice.ResolveTicketCommand) { value.SuccessorRunID = "run-other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := cmd
			tc.change(&changed)
			if _, _, _, err := service.EnsureResolvedTicketAndReturnWithTransition(ctx, changed); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("changed replay err = %v, want conflict", err)
			}
		})
	}
}

func TestService_DecisionAggregatePurgeHidesAndRecoversPartialDeletion(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-decision-purge-recovery")
	ticketID := common.TicketID("ticket-decision-purge-recovery")
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := newFailingDeleteDecisionResolutionRepository()
	if err := tickets.Create(scopedCtx, &decisionModel.Ticket{
		ID: ticketID, WorkspaceID: workspaceID, RunID: "run-decision-purge-recovery", State: common.DecisionTicketStateResolved, Reason: "purge recovery",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := resolutions.Create(scopedCtx, &decisionModel.Resolution{ID: "resolution-decision-purge-recovery", TicketID: ticketID, Type: decisioncommon.ResolutionTypeProvideMissingInfo}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	owner, err := New(rawHub, nil, Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub})
	if err != nil {
		t.Fatalf("New Biz: %v", err)
	}
	item, err := tickets.Get(scopedCtx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	resolutions.failDelete.Store(true)
	if err := owner.service.PurgeDecisionAggregate(scopedCtx, item); err == nil {
		t.Fatal("partial purge unexpectedly succeeded")
	}
	tombstone, err := tickets.Get(scopedCtx, ticketID)
	if err != nil || tombstone.DeletionRequestedAt == "" {
		t.Fatalf("partial purge tombstone = %+v err=%v", tombstone, err)
	}
	if _, err := owner.executeGetTicketCommand(scopedCtx, decisionevents.GetTicketCommand{WorkspaceID: workspaceID, TicketID: ticketID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("public get of tombstone err = %v, want not found", err)
	}
	page, err := owner.executeListTicketsByWorkspaceCommand(scopedCtx, decisionevents.ListTicketsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil || len(page.Tickets) != 0 {
		t.Fatalf("public list of tombstone = %+v err=%v", page, err)
	}

	resolutions.failDelete.Store(false)
	if err := owner.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("recover aggregate purge: %v", err)
	}
	if _, err := tickets.Get(scopedCtx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ticket after purge recovery err = %v, want not found", err)
	}
	if _, err := resolutions.GetByTicketID(scopedCtx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("resolution after purge recovery err = %v, want not found", err)
	}
}

func TestBiz_ReconciliationClosesOrdinaryResolutionWriteCut(t *testing.T) {
	ctx := context.Background()
	tickets := newCountingDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	owner, err := New(hub, nil, Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub})
	if err != nil {
		t.Fatalf("New Biz: %v", err)
	}
	create := decisionservice.CreateTicketCommand{
		TicketID: "ticket-ordinary-cut", WorkspaceID: "ws-ordinary-cut", RunID: "run-ordinary-cut", Reason: "need input",
	}
	if err := owner.service.CreateTicket(ctx, create); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	tickets.failNextUpdate.Store(true)
	command := decisionevents.EnsureResolvedCommand{
		ResolutionID: "resolution-ordinary-cut", TicketID: create.TicketID, WorkspaceID: create.WorkspaceID, RunID: create.RunID,
		Type: decisioncommon.ResolutionTypeProvideMissingInfo, Payload: mustDecisionPayload(t, map[string]any{"answer": "continue"}).Fields,
	}
	if _, err := owner.executeEnsureResolvedCommand(ctx, command); err == nil {
		t.Fatal("resolution unexpectedly crossed injected ticket update failure")
	}
	open, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, create.WorkspaceID), create.TicketID)
	if err != nil || open.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket at write cut = %+v err=%v", open, err)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, create.WorkspaceID), create.TicketID); err != nil {
		t.Fatalf("orphan resolution was not persisted: %v", err)
	}
	if err := owner.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("reconcile ordinary resolution cut: %v", err)
	}
	resolved, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, create.WorkspaceID), create.TicketID)
	if err != nil || resolved.State != common.DecisionTicketStateResolved || resolved.ResolutionNotificationPublishedAt == "" {
		t.Fatalf("recovered ticket = %+v err=%v", resolved, err)
	}
	published := hub.Posted()
	if len(published) != 1 || published[0].ID() != decisionevents.TopicTicketResolved {
		t.Fatalf("recovery notifications = %+v, want one resolved notification", published)
	}
	if err := owner.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("idempotent ordinary reconciliation: %v", err)
	}
	if got := len(hub.Posted()); got != 1 {
		t.Fatalf("ordinary notification replayed after durable marker: %d", got)
	}
}

func TestService_RuntimeGovernanceRejectsUnallowedActionBeforeProposal(t *testing.T) {
	ctx := context.Background()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	service := NewService(tickets, resolutions)
	if err := tickets.Create(ctx, &decisionModel.Ticket{
		ID:                     "ticket-runtime-actions",
		WorkspaceID:            "ws-runtime-actions",
		RunID:                  "run-runtime-actions",
		StepID:                 "runtime/dispatch-1",
		Reason:                 "provider recovery needs confirmation",
		DecisionKind:           "runtime_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	_, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: "resolution-runtime-actions",
		TicketID:     "ticket-runtime-actions",
		WorkspaceID:  "ws-runtime-actions",
		RunID:        "run-runtime-actions",
		StepID:       "runtime/dispatch-1",
		Type:         decisioncommon.ResolutionTypeReplan,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unallowed proposal err = %v, want conflict", err)
	}
	var actionConflict *decisioncommon.ResolutionActionConflictError
	if !errors.As(err, &actionConflict) || actionConflict.TicketID != "ticket-runtime-actions" || actionConflict.RequestedType != decisioncommon.ResolutionTypeReplan || len(actionConflict.AllowedActions) != 1 || actionConflict.AllowedActions[0] != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("unallowed proposal conflict = %#v, want actionable ticket conflict", actionConflict)
	}
	ticket, err := tickets.Get(ctx, "ticket-runtime-actions")
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if ticket.RuntimeGovernanceProposal != nil || ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("unallowed action mutated ticket: %+v", ticket)
	}
}

func TestService_CreateTicket_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository())

	tests := []decisionservice.CreateTicketCommand{
		{TicketID: "", RunID: common.RunID("run-1"), Reason: "need operator decision"},
		{TicketID: common.TicketID("ticket-1"), WorkspaceID: "", RunID: common.RunID("run-1"), Reason: "need operator decision"},
		{TicketID: common.TicketID("ticket-1"), RunID: "", Reason: "need operator decision"},
		{TicketID: common.TicketID("ticket-1"), RunID: common.RunID("run-1"), Reason: ""},
	}

	for _, tt := range tests {
		if err := service.CreateTicket(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}
}

func TestService_CreateTicket_RejectsDuplicateTicketID(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	first := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-dup"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, first); err != nil {
		t.Fatalf("first create ticket: %v", err)
	}

	second := decisionservice.CreateTicketCommand{
		TicketID:    first.TicketID,
		WorkspaceID: first.WorkspaceID,
		RunID:       common.RunID("run-2"),
		StepID:      common.StepID("run-2/step-1"),
		Reason:      "other decision",
	}
	if err := service.CreateTicket(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second create err = %v, want conflict", err)
	}

	got, err := ticketRepo.Get(ctx, first.TicketID)
	if err != nil {
		t.Fatalf("get ticket after duplicate create: %v", err)
	}
	if got.RunID != first.RunID || got.Reason != first.Reason {
		t.Fatalf("ticket mutated after duplicate create: %+v", got)
	}
}

func TestService_CreateTicket_AllowsDuplicateTicketIDAcrossWorkspaces(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	first := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-workspace-local"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	second := decisionservice.CreateTicketCommand{
		TicketID:    first.TicketID,
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       common.RunID("run-2"),
		StepID:      common.StepID("run-2/step-1"),
		Reason:      "other decision",
	}
	if err := service.CreateTicket(ctx, first); err != nil {
		t.Fatalf("first create ticket: %v", err)
	}
	if err := service.CreateTicket(ctx, second); err != nil {
		t.Fatalf("second workspace-local create ticket: %v", err)
	}
	if _, err := ticketRepo.Get(ctx, first.TicketID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped get err = %v, want conflict for duplicate workspace-local ticket id", err)
	}
	gotFirst, err := ticketRepo.Get(common.ContextWithRuntimeWorkspaceID(ctx, first.WorkspaceID), first.TicketID)
	if err != nil {
		t.Fatalf("get first scoped ticket: %v", err)
	}
	gotSecond, err := ticketRepo.Get(common.ContextWithRuntimeWorkspaceID(ctx, second.WorkspaceID), second.TicketID)
	if err != nil {
		t.Fatalf("get second scoped ticket: %v", err)
	}
	if gotFirst.RunID != first.RunID || gotSecond.RunID != second.RunID {
		t.Fatalf("scoped tickets = %+v/%+v, want distinct workspace-local records", gotFirst, gotSecond)
	}
}

func TestService_EnsureTicket_IsIdempotentAndRejectsBindingMismatch(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	service := NewService(ticketRepo, memory.NewDecisionResolutionRepository())

	cmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-ensure"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.EnsureTicket(ctx, cmd); err != nil {
		t.Fatalf("first ensure ticket: %v", err)
	}
	if err := service.EnsureTicket(ctx, cmd); err != nil {
		t.Fatalf("second ensure ticket: %v", err)
	}

	if err := service.EnsureTicket(ctx, decisionservice.CreateTicketCommand{
		TicketID:    cmd.TicketID,
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("binding mismatch err = %v, want conflict", err)
	}

	if err := service.EnsureTicket(ctx, decisionservice.CreateTicketCommand{
		TicketID:    cmd.TicketID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      "other reason",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("reason mismatch err = %v, want conflict", err)
	}
}

func TestService_ResolveTicket_OpenToResolvedAndPersistsResolution(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	resolveCmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-1"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"actor": "operator"}),
	}
	if err := service.ResolveTicket(ctx, resolveCmd); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get resolved ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket state = %q, want resolved", ticket.State)
	}
	if ticket.ResolvedAt == "" || ticket.ResolvedBy != "operator" {
		t.Fatalf("ticket resolution audit fields = %+v", ticket)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if resolution.ID != resolveCmd.ResolutionID {
		t.Fatalf("resolution id = %q, want %q", resolution.ID, resolveCmd.ResolutionID)
	}
	if resolution.Type != resolveCmd.Type {
		t.Fatalf("resolution type = %q, want %q", resolution.Type, resolveCmd.Type)
	}
}

func TestService_ResolveTicket_DoesNotUseEventHub(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)
	var evidenceCommand decisionevents.RecordDecisionEvidenceCommand
	observer := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(decisionevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		evidenceCommand = command
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	defer observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:     common.TicketID("ticket-evidence"),
		WorkspaceID:  common.WorkspaceID("ws-evidence"),
		RunID:        common.RunID("run-evidence"),
		StepID:       common.StepID("run-evidence/decision"),
		Reason:       "need operator decision",
		DecisionKind: "operator_input",
		Question:     "which recovery path should continue",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	resolveCmd := decisionservice.ResolveTicketCommand{
		ResolutionID:       common.ResolutionID("resolution-evidence"),
		TicketID:           createCmd.TicketID,
		WorkspaceID:        createCmd.WorkspaceID,
		RunID:              createCmd.RunID,
		StepID:             createCmd.StepID,
		Type:               decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:            mustDecisionPayload(t, map[string]any{"operatorInput": "continue with current plan"}),
		ContinuationAction: "resume",
		ResumeRunID:        createCmd.RunID,
	}
	if err := service.EnsureResolvedTicket(ctx, resolveCmd); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, resolveCmd); err != nil {
		t.Fatalf("second ensure resolved ticket: %v", err)
	}

	if evidenceCommand.EvidenceRecordID != "" {
		t.Fatalf("service unexpectedly submitted evidence: %#v", evidenceCommand)
	}
}

func TestService_ResolveTicket_RejectsSecondResolveOnResolvedTicket(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	first := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-1"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
	}
	if err := service.ResolveTicket(ctx, first); err != nil {
		t.Fatalf("first resolve ticket: %v", err)
	}

	second := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-2"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeRejectCurrentPath,
	}
	if err := service.ResolveTicket(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second resolve err = %v, want conflict", err)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get persisted resolution: %v", err)
	}
	if resolution.ID != first.ResolutionID {
		t.Fatalf("persisted resolution id = %q, want %q", resolution.ID, first.ResolutionID)
	}
}

func TestService_EnsureResolvedTicket_IsIdempotentAndRejectsResolutionMismatch(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-ensure-resolved"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-ensure"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
	}
	if err := service.EnsureResolvedTicket(ctx, cmd); err != nil {
		t.Fatalf("first ensure resolved ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
		Payload:      workorcheventpayload.Payload{},
	}); err != nil {
		t.Fatalf("second ensure resolved ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-ensure-other"),
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution id mismatch err = %v, want conflict", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeRejectCurrentPath,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution type mismatch err = %v, want conflict", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
		Payload:      mustDecisionPayload(t, map[string]any{"operator": "other"}),
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution payload mismatch err = %v, want conflict", err)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get persisted resolution: %v", err)
	}
	if resolution.ID != cmd.ResolutionID {
		t.Fatalf("persisted resolution id = %q, want %q", resolution.ID, cmd.ResolutionID)
	}
}

func TestService_EnsureResolvedTicketReusesLoadedTicket(t *testing.T) {
	ctx := context.Background()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-ensure-loaded"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-ensure-loaded"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"operator": "workorch"}),
	}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}
	if got := ticketRepo.getCount.Load(); got != 1 {
		t.Fatalf("ticket Get count = %d, want one read reused for transition", got)
	}
}

func TestService_EnsureResolvedTicketAndReturnReturnsResolvedEntitiesWithoutExtraRead(t *testing.T) {
	ctx := context.Background()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := newCountingDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-return-loaded"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-return-loaded"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"operator": "workorch"}),
	}
	ticket, resolution, err := service.EnsureResolvedTicketAndReturn(ctx, cmd)
	if err != nil {
		t.Fatalf("ensure resolved ticket and return: %v", err)
	}
	if ticket == nil || resolution == nil {
		t.Fatalf("ticket/resolution = %v/%v, want returned entities", ticket, resolution)
	}
	if ticket.ID != createCmd.TicketID || ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket = %+v, want resolved %s", ticket, createCmd.TicketID)
	}
	operator, _ := resolution.Payload.String("operator")
	if resolution.ID != cmd.ResolutionID || resolution.Type != cmd.Type || operator != "workorch" {
		t.Fatalf("resolution = %+v, want command payload", resolution)
	}
	if got := ticketRepo.getCount.Load(); got != 1 {
		t.Fatalf("ticket Get count = %d, want one read reused for response", got)
	}
	// orphan-recovery path: GetByTicketID runs once per call before the
	// resolution is created, then the response reuses that result.
	if got := resolutionRepo.getByTicketIDCount.Load(); got != 1 {
		t.Fatalf("resolution GetByTicketID count = %d, want one orphan-check read", got)
	}
}

func TestService_EnsureResolvedTicketAndReturnIdempotentReturnsPersistedResolution(t *testing.T) {
	ctx := context.Background()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := newCountingDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-return-idempotent"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-return-idempotent"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
	}
	if _, _, err := service.EnsureResolvedTicketAndReturn(ctx, cmd); err != nil {
		t.Fatalf("first ensure resolved ticket and return: %v", err)
	}
	ticket, resolution, err := service.EnsureResolvedTicketAndReturn(ctx, cmd)
	if err != nil {
		t.Fatalf("second ensure resolved ticket and return: %v", err)
	}
	if ticket == nil || ticket.State != common.DecisionTicketStateResolved || resolution == nil || resolution.ID != cmd.ResolutionID {
		t.Fatalf("ticket/resolution = %+v/%+v, want idempotent persisted entities", ticket, resolution)
	}
	if got := ticketRepo.getCount.Load(); got != 2 {
		t.Fatalf("ticket Get count = %d, want one per ensure call", got)
	}
	// orphan-recovery: first call reads once (not found), then creates;
	// second call reads once (found) and reuses the result. Total = 2.
	if got := resolutionRepo.getByTicketIDCount.Load(); got != 2 {
		t.Fatalf("resolution GetByTicketID count = %d, want one orphan-check per call", got)
	}
}

// TestService_EnsureResolvedTicketAndReturn_OrphanResolutionSamePayloadIsIdempotent
// verifies the recovery path for an orphan resolution: a resolution file
// exists on disk (GetByTicketID returns it) but the ticket is still in
// "open" state (e.g. an operator manually reset the run back to
// blocked_on_decision without deleting the resolution). When the new
// request matches the persisted resolution on ID/Type/Payload, the call
// is accepted idempotently and the ticket state is synced to "resolved"
// so a subsequent ResumeRun can pass the ticket.State check.
func TestService_EnsureResolvedTicketAndReturn_OrphanResolutionSamePayloadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := newCountingDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-orphan-recover"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	// Persist a resolution directly without going through resolveTicketItem,
	// so the ticket stays in "open" state. This is the orphan shape.
	orphan := &decisionModel.Resolution{
		ID:                 common.ResolutionID("resolution-orphan-recover"),
		TicketID:           createCmd.TicketID,
		Type:               decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:            mustDecisionPayload(t, map[string]any{"actor": "operator", "intent": "retry_same_step"}),
		ContinuationAction: "resume",
		ResumeRunID:        "run-1",
	}
	if err := resolutionRepo.inner.Create(ctx, orphan); err != nil {
		t.Fatalf("seed orphan resolution: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID:       orphan.ID,
		TicketID:           createCmd.TicketID,
		Type:               decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:            mustDecisionPayload(t, map[string]any{"actor": "operator", "intent": "retry_same_step"}),
		ContinuationAction: orphan.ContinuationAction,
		ResumeRunID:        orphan.ResumeRunID,
	}
	ticket, resolution, err := service.EnsureResolvedTicketAndReturn(ctx, cmd)
	if err != nil {
		t.Fatalf("orphan recovery: %v", err)
	}
	if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket = %+v, want state resolved after orphan recovery", ticket)
	}
	if resolution == nil || resolution.ID != orphan.ID {
		t.Fatalf("resolution = %+v, want persisted orphan %q", resolution, orphan.ID)
	}
	if resolution.ContinuationAction != "resume" || resolution.ResumeRunID != "run-1" {
		t.Fatalf("resolution recovery refs = %+v, want resume/run-1", resolution)
	}

	// Ticket state should have been synced to resolved.
	persisted, err := ticketRepo.inner.Get(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if persisted.State != common.DecisionTicketStateResolved {
		t.Fatalf("persisted ticket state = %q, want resolved", persisted.State)
	}
}

func TestService_EnsureResolvedTicketAndReturn_OrphanResolutionDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-orphan-event"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	orphan := &decisionModel.Resolution{
		ID:       common.ResolutionID("resolution-orphan-event"),
		TicketID: createCmd.TicketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  mustDecisionPayload(t, map[string]any{"actor": "operator"}),
	}
	if err := resolutionRepo.Create(ctx, orphan); err != nil {
		t.Fatalf("seed orphan resolution: %v", err)
	}

	if _, _, err := service.EnsureResolvedTicketAndReturn(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: orphan.ID,
		TicketID:     createCmd.TicketID,
		Type:         orphan.Type,
		Payload:      mustDecisionPayload(t, map[string]any{"actor": "operator"}),
	}); err != nil {
		t.Fatalf("orphan recovery: %v", err)
	}

	if published := hub.Posted(); len(published) != 0 {
		t.Fatalf("service unexpectedly published %d events", len(published))
	}
}

func TestService_EnsureResolvedTicketAndReturn_PromotesRecoveryRefsFromPayload(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	service := NewService(ticketRepo, resolutionRepo)

	cmd := decisionservice.CreateTicketCommand{
		TicketID:    "ticket-resolution-refs",
		WorkspaceID: "ws-refs",
		RunID:       "run-source",
		Reason:      "continue selected path",
	}
	if err := service.CreateTicket(ctx, cmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	_, resolution, err := service.EnsureResolvedTicketAndReturn(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: "resolution-refs",
		TicketID:     cmd.TicketID,
		WorkspaceID:  cmd.WorkspaceID,
		RunID:        cmd.RunID,
		Type:         decisioncommon.ResolutionTypeReplan,
		Payload: mustDecisionPayload(t, map[string]any{
			"continuationAction": "replan",
			"resumeRunId":        "run-source",
			"successorRunId":     "run-successor",
		}),
	})
	if err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if resolution.ContinuationAction != "replan" || resolution.ResumeRunID != "run-source" || resolution.SuccessorRunID != "run-successor" {
		t.Fatalf("resolution refs = %+v, want promoted payload refs", resolution)
	}
	if published := hub.Posted(); len(published) != 0 {
		t.Fatalf("service unexpectedly published %d events", len(published))
	}
}

// TestService_EnsureResolvedTicketAndReturn_OrphanResolutionDifferentPayloadReturnsConflict
// covers the "operator tried to change the decision" case: an orphan
// resolution exists, but the new request's payload differs. The fix must
// not silently accept a different decision.
func TestService_EnsureResolvedTicketAndReturn_OrphanResolutionDifferentPayloadReturnsConflict(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-orphan-payload-mismatch"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	orphan := &decisionModel.Resolution{
		ID:       common.ResolutionID("resolution-orphan-payload-mismatch"),
		TicketID: createCmd.TicketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  mustDecisionPayload(t, map[string]any{"actor": "operator", "intent": "retry_same_step"}),
	}
	if err := resolutionRepo.Create(ctx, orphan); err != nil {
		t.Fatalf("seed orphan resolution: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: orphan.ID,
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"actor": "operator", "intent": "terminate"}),
	}
	if _, _, err := service.EnsureResolvedTicketAndReturn(ctx, cmd); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("orphan + different payload err = %v, want conflict", err)
	}

	// Ticket state must remain open (orphan recovery did not run).
	persisted, err := ticketRepo.Get(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if persisted.State != common.DecisionTicketStateOpen {
		t.Fatalf("persisted ticket state = %q, want unchanged open", persisted.State)
	}
}

// TestService_EnsureResolvedTicketAndReturn_OrphanResolutionDifferentIDReturnsConflict
// covers the "operator supplies a different ResolutionID" case: the new
// request claims a different resolution ID than the persisted orphan.
// Must return ErrConflict so the operator knows to clean up the orphan
// first.
func TestService_EnsureResolvedTicketAndReturn_OrphanResolutionDifferentIDReturnsConflict(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-orphan-id-mismatch"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	orphan := &decisionModel.Resolution{
		ID:       common.ResolutionID("resolution-orphan-id-mismatch"),
		TicketID: createCmd.TicketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  mustDecisionPayload(t, map[string]any{"actor": "operator"}),
	}
	if err := resolutionRepo.Create(ctx, orphan); err != nil {
		t.Fatalf("seed orphan resolution: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-different-id"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"actor": "operator"}),
	}
	if _, _, err := service.EnsureResolvedTicketAndReturn(ctx, cmd); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("orphan + different resolutionID err = %v, want conflict", err)
	}
}

func TestService_ResolveTicket_ValidateArgumentsAndNotFound(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository())

	tests := []decisionservice.ResolveTicketCommand{
		{ResolutionID: "", TicketID: common.TicketID("ticket-1"), Type: decisioncommon.ResolutionTypeProvideMissingInfo},
		{ResolutionID: common.ResolutionID("resolution-1"), TicketID: "", Type: decisioncommon.ResolutionTypeProvideMissingInfo},
		{ResolutionID: common.ResolutionID("resolution-1"), TicketID: common.TicketID("ticket-1"), Type: ""},
	}

	for _, tt := range tests {
		if err := service.ResolveTicket(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}

	if err := service.ResolveTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-missing"),
		TicketID:     common.TicketID("missing"),
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing ticket err = %v, want not found", err)
	}
}

func TestService_CreateAndResolveTicket_DoesNotPublishEvents(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-event"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/manual-gate"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := service.ResolveTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-event"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionPayload(t, map[string]any{"operator": "workorch"}),
	}); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}

	if published := hub.Posted(); len(published) != 0 {
		t.Fatalf("service unexpectedly published %d events", len(published))
	}
}

func TestService_DoesNotPublishConcreteDecisionNotifications(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-async"),
		WorkspaceID: common.WorkspaceID("ws-async"),
		RunID:       common.RunID("run-async"),
		StepID:      common.StepID("run-async/manual-gate"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := service.ResolveTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-async"),
		TicketID:     createCmd.TicketID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}

	if published := hub.Posted(); len(published) != 0 {
		t.Fatalf("service unexpectedly published %d events", len(published))
	}
}

func TestBiz_PublishesDecisionNotificationsAndRecordsEvidence(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := newRecordingDecisionHub(t)
	owner, err := New(hub, nil, Dependencies{Tickets: ticketRepo, Resolutions: resolutionRepo, EventHub: hub})
	if err != nil {
		t.Fatalf("New Biz: %v", err)
	}
	create := decisionservice.CreateTicketCommand{
		TicketID: "ticket-biz-notification", WorkspaceID: "ws-biz", RunID: "run-biz", StepID: "run-biz/gate", Reason: "need a decision",
	}
	ticket, err := owner.service.CreateTicketAndReturn(ctx, create)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := owner.publishTicketCreated(ctx, ticket); err != nil {
		t.Fatalf("publish created: %v", err)
	}
	ticket, resolution, transitioned, err := owner.service.EnsureResolvedTicketAndReturnWithTransition(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: "resolution-biz-notification", TicketID: create.TicketID, WorkspaceID: create.WorkspaceID, RunID: create.RunID, StepID: create.StepID,
		Type: decisioncommon.ResolutionTypeProvideMissingInfo, Payload: mustDecisionPayload(t, map[string]any{"operatorInput": "continue"}),
	})
	if err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if err := owner.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
		t.Fatalf("record and publish resolved: %v", err)
	}
	_, _, transitioned, err = owner.service.EnsureResolvedTicketAndReturnWithTransition(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: resolution.ID, TicketID: create.TicketID, WorkspaceID: create.WorkspaceID, RunID: create.RunID, StepID: create.StepID,
		Type: resolution.Type, Payload: resolution.Payload.Clone(),
	})
	if err != nil {
		t.Fatalf("retry resolve ticket: %v", err)
	}
	if transitioned {
		t.Fatal("idempotent resolution unexpectedly transitioned ticket")
	}
	if err := owner.recordAndPublishResolvedTicket(ctx, ticket, resolution, transitioned); err != nil {
		t.Fatalf("record idempotent evidence: %v", err)
	}
	published := hub.Posted()
	if len(published) != 2 || published[0].ID() != decisionevents.TopicTicketCreated || published[1].ID() != decisionevents.TopicTicketResolved {
		t.Fatalf("published = %+v, want one created and one resolved notification", published)
	}
}
