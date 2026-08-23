package runtime

import (
	"context"
	"testing"
	"time"

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
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2",
		DueAt: due.Format(time.RFC3339Nano),
	}
	scheduled, lease, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, requested)
	if err != nil || !changed || lease.State != runtimecontract.RuntimeDriveLeaseScheduled || lease.AggregateRevision != scheduled.StateRevision || lease.RecoveryGeneration != 1 {
		t.Fatalf("scheduled lease=%+v aggregate=%+v changed=%v err=%v", lease, scheduled, changed, err)
	}
	saves := repository.saves
	duplicate, duplicateLease, changed, err := controller.scheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2",
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
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2", DueAt: due.Format(time.RFC3339Nano),
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
