package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDriveLeasePersistsSingleRetryIdentityAndRecoversRunningLease(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	requested := runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "cas/1/2",
		DueAt: due.Format(time.RFC3339Nano),
	}
	scheduled, lease, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, requested)
	if err != nil || !changed || lease.State != runtimecontract.RuntimeDriveLeaseScheduled || lease.AggregateRevision != scheduled.StateRevision || lease.RecoveryGeneration != 1 {
		t.Fatalf("scheduled lease=%+v aggregate=%+v changed=%v err=%v", lease, scheduled, changed, err)
	}
	saves := repository.saves
	duplicate, duplicateLease, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "cas/1/2",
		DueAt: due.Add(time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil || changed || duplicate.StateRevision != scheduled.StateRevision || duplicateLease.RecoveryGeneration != lease.RecoveryGeneration || repository.saves != saves {
		t.Fatalf("duplicate schedule aggregate=%+v lease=%+v changed=%v saves=%d err=%v", duplicate, duplicateLease, changed, repository.saves, err)
	}
	staleSchedulerRequest := requested
	staleSchedulerRequest.FailureSignature = "cas/stale/different"
	staleSchedulerRequest.DueAt = due.Add(2 * time.Hour).Format(time.RFC3339Nano)
	unchanged, retainedLease, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, staleSchedulerRequest)
	if err != nil || changed || unchanged.StateRevision != scheduled.StateRevision || retainedLease.RecoveryGeneration != lease.RecoveryGeneration ||
		retainedLease.FailureSignature != lease.FailureSignature || repository.saves != saves {
		t.Fatalf("stale scheduler replaced current lease aggregate=%+v lease=%+v changed=%v saves=%d err=%v", unchanged, retainedLease, changed, repository.saves, err)
	}
	_, early, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Add(-time.Second).Format(time.RFC3339Nano))
	if err != nil || claimed || early.State != runtimecontract.RuntimeDriveLeaseScheduled || repository.saves != saves {
		t.Fatalf("early claim lease=%+v claimed=%v saves=%d err=%v", early, claimed, repository.saves, err)
	}
	claimedAggregate, claimedLease, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Format(time.RFC3339Nano))
	if err != nil || !claimed || claimedLease.State != runtimecontract.RuntimeDriveLeaseRunning || claimedLease.AggregateRevision != claimedAggregate.StateRevision || repository.saves != saves+1 {
		t.Fatalf("claimed lease=%+v aggregate=%+v claimed=%v saves=%d err=%v", claimedLease, claimedAggregate, claimed, repository.saves, err)
	}
	recovered, recoveredLease, changed, err := controller.recoverDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, due.Add(time.Minute).Format(time.RFC3339Nano))
	if err != nil || !changed || recoveredLease.State != runtimecontract.RuntimeDriveLeaseScheduled || recoveredLease.RecoveryGeneration != claimedLease.RecoveryGeneration+1 || recoveredLease.AggregateRevision != recovered.StateRevision {
		t.Fatalf("recovered lease=%+v aggregate=%+v changed=%v err=%v", recoveredLease, recovered, changed, err)
	}
	_, released, err := controller.releaseDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, claimedLease.RecoveryGeneration)
	if err != nil || released {
		t.Fatalf("old generation release=%v err=%v", released, err)
	}
}

func TestRuntimeDriveLeaseTimerCannotClaimStaleRevision(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	_, lease, _, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "cas/1/2", DueAt: due.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Model a newer persisted owner-local fact. The old timer must consume only
	// its stale lease and must not begin a second drive.
	current := *repository.aggregate
	current.StateRevision++
	repository.aggregate = &current
	stale, claimedLease, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Format(time.RFC3339Nano))
	if err != nil || claimed || claimedLease.WorkspaceID != "" || stale.DriveLease != nil {
		t.Fatalf("stale timer aggregate=%+v lease=%+v claimed=%v err=%v", stale, claimedLease, claimed, err)
	}
}

func TestRuntimeDriveLeaseOldTimerAdoptsReissuedSameSuccessor(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
	_, oldLease, _, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "lifecycle-owner-delivery/exact-resolution", DueAt: due,
	})
	if err != nil {
		t.Fatal(err)
	}
	current := *repository.aggregate
	expectedRevision := current.StateRevision
	current.StateRevision++
	reissued := current.DriveLease.Clone()
	reissued.AggregateRevision = current.StateRevision
	reissued.RecoveryGeneration++
	current.DriveLease = &reissued
	if err := repository.Save(ctx, current, expectedRevision); err != nil {
		t.Fatal(err)
	}
	_, adopted, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, oldLease, time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano))
	if err != nil || claimed || !runtimecontract.SameRuntimeDriveLeaseIdentity(adopted, reissued) {
		t.Fatalf("old timer adoption=%+v claimed=%v want=%+v err=%v", adopted, claimed, reissued, err)
	}
}

func TestRuntimeDriveLeaseRunningOwnerAtomicallyReschedulesSameIdentity(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstDue := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	requested := runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "lifecycle-owner-delivery/decision_continuation/resolution-1",
		DueAt: firstDue.Format(time.RFC3339Nano),
	}
	_, scheduled, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, requested, 0)
	if err != nil || !changed {
		t.Fatalf("schedule changed=%v err=%v", changed, err)
	}
	_, running, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, scheduled, scheduled.DueAt)
	if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim lease=%+v claimed=%v err=%v", running, claimed, err)
	}

	secondDue := firstDue.Add(2 * time.Second)
	renewal := requested
	renewal.DueAt = secondDue.Format(time.RFC3339Nano)
	renewedAggregate, renewed, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, renewal, running.RecoveryGeneration)
	if err != nil || !changed || renewed.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		renewed.RecoveryGeneration != running.RecoveryGeneration+1 || renewed.AggregateRevision != renewedAggregate.StateRevision || renewed.DueAt != renewal.DueAt {
		t.Fatalf("renewed aggregate=%+v lease=%+v changed=%v err=%v", renewedAggregate, renewed, changed, err)
	}
	if _, released, err := controller.releaseDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, running.RecoveryGeneration); err != nil || released {
		t.Fatalf("stale running generation released successor=%v err=%v", released, err)
	}

	staleRenewal := renewal
	staleRenewal.DueAt = secondDue.Add(time.Second).Format(time.RFC3339Nano)
	unchanged, current, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, staleRenewal, running.RecoveryGeneration)
	if err != nil || changed || current.RecoveryGeneration != renewed.RecoveryGeneration || unchanged.StateRevision != renewedAggregate.StateRevision {
		t.Fatalf("stale renewal aggregate=%+v lease=%+v changed=%v err=%v", unchanged, current, changed, err)
	}
}

func TestRuntimeDriveLeaseAdministrativeTerminationReissuesRetiredGenerationOnlyWhileDeliveryPending(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	_, scheduled, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "pre-termination-running",
		DueAt: due.Format(time.RFC3339Nano),
	}, 0)
	if err != nil || !changed {
		t.Fatalf("schedule pre-termination lease changed=%v err=%v", changed, err)
	}
	_, running, claimed, err := controller.claimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, scheduled, scheduled.DueAt)
	if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim pre-termination lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	terminated, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run")
	if err != nil || terminated.DriveLease != nil || !AdministrativeTerminationAwaitsTerminalDelivery(terminated) {
		t.Fatalf("termination did not retire running generation: aggregate=%+v err=%v", terminated, err)
	}
	outcome, terminal, err := DeriveTerminalOutcome(terminated.Decision, terminated.Termination)
	if err != nil || !terminal {
		t.Fatalf("derive administrative terminal outcome=%+v terminal=%v err=%v", outcome, terminal, err)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID(terminated.Facts.WorkspaceID, terminated.Facts.RunID, terminated.Decision.AuthoritySnapshot.ID, outcome)
	initialRequest := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{TerminalDeliveryID: deliveryID}
	initialRequest.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, 0)
	saves := repository.saves
	if _, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", initialRequest); !errors.Is(err, common.ErrConflict) || consumed || repository.saves != saves {
		t.Fatalf("initial recovery bypassed retired running marker consumed=%v saves=%d/%d err=%v", consumed, repository.saves, saves, err)
	}
	recoveryRequest := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{
		TerminalDeliveryID: deliveryID, FailedLeaseGeneration: running.RecoveryGeneration,
	}
	recoveryRequest.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, running.RecoveryGeneration)
	terminated, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", recoveryRequest)
	if err != nil || !consumed || terminated.Facts.Recovery.TerminalDeliveryRequest != recoveryRequest {
		t.Fatalf("authorize retired terminal generation consumed=%v recovery=%+v err=%v", consumed, terminated.Facts.Recovery, err)
	}

	retry := runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		RecoveryRequestID: recoveryRequest.RequestID, DueAt: due.Add(time.Minute).Format(time.RFC3339Nano),
	}
	reissuedAggregate, reissued, changed, err := controller.driveDelivery.scheduleDriveLease(ctx,
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, retry, running.RecoveryGeneration)
	if err != nil || !changed || reissued.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		reissued.RecoveryGeneration != running.RecoveryGeneration+1 || reissued.AggregateRevision != reissuedAggregate.StateRevision {
		t.Fatalf("reissue retired terminal generation aggregate=%+v lease=%+v changed=%v err=%v", reissuedAggregate, reissued, changed, err)
	}
	tamperedTerminationLease := reissuedAggregate
	tamperedLease := reissued.Clone()
	tamperedLease.FailureSignature = "lifecycle-owner-delivery/forged"
	tamperedTerminationLease.DriveLease = &tamperedLease
	if err := tamperedTerminationLease.Validate(); err == nil {
		t.Fatal("administratively terminated aggregate accepted a non-canonical delivery lease on fresh validation")
	}

	// Model the tighter response-loss race on a separately terminated aggregate:
	// another delivery path completes all owner acknowledgements before the stale
	// worker reaches its scheduling CAS. A fully delivered terminal must reject
	// the retired-generation exception as a zero-write stale request.
	fullyAcknowledged := reissuedAggregate
	fullyAcknowledged.DriveLease = nil
	outcome, terminal, err = DeriveTerminalOutcome(fullyAcknowledged.Decision, fullyAcknowledged.Termination)
	if err != nil || !terminal {
		t.Fatalf("derive administrative terminal outcome=%+v terminal=%v err=%v", outcome, terminal, err)
	}
	deliveryID = runtimecontract.StableTerminalDeliveryID(
		fullyAcknowledged.Facts.WorkspaceID,
		fullyAcknowledged.Facts.RunID,
		fullyAcknowledged.Decision.AuthoritySnapshot.ID,
		outcome,
	)
	fullyAcknowledged.DeliveryAcknowledgements = nil
	for _, owner := range []runtimecontract.RuntimeDeliveryOwner{
		runtimecontract.RuntimeDeliveryOwnerResult,
		runtimecontract.RuntimeDeliveryOwnerWorkspace,
		runtimecontract.RuntimeDeliveryOwnerRun,
	} {
		state := runtimecontract.RuntimeDeliveryAcknowledgedNoop
		resultID := common.ResultID("")
		switch owner {
		case runtimecontract.RuntimeDeliveryOwnerResult:
			state = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
			resultID = runtimecontract.RuntimeTerminalResultID(fullyAcknowledged.Facts.WorkspaceID, fullyAcknowledged.Facts.RunID)
		case runtimecontract.RuntimeDeliveryOwnerWorkspace:
			state = runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased
		case runtimecontract.RuntimeDeliveryOwnerRun:
			state = runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal
		}
		fullyAcknowledged.DeliveryAcknowledgements = append(fullyAcknowledged.DeliveryAcknowledgements, runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID:         fullyAcknowledged.Facts.WorkspaceID,
			RunID:               fullyAcknowledged.Facts.RunID,
			ResultID:            resultID,
			TerminalDeliveryID:  deliveryID,
			Owner:               owner,
			AcknowledgedState:   state,
			AuthoritySnapshotID: fullyAcknowledged.Decision.AuthoritySnapshot.ID,
			CausationID:         "terminal-delivery/" + deliveryID,
			Outcome:             outcome,
		})
	}
	fullyAcknowledged.StateRevision++
	if err := fullyAcknowledged.Validate(); err != nil {
		t.Fatalf("fully acknowledged administrative terminal: %v", err)
	}
	repository.aggregate = &fullyAcknowledged
	saves = repository.saves
	unchanged, absent, changed, err := controller.driveDelivery.scheduleDriveLease(ctx,
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, retry, running.RecoveryGeneration)
	if err != nil || changed || absent.WorkspaceID != "" || unchanged.StateRevision != fullyAcknowledged.StateRevision ||
		unchanged.DriveLease != nil || repository.saves != saves || !TerminalDeliveryFullyAcknowledged(unchanged) {
		t.Fatalf("fully acknowledged terminal admitted stale successor: aggregate=%+v lease=%+v changed=%v saves=%d/%d err=%v", unchanged, absent, changed, repository.saves, saves, err)
	}

	fullyAcknowledgedRunning := fullyAcknowledged
	fullyAcknowledgedRunning.StateRevision++
	runningTerminalLease := reissued.Clone()
	runningTerminalLease.AggregateRevision = fullyAcknowledgedRunning.StateRevision
	runningTerminalLease.State = runtimecontract.RuntimeDriveLeaseRunning
	fullyAcknowledgedRunning.DriveLease = &runningTerminalLease
	if err := fullyAcknowledgedRunning.Validate(); err != nil {
		t.Fatalf("fully acknowledged terminal with exact running lease: %v", err)
	}
	repository.aggregate = &fullyAcknowledgedRunning
	saves = repository.saves
	retired, absent, changed, err := controller.driveDelivery.scheduleDriveLease(ctx,
		initialized.Facts.WorkspaceID, initialized.Facts.RunID, retry, runningTerminalLease.RecoveryGeneration)
	if err != nil || !changed || absent.WorkspaceID != "" || retired.DriveLease != nil ||
		retired.StateRevision != fullyAcknowledgedRunning.StateRevision+1 || repository.saves != saves+1 ||
		!TerminalDeliveryFullyAcknowledged(retired) {
		t.Fatalf("fully acknowledged terminal retained exact running lease: aggregate=%+v lease=%+v changed=%v saves=%d/%d err=%v", retired, absent, changed, repository.saves, saves, err)
	}

	saves = repository.saves
	budgetReplay, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx,
		initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", recoveryRequest)
	if err != nil || consumed || budgetReplay.StateRevision != retired.StateRevision ||
		budgetReplay.Facts.Recovery != retired.Facts.Recovery || repository.saves != saves {
		t.Fatalf("fully acknowledged terminal consumed delivery budget: aggregate=%+v consumed=%v saves=%d/%d err=%v", budgetReplay, consumed, repository.saves, saves, err)
	}
}

func TestRuntimeTerminalDeliveryBudgetExhaustionPersistsExactInterventionOnce(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxTerminalDeliveryAttempts = 1
	facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(facts.ConvergencePolicy, facts.RecoveryPolicy)
	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run")
	if err != nil {
		t.Fatal(err)
	}
	outcome, sealed, err := DeriveTerminalOutcome(terminal.Decision, terminal.Termination)
	if err != nil || !sealed {
		t.Fatalf("derive terminal outcome=%+v sealed=%v err=%v", outcome, sealed, err)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID(terminal.Facts.WorkspaceID, terminal.Facts.RunID, terminal.Decision.AuthoritySnapshot.ID, outcome)
	initialRequest := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{TerminalDeliveryID: deliveryID}
	initialRequest.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, 0)
	authorized, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", initialRequest)
	if err != nil || !consumed || authorized.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("initial terminal recovery authorization aggregate=%+v consumed=%v err=%v", authorized, consumed, err)
	}
	dueAt := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	_, scheduled, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		RecoveryRequestID: initialRequest.RequestID, DueAt: dueAt,
	}, 0)
	if err != nil || !changed {
		t.Fatalf("schedule first terminal retry lease=%+v changed=%v err=%v", scheduled, changed, err)
	}
	_, running, claimed, err := controller.claimDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, scheduled, dueAt)
	if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim terminal retry lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	exhaustedRequest := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{
		TerminalDeliveryID: deliveryID, FailedLeaseGeneration: running.RecoveryGeneration,
	}
	exhaustedRequest.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, running.RecoveryGeneration)
	saves := repository.saves
	intervened, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", exhaustedRequest)
	if err != nil || consumed || intervened.DriveLease != nil ||
		intervened.Facts.Recovery.TerminalDeliveryIntervention != exhaustedRequest || repository.saves != saves+1 {
		t.Fatalf("terminal exhaustion disposition aggregate=%+v consumed=%v saves=%d/%d err=%v", intervened, consumed, repository.saves, saves+1, err)
	}
	if err := intervened.Validate(); err != nil {
		t.Fatalf("terminal intervention aggregate invalid: %v", err)
	}
	saves = repository.saves
	duplicate, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", exhaustedRequest)
	if err != nil || consumed || duplicate.StateRevision != intervened.StateRevision || repository.saves != saves {
		t.Fatalf("terminal intervention duplicate mutated aggregate=%+v consumed=%v saves=%d/%d err=%v", duplicate, consumed, repository.saves, saves, err)
	}
}

func TestRuntimeTerminalScheduledLeaseStaleClaimRematerializesExactAuthorizedGenerationAfterRestart(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run")
	if err != nil {
		t.Fatal(err)
	}
	outcome, sealed, err := DeriveTerminalOutcome(terminal.Decision, terminal.Termination)
	if err != nil || !sealed {
		t.Fatalf("derive terminal outcome=%+v sealed=%v err=%v", outcome, sealed, err)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID(terminal.Facts.WorkspaceID, terminal.Facts.RunID, terminal.Decision.AuthoritySnapshot.ID, outcome)
	request0 := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{TerminalDeliveryID: deliveryID}
	request0.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, 0)
	if _, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", request0); err != nil || !consumed {
		t.Fatalf("consume request0 consumed=%v err=%v", consumed, err)
	}
	dueAt := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	_, generation1, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		RecoveryRequestID: request0.RequestID, DueAt: dueAt,
	}, 0)
	if err != nil || !changed {
		t.Fatalf("schedule generation1=%+v changed=%v err=%v", generation1, changed, err)
	}
	_, running1, claimed, err := controller.claimDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, generation1, dueAt)
	if err != nil || !claimed {
		t.Fatalf("claim generation1=%+v claimed=%v err=%v", running1, claimed, err)
	}
	request1 := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{
		TerminalDeliveryID: deliveryID, FailedLeaseGeneration: running1.RecoveryGeneration,
	}
	request1.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, running1.RecoveryGeneration)
	if _, consumed, err := controller.driveDelivery.consumeRecoveryBudget(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, "", "", request1); err != nil || !consumed {
		t.Fatalf("consume request1 consumed=%v err=%v", consumed, err)
	}
	_, generation2, changed, err := controller.driveDelivery.scheduleDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
		RecoveryRequestID: request1.RequestID, DueAt: dueAt,
	}, running1.RecoveryGeneration)
	if err != nil || !changed || generation2.RecoveryGeneration != 2 {
		t.Fatalf("schedule generation2=%+v changed=%v err=%v", generation2, changed, err)
	}
	// A partial owner acknowledgement advances the aggregate revision while the
	// scheduled generation remains the same durable delivery authority.
	drifted := *repository.aggregate
	drifted.StateRevision++
	if err := drifted.Validate(); err != nil {
		t.Fatalf("revision-drifted scheduled terminal lease: %v", err)
	}
	repository.aggregate = &drifted
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	cleared, absent, claimed, err := restarted.claimDriveLease(ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, generation2, dueAt)
	if err != nil || claimed || absent.WorkspaceID != "" || cleared.DriveLease != nil {
		t.Fatalf("stale scheduled claim aggregate=%+v lease=%+v claimed=%v err=%v", cleared, absent, claimed, err)
	}
	beforeAttempts := cleared.Facts.Recovery.TerminalDeliveryAttempts
	rematerialized, generation2Again, changed, err := restarted.driveDelivery.scheduleDriveLease(ctx,
		terminal.Facts.WorkspaceID, terminal.Facts.RunID, runtimecontract.RuntimeDriveLease{
			WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
			Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
			RecoveryRequestID: request1.RequestID, DueAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		}, request1.FailedLeaseGeneration)
	if err != nil || !changed || generation2Again.RecoveryGeneration != generation2.RecoveryGeneration ||
		generation2Again.RecoveryRequestID != request1.RequestID ||
		rematerialized.Facts.Recovery.TerminalDeliveryAttempts != beforeAttempts {
		t.Fatalf("rematerialized generation=%+v changed=%v recovery=%+v err=%v", generation2Again, changed, rematerialized.Facts.Recovery, err)
	}
}
