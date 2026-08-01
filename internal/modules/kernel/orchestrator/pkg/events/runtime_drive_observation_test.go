package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDriveObservedDataIsReadOnlyScopedTelemetry(t *testing.T) {
	data := RuntimeDriveObservedData{
		WorkspaceID: "ws-observed", RunID: "run-observed", Kind: RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: 7, RecoveryGeneration: 3, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseScheduled, Source: "startup_recovery",
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
	if err := (RuntimeDriveObservedData{WorkspaceID: common.WorkspaceID("ws"), RunID: "run", Source: "test"}).Validate(); err == nil {
		t.Fatal("observation without kind was accepted")
	}
}
