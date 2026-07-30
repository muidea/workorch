package server

import (
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestServerMetricsProjectsRuntimeV2DriveObservationsWithoutRuntimeMutation(t *testing.T) {
	metrics := &serverMetrics{}
	metrics.observeRuntimeV2Drive(orchestratorevents.RuntimeV2DriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeV2DriveObservationScheduled,
		AggregateRevision: 9, Source: "run_state_active",
	})
	metrics.observeRuntimeV2Drive(orchestratorevents.RuntimeV2DriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeV2DriveObservationRetryTimerScheduled,
		AggregateRevision: 10, RecoveryGeneration: 2, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseScheduled, Source: "startup_recovery",
	})
	metrics.observeRuntimeV2Drive(orchestratorevents.RuntimeV2DriveObservedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", Kind: orchestratorevents.RuntimeV2DriveObservationCASReload,
		AggregateRevision: 10, RecoveryGeneration: 2, FailureCategory: "cas_conflict",
		LeaseState: workorchruntimecontract.RuntimeDriveLeaseRunning, Source: "startup_recovery",
	})
	body := metrics.renderRuntimeV2DriveMetrics()
	for _, want := range []string{
		`workorch_runtime_v2_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="drive_scheduled"} 1`,
		`workorch_runtime_v2_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="retry_timer_scheduled"} 1`,
		`workorch_runtime_v2_drive_observations_total{workspace="ws-metrics",run="run-metrics",kind="cas_reload"} 1`,
		`aggregate_revision="10",recovery_generation="2",failure_category="cas_conflict",lease_state="running",source="startup_recovery"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runtime observation metric missing %q:\n%s", want, body)
		}
	}
}
