package orchestrator

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeTerminalNonterminalReleaseFaultRepository struct {
	delegate   runtime.RuntimeStateRepository
	failBefore bool
	failAfter  bool
	fired      bool
}

func (r *runtimeTerminalNonterminalReleaseFaultRepository) Load(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeTerminalNonterminalReleaseFaultRepository) Save(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	expectedRevision uint64,
) error {
	release := !r.fired && aggregate.Termination == nil && aggregate.Decision.Terminal != nil &&
		aggregate.DriveLease == nil
	if release && r.failBefore {
		r.fired = true
		return errors.New("injected running handoff release failure before commit")
	}
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if release && r.failAfter {
		r.fired = true
		return errors.New("injected running handoff release response loss after commit")
	}
	return nil
}

func runtimeTerminalNonterminalHandoffSubscribeOwners(
	t *testing.T,
	hub magicevent.Hub,
	delivery runtimecontract.RuntimeTerminalDeliveryIdentity,
	causationID string,
) *[3]int {
	t.Helper()
	calls := &[3]int{}
	resultObserver := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	resultObserver.Subscribe(resultevents.TopicResultFactTerminalDelivery, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(resultevents.RuntimeTerminalDeliveryFact)
		if !ok || command.Identity() != delivery || command.CausationID != causationID {
			t.Errorf("unexpected Result terminal fact: %T %+v", event.Data(), event.Data())
			result.Set(nil, nil)
			return
		}
		calls[0]++
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: delivery.WorkspaceID, RunID: delivery.RunID,
			ResultID:           common.ResultID(string(delivery.WorkspaceID) + "/" + string(delivery.RunID) + "/result/dod-assessment"),
			TerminalDeliveryID: delivery.TerminalDeliveryID, Owner: runtimecontract.RuntimeDeliveryOwnerResult,
			AcknowledgedState:   runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded,
			AuthoritySnapshotID: delivery.AuthoritySnapshotID, CausationID: causationID, Outcome: delivery.Outcome,
		}
		result.Set(resultevents.ResultLifecycleCommandResult{DeliveryAck: &ack}, nil)
	})

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceFactTerminalDelivery, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(workspaceevents.RuntimeTerminalDeliveryFact)
		if !ok || command.Identity() != delivery || command.CausationID != causationID {
			t.Errorf("unexpected Workspace terminal fact: %T %+v", event.Data(), event.Data())
			result.Set(nil, nil)
			return
		}
		calls[1]++
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: delivery.WorkspaceID, RunID: delivery.RunID,
			TerminalDeliveryID: delivery.TerminalDeliveryID, Owner: runtimecontract.RuntimeDeliveryOwnerWorkspace,
			AcknowledgedState:   runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased,
			AuthoritySnapshotID: delivery.AuthoritySnapshotID, CausationID: causationID, Outcome: delivery.Outcome,
		}
		result.Set(workspaceevents.WorkspaceLifecycleCommandResult{DeliveryAck: &ack}, nil)
	})

	runObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	runObserver.Subscribe(runevents.TopicRunFactTerminalDelivery, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.RuntimeTerminalDeliveryFact)
		if !ok || command.Identity() != delivery || command.CausationID != causationID {
			t.Errorf("unexpected Run terminal fact: %T %+v", event.Data(), event.Data())
			result.Set(nil, nil)
			return
		}
		calls[2]++
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: delivery.WorkspaceID, RunID: delivery.RunID,
			TerminalDeliveryID: delivery.TerminalDeliveryID, Owner: runtimecontract.RuntimeDeliveryOwnerRun,
			AcknowledgedState:   runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal,
			AuthoritySnapshotID: delivery.AuthoritySnapshotID, CausationID: causationID, Outcome: delivery.Outcome,
		}
		result.Set(runevents.RunLifecycleCommandResult{DeliveryAck: &ack}, nil)
	})

	t.Cleanup(func() {
		resultObserver.Unsubscribe(resultevents.TopicResultFactTerminalDelivery)
		workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceFactTerminalDelivery)
		runObserver.Unsubscribe(runevents.TopicRunFactTerminalDelivery)
	})
	return calls
}

func runtimeTerminalNonterminalHandoffFixture(
	t *testing.T,
	ctx context.Context,
	module *Biz,
	aggregate runtime.RuntimeAggregate,
) (runtime.RuntimeAggregate, runtimecontract.RuntimeDriveLease) {
	t.Helper()
	cause := &runtime.RuntimeCASConflictError{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ExpectedRevision: aggregate.StateRevision, ObservedRevision: aggregate.StateRevision + 1,
	}
	request, err := runtime.NewRuntimeNonTerminalRecoveryRequest(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		0,
		aggregate.StateRevision,
		"2026-09-04T12:00:00Z",
		cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := module.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &request,
			NonTerminalRecoveryCause:   cause,
		},
	})
	if err != nil || scheduled.DriveLease == nil || scheduled.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		t.Fatalf("schedule exact nonterminal recovery: result=%+v err=%v", scheduled, err)
	}
	running, claimed, err := module.runtimeClaimDriveLease(ctx, scheduled.DriveLease.Clone(), scheduled.DriveLease.DueAt)
	if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim exact nonterminal recovery: lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	claimedAggregate, err := module.loadRuntime(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := module.runtimeRecordFailure(ctx, claimedAggregate, runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed",
		Signature: "terminal-running-nonterminal-recovery", Message: "authority transaction mismatch",
	})
	if err != nil {
		t.Fatalf("persist convergence terminal under exact running nonterminal lease: %v", err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
		terminal.Termination != nil || terminal.DriveLease == nil || terminal.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(*terminal.DriveLease, running) ||
		runtimecontract.ValidateRuntimeNonTerminalRecoveryLeaseBinding(terminal.Facts.Recovery, *terminal.DriveLease) != nil {
		t.Fatalf("terminal handoff did not preserve exact running authority: terminal=%+v running=%+v", terminal, running)
	}
	return terminal, running
}

func TestRuntimeRunningNonterminalRecoveryTerminalWorkerExitAndRestartMatrix(t *testing.T) {
	for _, test := range []struct {
		name       string
		failBefore bool
		failAfter  bool
	}{
		{name: "normal release"},
		{name: "release failure enters first delivery with running lease", failBefore: true},
		{name: "release response loss reloads committed release", failAfter: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			initial := runtimeAggregate(t, 1)
			base := &inMemoryRuntimeStateRepository{aggregate: &initial}
			fault := &runtimeTerminalNonterminalReleaseFaultRepository{
				delegate: base, failBefore: test.failBefore, failAfter: test.failAfter,
			}
			owner := newRuntimeTestOwner()
			if err := owner.ConfigureRuntimeRepository(fault); err != nil {
				t.Fatal(err)
			}
			background := &recordingBackgroundRoutine{}
			module := newTestBiz(owner, hub, background)
			terminal, running := runtimeTerminalNonterminalHandoffFixture(t, ctx, module, initial)
			delivery, err := terminalDeliveryRequest(terminal)
			if err != nil {
				t.Fatal(err)
			}
			calls := runtimeTerminalNonterminalHandoffSubscribeOwners(t, hub, delivery.Identity(), delivery.CausationID)
			if !test.failBefore {
				// The ordinary scheduled worker projects its newly committed terminal
				// before exit. Keeping the running lease through all acknowledgement
				// CASes is part of the same bounded handoff.
				if err := module.deliverRuntimeTerminal(ctx, terminal); err != nil {
					t.Fatalf("deliver terminal before worker exit: %v", err)
				}
			}
			key := runtimeDriveKey(terminal.Facts.WorkspaceID, terminal.Facts.RunID)
			if !module.beginRuntimeDrive(key) {
				t.Fatal("terminal handoff worker did not acquire gate")
			}
			gate, ok := module.currentRuntimeDriveGate(key)
			if !ok {
				t.Fatal("terminal handoff gate identity is unavailable")
			}
			var retry *runtimeRecoveryLeaseRequest
			module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
				workspaceID: terminal.Facts.WorkspaceID, runID: terminal.Facts.RunID,
				source: "running_nonterminal_terminal_handoff", driveKey: key,
				leaseGeneration: running.RecoveryGeneration, gateEpoch: gate.Epoch, gateToken: gate.Token,
			}, &retry)

			persisted, err := base.Load(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID)
			if err != nil || persisted.DriveLease != nil || !runtimeTerminalDeliveryFullyAcknowledged(persisted) ||
				persisted.Facts.Recovery.CASReloadAttempts != 1 || persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 ||
				persisted.StateRevision != terminal.StateRevision+4 || *calls != [3]int{1, 1, 1} ||
				background.asyncFunctionCalls != 0 {
				t.Fatalf("worker exit did not atomically release/deliver: aggregate=%+v calls=%v timers=%d err=%v", persisted, *calls, background.asyncFunctionCalls, err)
			}
			if fault.fired != (test.failBefore || test.failAfter) {
				t.Fatalf("release fault fired=%v, want %v", fault.fired, test.failBefore || test.failAfter)
			}
			if !module.beginRuntimeDrive(key) {
				t.Fatal("completed worker exit retained process gate")
			}
			module.endRuntimeDrive(key)

			// A fresh owner has only the persisted aggregate. Reconcile the same
			// startup classification and prove that completed delivery is a zero-write,
			// zero-budget, zero-redelivery outcome.
			restartedOwner := newRuntimeTestOwner()
			if err := restartedOwner.ConfigureRuntimeRepository(base); err != nil {
				t.Fatal(err)
			}
			restarted := newTestBiz(restartedOwner, hub, &recordingBackgroundRoutine{})
			if !restarted.beginRuntimeDrive(key) {
				t.Fatal("fresh owner did not acquire startup settlement gate")
			}
			restartGate, ok := restarted.currentRuntimeDriveGate(key)
			if !ok {
				t.Fatal("fresh owner startup gate identity is unavailable")
			}
			beforeRevision := persisted.StateRevision
			if !restarted.settleRuntimeDriveGateOrRetain(ctx, runtimeDriveGateSettlementRequest{
				WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, DriveKey: key,
				GateEpoch: restartGate.Epoch, GateToken: restartGate.Token, Source: "fresh_owner_terminal_handoff",
				ReleaseWhenNoLease: true,
			}) {
				t.Fatal("fresh owner did not settle fully acknowledged terminal handoff")
			}
			reloaded, err := base.Load(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
			if err != nil || reloaded.StateRevision != beforeRevision || reloaded.DriveLease != nil ||
				reloaded.Facts.Recovery.CASReloadAttempts != 1 || reloaded.Facts.Recovery.TerminalDeliveryAttempts != 0 ||
				*calls != [3]int{1, 1, 1} {
				t.Fatalf("fresh owner replay changed terminal handoff: aggregate=%+v calls=%v err=%v", reloaded, *calls, err)
			}
			if !restarted.beginRuntimeDrive(key) {
				t.Fatal("fresh owner terminal replay retained process gate")
			}
			restarted.endRuntimeDrive(key)
		})
	}
}
