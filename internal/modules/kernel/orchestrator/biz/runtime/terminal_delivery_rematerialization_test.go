package runtime

import (
	"context"
	"testing"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeTerminalDeliveryHistoricalRetiredGenerationDoesNotBlockLaterRematerialization(t *testing.T) {
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
	terminal, err := controller.requestTermination(
		ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run",
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, sealed, err := DeriveTerminalOutcome(terminal.Decision, terminal.Termination)
	if err != nil || !sealed {
		t.Fatalf("terminal outcome=%+v sealed=%v err=%v", outcome, sealed, err)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID(
		terminal.Facts.WorkspaceID, terminal.Facts.RunID, terminal.Decision.AuthoritySnapshot.ID, outcome,
	)
	request5 := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{TerminalDeliveryID: deliveryID, FailedLeaseGeneration: 5}
	request5.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, 5)
	request6 := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{TerminalDeliveryID: deliveryID, FailedLeaseGeneration: 6}
	request6.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, 6)

	// G5 was administratively retired and its exact R5 handoff already created
	// G6. A later durable R6 is now the only authorization; the historical G5
	// marker must not be compared as though it were the failed generation again.
	retired5 := runtimecontract.RuntimeDriveLease{
		WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
		AggregateRevision: terminal.Termination.Revision - 1, RecoveryGeneration: 5,
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "pre-terminal-running-g5",
		DueAt: "2026-09-04T12:00:00Z", State: runtimecontract.RuntimeDriveLeaseRunning,
	}
	lineage := terminal
	lineage.Facts.Recovery.TerminalDeliveryAttempts = 2
	lineage.Facts.Recovery.TerminalDeliveryRequest = request6
	lineage.Facts.Recovery.RetiredTerminalDeliveryLease = retired5
	lineage.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID = request5.RequestID
	if err := lineage.Validate(); err != nil {
		t.Fatalf("historical retired lineage invalid: %v", err)
	}
	repository.aggregate = &lineage

	dueAt := time.Date(2026, 9, 4, 12, 1, 0, 0, time.UTC).Format(time.RFC3339Nano)
	materialized, generation7, changed, err := controller.driveDelivery.scheduleDriveLease(
		ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeDriveLease{
			WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
			Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
			RecoveryRequestID: request6.RequestID, DueAt: dueAt,
		},
		6,
	)
	if err != nil || !changed || generation7.RecoveryGeneration != 7 || generation7.RecoveryRequestID != request6.RequestID ||
		materialized.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != request5.RequestID ||
		materialized.Facts.Recovery.TerminalDeliveryAttempts != lineage.Facts.Recovery.TerminalDeliveryAttempts {
		t.Fatalf("G5/R5 -> R6/G7 materialization aggregate=%+v lease=%+v changed=%v err=%v", materialized, generation7, changed, err)
	}

	// Model restart after aggregate revision drift invalidates the scheduled G7.
	// Clearing the stale lease and rematerializing the still-current R6 must be
	// idempotent with respect to both budget and historical marker consumption.
	drifted := materialized
	drifted.StateRevision++
	if err := drifted.Validate(); err != nil {
		t.Fatalf("drifted G7 aggregate invalid: %v", err)
	}
	repository.aggregate = &drifted
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	cleared, absent, claimed, err := restarted.claimDriveLease(
		ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID, generation7, dueAt,
	)
	if err != nil || claimed || absent.WorkspaceID != "" || cleared.DriveLease != nil {
		t.Fatalf("stale G7 clear aggregate=%+v lease=%+v claimed=%v err=%v", cleared, absent, claimed, err)
	}
	attempts := cleared.Facts.Recovery.TerminalDeliveryAttempts
	rematerialized, generation7Again, changed, err := restarted.driveDelivery.scheduleDriveLease(
		ctx, terminal.Facts.WorkspaceID, terminal.Facts.RunID,
		runtimecontract.RuntimeDriveLease{
			WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID,
			Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery",
			RecoveryRequestID: request6.RequestID, DueAt: dueAt,
		},
		6,
	)
	if err != nil || !changed || generation7Again.RecoveryGeneration != 7 ||
		rematerialized.Facts.Recovery.TerminalDeliveryAttempts != attempts ||
		rematerialized.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != request5.RequestID {
		t.Fatalf("restart R6/G7 rematerialization aggregate=%+v lease=%+v changed=%v err=%v", rematerialized, generation7Again, changed, err)
	}
}
