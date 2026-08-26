package approval

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestApprovalWorkspaceDeletionPurgeUsesOwnerPrivateDispatcher(t *testing.T) {
	source, err := os.ReadFile("workspace_deletion_internal_event.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	adapter := approvalDeletionFunctionBody(t, text,
		"func (m *Biz) executePurgeWorkspaceCommand(",
		"func (m *Biz) applyWorkspaceDeletionInternalEvent(")
	for _, sink := range []string{"WithWorkspacePurgeFence", "m.purgeWorkspaceData"} {
		if strings.Contains(adapter, sink) {
			t.Fatalf("Approval deletion command adapter directly invokes physical sink %s", sink)
		}
	}
	if !strings.Contains(adapter, "m.dispatchWorkspaceDeletionInternalEvent") ||
		!strings.Contains(adapter, "workspaceDeletionInternalEventPurgeRequested") {
		t.Fatal("Approval deletion command adapter does not translate purge into the owner-private dispatcher")
	}

	dispatcher := approvalDeletionFunctionBody(t, text,
		"func (m *Biz) dispatchWorkspaceDeletionInternalEvent(",
		"func reduceWorkspaceDeletionInternalEvent(")
	for _, sink := range []string{"WithWorkspacePurgeFence", "m.purgeWorkspaceData"} {
		if count := strings.Count(dispatcher, sink); count != 1 {
			t.Fatalf("Approval owner-private dispatcher sink %s count=%d, want exactly one", sink, count)
		}
	}
}

func TestApprovalWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewApprovalOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-approval-private-purge")
	runID := common.RunID("run-approval-private-purge")
	seedResolvedApprovalForDeletionDelivery(t, ctx, repositories, workspaceID, runID, "approval-private-purge")

	hub := magicevent.NewHub(16)
	owner, err := New(hub, nil, Dependencies{
		Approvals: repositories.Approvals, WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: hub,
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

	forgedCommand := approvalevents.PurgeWorkspaceApprovalsCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
	forgedEvent := magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandPurgeWorkspace, "application/workorchd", approvalevents.CommandDestination,
		magicevent.NewHeader(), bound, forgedCommand,
	)
	if _, err := owner.executePurgeWorkspaceCommand(bound, forgedEvent); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged Approval purge source error=%v, want invalid argument", err)
	}
	if _, err := owner.dispatchWorkspaceDeletionInternalEvent(bound, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged Approval purge_completed error=%v, want conflict", err)
	}
	if count := approvalPhysicalItemCount(t, bound, repositories, scope); count != 1 {
		t.Fatalf("forged Approval purge changed physical approvals: count=%d", count)
	}

	purged, err := owner.dispatchWorkspaceDeletionInternalEvent(bound, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if purged.Phase != approvalmodel.WorkspaceDeletionPhasePurged || purged.PurgeProofHash != approvalevents.StableApprovalDeletionPurgeProofHash(scope) {
		t.Fatalf("Approval private purge result=%+v", purged)
	}
	if count := approvalPhysicalItemCount(t, bound, repositories, scope); count != 0 {
		t.Fatalf("Approval private purge retained physical approvals: count=%d", count)
	}
}

func approvalDeletionFunctionBody(t *testing.T, source, start, end string) string {
	t.Helper()
	startIndex := strings.Index(source, start)
	endIndex := strings.Index(source, end)
	if startIndex < 0 || endIndex <= startIndex {
		t.Fatalf("Approval deletion source boundaries are missing: start=%q end=%q", start, end)
	}
	return source[startIndex:endIndex]
}

func approvalPhysicalItemCount(t *testing.T, ctx context.Context, repositories memory.ApprovalOwnerRepositorySet, scope deletioncontract.Scope) int {
	t.Helper()
	count := -1
	err := repositories.WorkspaceDeletions.WithWorkspacePurgeFence(ctx, scope, func(guarded context.Context) error {
		items, err := repositories.Approvals.ListByWorkspaceID(guarded, scope.WorkspaceID)
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

type approvalDeletionDeliveryProbe struct {
	magicevent.Hub
	mu         sync.Mutex
	sent       []string
	posted     []string
	blockTopic string
	entered    chan struct{}
	release    chan struct{}
	enterOnce  sync.Once
}

func (h *approvalDeletionDeliveryProbe) Send(event magicevent.Event) magicevent.Result {
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

func (h *approvalDeletionDeliveryProbe) Post(event magicevent.Event) {
	if event != nil && event.Source() == ModuleID {
		h.mu.Lock()
		h.posted = append(h.posted, event.ID())
		h.mu.Unlock()
	}
	h.Hub.Post(event)
}

func (h *approvalDeletionDeliveryProbe) counts(topic string) (sent int, posted int) {
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

func TestApprovalDeletionSealLinearizesWithReconciliationDeliveryWithoutDeadlock(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewApprovalOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-approval-delivery-race")
	runID := common.RunID("run-approval-delivery-race")
	approvalID := common.ApprovalID("approval-delivery-race")
	seedResolvedApprovalForDeletionDelivery(t, ctx, repositories, workspaceID, runID, approvalID)

	rawHub := magicevent.NewHub(64)
	probe := &approvalDeletionDeliveryProbe{
		Hub: rawHub, blockTopic: approvalevents.TopicApprovalCommandRecordEvidence,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	evidence := magicevent.NewSimpleObserver(approvalevents.EvidenceCommandDestination, rawHub)
	evidence.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(approvalevents.RecordApprovalEvidenceCommand)
		result.Set(approvalevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			ApprovalID: command.ApprovalID, ResolutionID: command.ResolutionID, ApprovalState: command.ApprovalState,
		}, nil)
	})
	resolutionRecord := magicevent.NewSimpleObserver(approvalevents.ResolutionRecordCommandDestination, rawHub)
	resolutionRecord.Subscribe(approvalevents.TopicApprovalCommandRecordResolution, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(approvalevents.RecordApprovalResolutionCommand)
		data := command.Data
		result.Set(approvalevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	owner, err := New(probe, nil, Dependencies{
		Approvals: repositories.Approvals, WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		owner.Teardown(context.Background())
		evidence.Unsubscribe(approvalevents.TopicApprovalCommandRecordEvidence)
		resolutionRecord.Unsubscribe(approvalevents.TopicApprovalCommandRecordResolution)
		rawHub.Terminate(context.Background())
	})

	reconcileDone := make(chan error, 1)
	go func() { reconcileDone <- owner.ReconcileApprovalDeliveries(ctx) }()
	waitApprovalDeliverySignal(t, probe.entered, "reconciliation evidence delivery")

	sealDone := make(chan error, 1)
	go func() { sealDone <- sendApprovalDeletionSeal(ctx, probe, approvalDeletionRetainCommand(workspaceID)) }()
	select {
	case err := <-sealDone:
		t.Fatalf("deletion seal crossed active reconciliation delivery: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(probe.release)
	waitApprovalDeliveryResult(t, reconcileDone, "reconciliation")
	waitApprovalDeliveryResult(t, sealDone, "deletion seal")

	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordEvidence); sent != 1 {
		t.Fatalf("Approval evidence deliveries = %d, want exactly the pre-seal delivery", sent)
	}
	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordResolution); sent != 1 {
		t.Fatalf("Approval Eventstream records = %d, want exactly the pre-seal record", sent)
	}
	if _, posted := probe.counts(approvalevents.TopicApprovalResolved); posted != 1 {
		t.Fatalf("Approval resolved notifications = %d, want exactly the pre-seal notification", posted)
	}
	if err := owner.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatalf("post-seal reconciliation: %v", err)
	}
	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordEvidence); sent != 1 {
		t.Fatalf("post-seal Approval evidence count = %d, want 1", sent)
	}
	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordResolution); sent != 1 {
		t.Fatalf("post-seal Approval Eventstream record count = %d, want 1", sent)
	}
	if _, posted := probe.counts(approvalevents.TopicApprovalResolved); posted != 1 {
		t.Fatalf("post-seal Approval notification count = %d, want 1", posted)
	}
}

func TestApprovalRestartSkipsLateResolvedDeliveryAfterDeletionSeal(t *testing.T) {
	ctx := context.Background()
	repositories := memory.NewApprovalOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-approval-late-delivery")
	runID := common.RunID("run-approval-late-delivery")
	approvalID := common.ApprovalID("approval-late-delivery")
	item := seedResolvedApprovalForDeletionDelivery(t, ctx, repositories, workspaceID, runID, approvalID)

	firstRaw := magicevent.NewHub(16)
	first, err := New(firstRaw, nil, Dependencies{
		Approvals: repositories.Approvals, WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: firstRaw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sendApprovalDeletionSeal(ctx, firstRaw, approvalDeletionRetainCommand(workspaceID)); err != nil {
		t.Fatal(err)
	}
	first.Teardown(ctx)
	firstRaw.Terminate(ctx)

	restartedRaw := magicevent.NewHub(16)
	probe := &approvalDeletionDeliveryProbe{Hub: restartedRaw}
	restarted, err := New(probe, nil, Dependencies{
		Approvals: repositories.Approvals, WorkspaceDeletions: repositories.WorkspaceDeletions, EventHub: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restartedRaw.Terminate(context.Background()) })

	if err := restarted.deliverResolvedApproval(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), item); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("late direct Approval delivery err = %v, want deletion fence", err)
	}
	if err := restarted.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatalf("restarted reconciliation: %v", err)
	}
	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordEvidence); sent != 0 {
		t.Fatalf("restarted Approval evidence deliveries = %d, want 0", sent)
	}
	if sent, _ := probe.counts(approvalevents.TopicApprovalCommandRecordResolution); sent != 0 {
		t.Fatalf("restarted Approval Eventstream records = %d, want 0", sent)
	}
	if _, posted := probe.counts(approvalevents.TopicApprovalResolved); posted != 0 {
		t.Fatalf("restarted Approval notifications = %d, want 0", posted)
	}
}

func seedResolvedApprovalForDeletionDelivery(t *testing.T, ctx context.Context, repositories memory.ApprovalOwnerRepositorySet, workspaceID common.WorkspaceID, runID common.RunID, approvalID common.ApprovalID) *approvalmodel.Approval {
	t.Helper()
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	service := newMutationService(repositories.Approvals)
	if err := service.CreateApproval(bound, approvalcontracts.CreateApprovalCommand{
		ApprovalID: approvalID, WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/delivery-race", Reason: "late approval delivery",
	}); err != nil {
		t.Fatal(err)
	}
	item, err := service.EnsureApprovedAndReturn(bound, approvalcontracts.ResolveApprovalCommand{
		ApprovalID: approvalID, WorkspaceID: workspaceID, RunID: runID,
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "test-operator",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func approvalDeletionRetainCommand(workspaceID common.WorkspaceID) approvalevents.SealWorkspaceApprovalDeletionCommand {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return approvalevents.SealWorkspaceApprovalDeletionCommand{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
	}
}

func sendApprovalDeletionSeal(ctx context.Context, hub magicevent.Hub, command approvalevents.SealWorkspaceApprovalDeletionCommand) error {
	result := hub.Send(magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandSealWorkspaceDeletion, workspaceDeletionCommandSource,
		approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command,
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

func waitApprovalDeliverySignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitApprovalDeliveryResult(t *testing.T, result <-chan error, name string) {
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
