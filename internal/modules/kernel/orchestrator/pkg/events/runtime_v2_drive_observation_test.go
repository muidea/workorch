package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2DriveObservedDataIsReadOnlyScopedTelemetry(t *testing.T) {
	data := RuntimeV2DriveObservedData{
		WorkspaceID: "ws-observed", RunID: "run-observed", Kind: RuntimeV2DriveObservationRetryTimerScheduled,
		AggregateRevision: 7, RecoveryGeneration: 3, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseScheduled, Source: "startup_recovery",
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
	if err := (RuntimeV2DriveObservedData{WorkspaceID: common.WorkspaceID("ws"), RunID: "run", Source: "test"}).Validate(); err == nil {
		t.Fatal("observation without kind was accepted")
	}
}
