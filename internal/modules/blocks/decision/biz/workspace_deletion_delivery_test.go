package decision

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestDecisionWorkspaceDeletionPurgeUsesOwnerPrivateDispatcher(t *testing.T) {
	source, err := os.ReadFile("workspace_deletion_internal_event.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	adapter := decisionDeletionFunctionBody(t, text,
		"func (m *Biz) executePurgeWorkspaceCommand(",
		"func (m *Biz) applyWorkspaceDeletionInternalEvent(")
	for _, sink := range []string{"WithWorkspacePurgeFence", "m.purgeWorkspaceData"} {
		if strings.Contains(adapter, sink) {
			t.Fatalf("Decision deletion command adapter directly invokes physical sink %s", sink)
		}
	}
	if !strings.Contains(adapter, "m.dispatchWorkspaceDeletionInternalEvent") ||
		!strings.Contains(adapter, "workspaceDeletionInternalEventPurgeRequested") {
		t.Fatal("Decision deletion command adapter does not translate purge into the owner-private dispatcher")
	}

	dispatcher := decisionDeletionFunctionBody(t, text,
		"func (m *Biz) dispatchWorkspaceDeletionInternalEvent(",
		"func reduceWorkspaceDeletionInternalEvent(")
	for _, sink := range []string{"WithWorkspacePurgeFence", "m.purgeWorkspaceData"} {
		if count := strings.Count(dispatcher, sink); count != 1 {
			t.Fatalf("Decision owner-private dispatcher sink %s count=%d, want exactly one", sink, count)
		}
	}
}

func TestDecisionWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewDecisionOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-decision-private-purge")
	runID := common.RunID("run-decision-private-purge")
	seedResolvedDecisionForDeletionDelivery(t, ctx, repositories, workspaceID, runID, "ticket-decision-private-purge")

	hub := magicevent.NewHub(16)
	owner, err := New(hub, nil, Dependencies{
		Tickets: repositories.Tickets, Resolutions: repositories.Resolutions,
		WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: hub,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	current, err := owner.applyWorkspaceDeletionInternalEvent(bound, nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	forgedCommand := decisionevents.PurgeWorkspaceDecisionsCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	forgedEvent := magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandPurgeWorkspace, "application/workorchd", decisionevents.CommandDestination,
		magicevent.NewHeader(), bound, forgedCommand,
	)
	if _, err := owner.executePurgeWorkspaceCommand(bound, forgedEvent); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged Decision purge source error=%v, want invalid argument", err)
	}
	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(bound, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged Decision purge_completed error=%v, want conflict", err)
	}
	if count := decisionPhysicalTicketCount(t, bound, repositories, scope); count != 1 {
		t.Fatalf("forged Decision purge changed physical tickets: count=%d", count)
	}

	purged, err := owner.dispatchWorkspaceDeletionInternalEvent(bound, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if purged.Phase != decisionmodel.WorkspaceDeletionPhasePurged || purged.PurgeProofHash != decisionevents.StableDecisionDeletionPurgeProofHash(scope) {
		t.Fatalf("Decision private purge result=%+v", purged)
	}
	if count := decisionPhysicalTicketCount(t, bound, repositories, scope); count != 0 {
		t.Fatalf("Decision private purge retained physical tickets: count=%d", count)
	}
}

func decisionDeletionFunctionBody(t *testing.T, source, start, end string) string {
	t.Helper()
	startIndex := strings.Index(source, start)
	endIndex := strings.Index(source, end)
	if startIndex < 0 || endIndex <= startIndex {
		t.Fatalf("Decision deletion source boundaries are missing: start=%q end=%q", start, end)
	}
	return source[startIndex:endIndex]
}

func decisionPhysicalTicketCount(t *testing.T, ctx context.Context, repositories memory.DecisionOwnerRepositorySet, scope deletioncontract.Scope) int {
	t.Helper()
	count := -1
	err := repositories.WorkspaceDeletions.WithWorkspacePurgeFence(ctx, scope, func(guarded context.Context) error {
		items, err := repositories.Tickets.ListByWorkspaceID(guarded, scope.WorkspaceID)
		if err != nil {
			return err
		}
		count = len(items)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

type decisionDeletionDeliveryProbe struct {
	magicevent.Hub
	mu         sync.Mutex
	sent       []string
	posted     []string
	blockTopic string
	entered    chan struct{}
	release    chan struct{}
	enterOnce  sync.Once
}

func (h *decisionDeletionDeliveryProbe) Send(event magicevent.Event) magicevent.Result {
	if event != nil && event.Source() == ModuleID {
		h.mu.Lock()
		h.sent = append(h.sent, event.ID())
		h.mu.Unlock()
		if event.ID() == h.blockTopic && h.entered != nil && h.release != nil {
			h.enterOnce.Do(func() { close(h.entered) })
			<-h.release
		}
	}
	return h.Hub.Send(event)
}

func (h *decisionDeletionDeliveryProbe) Post(event magicevent.Event) {
	if event != nil && event.Source() == ModuleID {
		h.mu.Lock()
		h.posted = append(h.posted, event.ID())
		h.mu.Unlock()
	}
	h.Hub.Post(event)
}

func (h *decisionDeletionDeliveryProbe) counts(topic string) (sent int, posted int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, current := range h.sent {
		if current == topic {
			sent++
		}
	}
	for _, current := range h.posted {
		if current == topic {
			posted++
		}
	}
	return sent, posted
}

func TestDecisionDeletionSealLinearizesWithReconciliationDelivery(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewDecisionOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-decision-delivery-race")
	ticketID := common.TicketID("ticket-decision-delivery-race")
	runID := common.RunID("run-decision-delivery-race")
	seedResolvedDecisionForDeletionDelivery(t, ctx, repositories, workspaceID, runID, ticketID)

	rawHub := magicevent.NewHub(64)
	probe := &decisionDeletionDeliveryProbe{
		Hub: rawHub, blockTopic: decisionevents.TopicDecisionCommandRecordEvidence,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	evidence := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	evidence.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(decisionevents.RecordDecisionEvidenceCommand)
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	owner, err := New(probe, nil, Dependencies{
		Tickets: repositories.Tickets, Resolutions: repositories.Resolutions,
		WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		owner.Teardown(context.Background())
		evidence.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
		rawHub.Terminate(context.Background())
	})

	reconcileDone := make(chan error, 1)
	go func() { reconcileDone <- owner.ReconcileRuntimeGovernanceProposals(ctx) }()
	waitDecisionDeliverySignal(t, probe.entered, "reconciliation evidence delivery")

	sealDone := make(chan error, 1)
	go func() { sealDone <- sendDecisionDeletionSeal(ctx, probe, decisionDeletionRetainCommand(workspaceID)) }()
	select {
	case err := <-sealDone:
		t.Fatalf("deletion seal crossed active reconciliation delivery: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(probe.release)
	waitDecisionDeliveryResult(t, reconcileDone, "reconciliation")
	waitDecisionDeliveryResult(t, sealDone, "deletion seal")

	if sent, _ := probe.counts(decisionevents.TopicDecisionCommandRecordEvidence); sent != 1 {
		t.Fatalf("Decision evidence deliveries = %d, want exactly the pre-seal delivery", sent)
	}
	if _, posted := probe.counts(decisionevents.TopicTicketResolved); posted != 1 {
		t.Fatalf("Decision resolved notifications = %d, want exactly the pre-seal notification", posted)
	}
	if err := owner.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("post-seal reconciliation: %v", err)
	}
	if sent, _ := probe.counts(decisionevents.TopicDecisionCommandRecordEvidence); sent != 1 {
		t.Fatalf("post-seal Decision evidence delivery count = %d, want 1", sent)
	}
	if _, posted := probe.counts(decisionevents.TopicTicketResolved); posted != 1 {
		t.Fatalf("post-seal Decision notification count = %d, want 1", posted)
	}
}

func TestDecisionRestartSkipsLateResolvedDeliveryAfterDeletionSeal(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewDecisionOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-decision-late-delivery")
	ticketID := common.TicketID("ticket-decision-late-delivery")
	runID := common.RunID("run-decision-late-delivery")
	ticket, resolution := seedResolvedDecisionForDeletionDelivery(t, ctx, repositories, workspaceID, runID, ticketID)

	firstRaw := magicevent.NewHub(16)
	first, err := New(firstRaw, nil, Dependencies{
		Tickets: repositories.Tickets, Resolutions: repositories.Resolutions,
		WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: firstRaw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sendDecisionDeletionSeal(ctx, firstRaw, decisionDeletionRetainCommand(workspaceID)); err != nil {
		t.Fatal(err)
	}
	first.Teardown(ctx)
	firstRaw.Terminate(ctx)

	restartedRaw := magicevent.NewHub(16)
	probe := &decisionDeletionDeliveryProbe{Hub: restartedRaw}
	restarted, err := New(probe, nil, Dependencies{
		Tickets: repositories.Tickets, Resolutions: repositories.Resolutions,
		WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restartedRaw.Terminate(context.Background()) })

	if err := restarted.recordAndPublishResolvedTicket(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticket, resolution, false); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("late direct Decision delivery err = %v, want deletion fence", err)
	}
	if err := restarted.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("restarted reconciliation: %v", err)
	}
	if sent, _ := probe.counts(decisionevents.TopicDecisionCommandRecordEvidence); sent != 0 {
		t.Fatalf("restarted Decision evidence deliveries = %d, want 0", sent)
	}
	if _, posted := probe.counts(decisionevents.TopicTicketResolved); posted != 0 {
		t.Fatalf("restarted Decision notifications = %d, want 0", posted)
	}
}

func seedResolvedDecisionForDeletionDelivery(t *testing.T, ctx context.Context, repositories memory.DecisionOwnerRepositorySet, workspaceID common.WorkspaceID, runID common.RunID, ticketID common.TicketID) (*decisionmodel.Ticket, *decisionmodel.Resolution) {
	t.Helper()
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	service := newMutationService(repositories.Tickets, repositories.Resolutions)
	if err := service.CreateTicket(bound, decisioncontracts.CreateTicketCommand{
		TicketID: ticketID, WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/delivery-race", Reason: "late delivery",
	}); err != nil {
		t.Fatal(err)
	}
	ticket, resolution, _, err := service.EnsureResolvedTicketAndReturnWithTransition(bound, decisioncontracts.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-" + string(ticketID)), TicketID: ticketID,
		WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/delivery-race", Type: decisioncommon.ResolutionTypeProvideMissingInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ticket, resolution
}

func decisionDeletionRetainCommand(workspaceID common.WorkspaceID) decisionevents.SealWorkspaceDecisionDeletionCommand {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return decisionevents.SealWorkspaceDecisionDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
}

func sendDecisionDeletionSeal(ctx context.Context, hub magicevent.Hub, command decisionevents.SealWorkspaceDecisionDeletionCommand) error {
	result := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandSealWorkspaceDeletion, workspaceDeletionCommandSource,
		decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		return common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	_, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	return nil
}

func waitDecisionDeliverySignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitDecisionDeliveryResult(t *testing.T, result <-chan error, name string) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
