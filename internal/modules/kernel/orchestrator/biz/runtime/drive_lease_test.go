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
	initialized, err := controller.Initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	requested := runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2",
		DueAt: due.Format(time.RFC3339Nano),
	}
	scheduled, lease, changed, err := controller.ScheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, requested)
	if err != nil || !changed || lease.State != runtimecontract.RuntimeDriveLeaseScheduled || lease.AggregateRevision != scheduled.StateRevision || lease.RecoveryGeneration != 1 {
		t.Fatalf("scheduled lease=%+v aggregate=%+v changed=%v err=%v", lease, scheduled, changed, err)
	}
	saves := repository.saves
	duplicate, duplicateLease, changed, err := controller.ScheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2",
		DueAt: due.Add(time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil || changed || duplicate.StateRevision != scheduled.StateRevision || duplicateLease.RecoveryGeneration != lease.RecoveryGeneration || repository.saves != saves {
		t.Fatalf("duplicate schedule aggregate=%+v lease=%+v changed=%v saves=%d err=%v", duplicate, duplicateLease, changed, repository.saves, err)
	}
	_, early, claimed, err := controller.ClaimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Add(-time.Second).Format(time.RFC3339Nano))
	if err != nil || claimed || early.State != runtimecontract.RuntimeDriveLeaseScheduled || repository.saves != saves {
		t.Fatalf("early claim lease=%+v claimed=%v saves=%d err=%v", early, claimed, repository.saves, err)
	}
	claimedAggregate, claimedLease, claimed, err := controller.ClaimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Format(time.RFC3339Nano))
	if err != nil || !claimed || claimedLease.State != runtimecontract.RuntimeDriveLeaseRunning || claimedLease.AggregateRevision != claimedAggregate.StateRevision || repository.saves != saves+1 {
		t.Fatalf("claimed lease=%+v aggregate=%+v claimed=%v saves=%d err=%v", claimedLease, claimedAggregate, claimed, repository.saves, err)
	}
	recovered, recoveredLease, changed, err := controller.RecoverDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, due.Add(time.Minute).Format(time.RFC3339Nano))
	if err != nil || !changed || recoveredLease.State != runtimecontract.RuntimeDriveLeaseScheduled || recoveredLease.RecoveryGeneration != claimedLease.RecoveryGeneration+1 || recoveredLease.AggregateRevision != recovered.StateRevision {
		t.Fatalf("recovered lease=%+v aggregate=%+v changed=%v err=%v", recoveredLease, recovered, changed, err)
	}
	_, released, err := controller.ReleaseDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, claimedLease.RecoveryGeneration)
	if err != nil || released {
		t.Fatalf("old generation release=%v err=%v", released, err)
	}
}

func TestRuntimeDriveLeaseTimerCannotClaimStaleRevision(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	initialized, err := controller.Initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	_, lease, _, err := controller.ScheduleDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeDriveLease{
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
	stale, claimedLease, claimed, err := controller.ClaimDriveLease(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, lease, due.Format(time.RFC3339Nano))
	if err != nil || claimed || claimedLease.WorkspaceID != "" || stale.DriveLease != nil {
		t.Fatalf("stale timer aggregate=%+v lease=%+v claimed=%v err=%v", stale, claimedLease, claimed, err)
	}
}
