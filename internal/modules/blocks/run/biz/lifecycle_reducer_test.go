package run

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func newLifecycleTestBiz(t *testing.T) (*Biz, *memory.RunRepository, magicevent.Hub) {
	t.Helper()
	runs := memory.NewRunRepository()
	hub := magicevent.NewHub(32)
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := biz.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = biz.Stop(context.Background()) })
	return biz, runs, hub
}

func seedRun(t *testing.T, runs *memory.RunRepository, state common.RunState) *runmodel.Run {
	t.Helper()
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", State: state, CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return run
}

func seedV2TerminalRun(t *testing.T, runs *memory.RunRepository, state common.RunState, action runtimecontract.RuntimeAction, code, summary, authID string) *runmodel.Run {
	t.Helper()
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", State: state, CreatedAt: "2026-01-01T00:00:00Z",
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 3,
			Facts: runtimecontract.RuntimeFacts{
				WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1",
			},
			Decision: runtimecontract.RuntimeDecision{
				Action:            action,
				Terminal:          &runtimecontract.TerminalOutcome{Code: code, Summary: summary},
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: authID},
			},
		},
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	return run
}

func TestReduceActivateRunPendingToActive(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStatePending}
	tr, err := reduceActivateRun(current, runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || tr.Noop || tr.Target != common.RunStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceActivateRunDuplicateIsNoop(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive}
	tr, err := reduceActivateRun(current, runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceActivateRunRejectsWrongScope(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStatePending}
	_, err := reduceActivateRun(current, runevents.ActivateRunCommand{WorkspaceID: "ws-other", RunID: "run-1"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceActivateRunTerminalIsNoop(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted}
	tr, err := reduceActivateRun(current, runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"})
	if err != nil || !tr.Noop {
		t.Fatalf("expected terminal noop, got %+v err=%v", tr, err)
	}
}

func TestReduceContinueWithoutGovernanceFactRejected(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-1",
	}
	_, err := reduceContinueRun(current, runevents.ContinueRunCommand{WorkspaceID: "ws-1", RunID: "run-1", CausationID: "resume-test"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceContinuationEligibleRequiresMatchingTicket(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateBlockedOnDecision,
		BlockingTicketID:       "ticket-1",
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
		RuntimeState:           &runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{DefinitionRevision: "def-1", EvidenceRevision: "evidence-1"}, GovernanceResolutions: []runtimecontract.RuntimeGovernanceResolutionRecord{{ResolutionID: "res-1", TicketID: "ticket-1", AuthoritySnapshotID: "auth-1", DispatchID: "dispatch-1"}}},
	}
	_, err := reduceContinuationEligible(current, runevents.ApplyRuntimeContinuationEligibleCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-other",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	tr, err := reduceContinuationEligible(current, runevents.ApplyRuntimeContinuationEligibleCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-1",
	})
	if err != nil || tr.Target != common.RunStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceGovernanceWaitDuplicateIsNoop(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateBlockedOnDecision,
		BlockingTicketID:       "ticket-1",
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
		RuntimeState:           &runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{DefinitionRevision: "def-1", EvidenceRevision: "evidence-1"}, GovernanceRequests: []runtimecontract.RuntimeGovernanceRequestRecord{{TicketID: "ticket-1", AuthoritySnapshotID: "auth-1", DispatchID: "dispatch-1"}}},
	}
	tr, err := reduceGovernanceWait(current, runevents.ApplyRuntimeGovernanceWaitCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
	})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceTerminalDeliveryIdentityMismatch(t *testing.T) {
	current := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			Decision: runtimecontract.RuntimeDecision{
				Action:            runtimecontract.RuntimeActionComplete,
				Terminal:          &runtimecontract.TerminalOutcome{Code: "completed", Summary: "done"},
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "auth-1"},
			},
			Facts: runtimecontract.RuntimeFacts{DefinitionRevision: "def-1", EvidenceRevision: "evidence-1"},
		},
	}
	_, err := reduceTerminalDelivery(current, runevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-other", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceTerminalDeliveryAfterUnfencedTerminalIsConflict(t *testing.T) {
	current := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted}
	tr, err := reduceTerminalDelivery(current, runevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Action: runtimecontract.RuntimeActionFail, TerminalCode: "failed", TerminalSummary: "no",
	})
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected terminal fence conflict, got %+v err=%v", tr, err)
	}
}

func TestLifecycleHandlerActivateDuplicatePublishesOnce(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	seedRun(t, runs, common.RunStatePending)

	received := make(chan runevents.RunStateChangedData, 4)
	observer := magicevent.NewSimpleObserverWithMatchID("/run/consumers/lifecycle-test", "/run/consumers/lifecycle-test", hub)
	observer.Subscribe(runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(runevents.RunStateChangedData)
		if ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunStateChanged) })

	ctx := context.Background()
	first, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandActivate, "test", runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"},
	))
	if err != nil || !first.Transitioned || first.Run.State != common.RunStateActive {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandActivate, "test", runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"},
	))
	if err != nil || second.Transitioned {
		t.Fatalf("second should be noop: %+v err=%v", second, err)
	}

	select {
	case data := <-received:
		if data.State != common.RunStateActive || data.RunID != "run-1" {
			t.Fatalf("notification=%+v", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected one RunStateChanged notification after CAS success")
	}
	// Drain optional specific-topic duplicate if any fan-in path posts twice.
	deadline := time.After(50 * time.Millisecond)
	drain := 0
	for {
		select {
		case <-received:
			drain++
			if drain > 1 {
				t.Fatalf("too many extra notifications: %d", drain)
			}
		case <-deadline:
			goto drained
		}
	}
drained:

	stored, err := runs.GetByWorkspaceID(ctx, "ws-1", "run-1")
	if err != nil || stored.State != common.RunStateActive {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func TestLifecycleHandlerRejectsConflictingEventWorkspaceScopeWithoutMutation(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedRun(t, runs, common.RunStatePending)

	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-other")
	_, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandActivate, "test", runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"},
	))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v, want workspace scope conflict", err)
	}
	item, err := runs.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("get run after rejected command: %v", err)
	}
	if item.State != common.RunStatePending {
		t.Fatalf("run state=%q, want unchanged pending", item.State)
	}
}

// An SM-2/3/4 aggregate save is a Run-owned persistence operation, not an
// SM-1 lifecycle transition. In particular it must not emit an active fact
// after Workspace has already released a terminal Run.
func TestGenericRunUpdateDoesNotPublishLifecycleFactOrChangeSM1(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	seed := seedRun(t, runs, common.RunStateActive)
	received := make(chan runevents.RunStateChangedData, 1)
	observer := magicevent.NewSimpleObserverWithMatchID("/run/consumers/update-test", "/run/consumers/update-test", hub)
	observer.Subscribe(runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.RunStateChangedData); ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunStateChanged) })

	_, err := biz.executeCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandUpdate, "test", runevents.CommandDestination, magicevent.NewHeader(), context.Background(),
		runevents.UpdateRunCommand{
			RunID: seed.ID, WorkspaceID: seed.WorkspaceID,
		},
	))
	if err != nil {
		t.Fatalf("aggregate-compatible update: %v", err)
	}
	select {
	case data := <-received:
		t.Fatalf("non-lifecycle update emitted RunStateChanged: %+v", data)
	case <-time.After(50 * time.Millisecond):
	}

	// UpdateRunCommand intentionally has no State/blocker/failure/lineage fields;
	// this is the regression assertion that generic persistence cannot express an
	// SM-1 transition.
}

// Runtime V2 aggregate writes are semantically fenced by RuntimeStateRevision,
// not by the enclosing Run record revision. This exercises the physical CAS
// rebase used when an unrelated SM-1/model write wins the repository race.
func TestRuntimeV2AggregatePersistenceRebasesUnrelatedRunModelWrite(t *testing.T) {
	ctx := context.Background()
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "def-v2", GoalID: "goal-v2", DoDID: "dod-v2"}
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: definition, Activation: activation}
	currentState := runtimecontract.RuntimeStateRecord{
		StateRevision: 7, Definition: definition,
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: definition.Revision, Activation: activation},
	}
	base := memory.NewRunRepository()
	if err := base.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}); err != nil {
		t.Fatal(err)
	}
	runs := &runtimeStateCASRaceRepository{RunRepository: base, mutateModelOnly: true}
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(8)}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = biz.Stop(ctx) }()

	nextState := currentState.Clone()
	nextState.StateRevision++
	expected := currentState.StateRevision
	result, err := biz.updateRuntimeV2State(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}, runevents.UpdateRunCommand{RunID: "run-v2", WorkspaceID: "ws-v2", RuntimeState: &nextState, ExpectedRuntimeStateRevision: &expected})
	if err != nil {
		t.Fatalf("Runtime V2 aggregate update: %v", err)
	}
	if result.Run == nil || result.Run.Priority != 17 || result.Run.RuntimeState == nil || result.Run.RuntimeState.StateRevision != nextState.StateRevision {
		t.Fatalf("rebased Run = %+v", result.Run)
	}
	if runs.casCalls != 2 {
		t.Fatalf("physical CAS calls = %d, want 2", runs.casCalls)
	}
}

func TestRuntimeV2AggregatePersistenceRejectsAggregateRevisionRace(t *testing.T) {
	ctx := context.Background()
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "def-v2", GoalID: "goal-v2", DoDID: "dod-v2"}
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: definition, Activation: activation}
	currentState := runtimecontract.RuntimeStateRecord{
		StateRevision: 7, Definition: definition,
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: definition.Revision, Activation: activation},
	}
	base := memory.NewRunRepository()
	if err := base.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}); err != nil {
		t.Fatal(err)
	}
	runs := &runtimeStateCASRaceRepository{RunRepository: base, mutateAggregate: true}
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(8)}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = biz.Stop(ctx) }()
	nextState := currentState.Clone()
	nextState.StateRevision++
	expected := currentState.StateRevision
	_, err := biz.updateRuntimeV2State(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: definition.GoalID, DoDID: definition.DoDID, DefinitionRevision: definition.Revision,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeBootstrap: &bootstrap, RuntimeState: &currentState,
	}, runevents.UpdateRunCommand{RunID: "run-v2", WorkspaceID: "ws-v2", RuntimeState: &nextState, ExpectedRuntimeStateRevision: &expected})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("aggregate revision race err = %v, want conflict", err)
	}
}

type runtimeStateCASRaceRepository struct {
	*memory.RunRepository
	mutateModelOnly bool
	mutateAggregate bool
	casCalls        int
}

func (r *runtimeStateCASRaceRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	r.casCalls++
	if r.casCalls != 1 {
		return r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
	}
	current, err := r.RunRepository.GetByWorkspaceID(ctx, item.WorkspaceID, item.ID)
	if err != nil {
		return err
	}
	changed := *current
	if r.mutateAggregate {
		next := current.RuntimeState.Clone()
		next.StateRevision++
		changed.RuntimeState = &next
	} else if r.mutateModelOnly {
		changed.Priority = 17
	}
	if err := r.RunRepository.CompareAndSwap(ctx, &changed, current.Revision); err != nil {
		return err
	}
	return common.ErrConflict
}

func TestLifecycleHandlerTerminalThenActivateRemainsTerminal(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedV2TerminalRun(t, runs, common.RunStateActive, runtimecontract.RuntimeActionComplete, "completed", "done", "auth-1")
	ctx := context.Background()

	term, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyTerminalDelivery, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
			Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
		},
	))
	if err != nil || !term.Transitioned || term.Run.State != common.RunStateCompleted {
		t.Fatalf("terminal=%+v err=%v", term, err)
	}
	if term.DeliveryAck == nil || term.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal {
		t.Fatalf("ack=%+v", term.DeliveryAck)
	}

	activate, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandActivate, "test", runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"},
	))
	if err != nil || activate.Transitioned || activate.Run.State != common.RunStateCompleted {
		t.Fatalf("activate after terminal=%+v err=%v", activate, err)
	}

	wait, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyGovernanceWait, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ApplyRuntimeGovernanceWaitCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
		},
	))
	if err != nil || wait.Transitioned || wait.Run.State != common.RunStateCompleted {
		t.Fatalf("wait after terminal=%+v err=%v", wait, err)
	}
}

func TestLifecycleHandlerStaleTerminalDoesNotChangeRun(t *testing.T) {
	biz, runs, _ := newLifecycleTestBiz(t)
	seedV2TerminalRun(t, runs, common.RunStateActive, runtimecontract.RuntimeActionComplete, "completed", "done", "auth-1")
	ctx := context.Background()

	_, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyTerminalDelivery, runtimeLifecycleFactSource, runevents.CommandDestination, magicevent.NewHeader(), ctx,
		runevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "terminal-delivery/td-stale", TerminalDeliveryID: "td-stale", AuthoritySnapshotID: "auth-stale", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
			Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
		},
	))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, "ws-1", "run-1")
	if err != nil || stored.State != common.RunStateActive {
		t.Fatalf("stored must remain active: %+v err=%v", stored, err)
	}
}
