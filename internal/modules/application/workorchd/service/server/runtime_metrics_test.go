package server

import (
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestServerMetricsProjectsRuntimeDriveObservationsWithoutRuntimeMutation(t *testing.T) {
	metrics := &serverMetrics{}
	metrics.observeRuntimeDrive(orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeDriveObservationScheduled,
		AggregateRevision: 9, Source: "run_state_active",
	})
	metrics.observeRuntimeDrive(orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: 10, RecoveryGeneration: 2, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseScheduled, Source: "startup_recovery",
	})
	metrics.observeRuntimeDrive(orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeDriveObservationCASReload,
		AggregateRevision: 10, RecoveryGeneration: 2, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseRunning, Source: "startup_recovery",
	})
	body := metrics.renderRuntimeDriveMetrics()
	for _, want := range []string{
		`workorch_runtime_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="drive_scheduled"} 1`,
		`workorch_runtime_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="retry_timer_scheduled"} 1`,
		`workorch_runtime_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="cas_reload"} 1`,
		`aggregate_revision="10",recovery_generation="2",failure_category="cas_conflict",lease_state="running",source="startup_recovery"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runtime observation metric missing %q:\n%s", want, body)
		}
	}
}
