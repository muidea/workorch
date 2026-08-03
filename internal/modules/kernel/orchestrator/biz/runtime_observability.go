package orchestrator

import (
	"context"
	"errors"
	"log/slog"

	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// observeRuntimeDrive emits an immutable, display-only SM-4 observation.
// It is deliberately not routed back into Runtime: the persisted aggregate
// and its owner-local internal events remain the only recovery authority.
func (m *Biz) observeRuntimeDrive(ctx context.Context, data orchestratorevents.RuntimeDriveObservedData) {
	if m == nil {
		return
	}
	data = data.Normalized()
	if err := data.Validate(); err != nil {
		return
	}
	// Normal drive/timer observations are intentionally debug-level. The
	// process-local metric stream remains available at normal log levels while
	// a busy but healthy Runtime cannot turn observability itself into a CPU or
	// log-volume failure mode.
	slog.Debug("workorch Runtime drive observed",
		"kind", data.Kind,
		"workspaceID", data.WorkspaceID,
		"runID", data.RunID,
		"aggregateRevision", data.AggregateRevision,
		"recoveryGeneration", data.RecoveryGeneration,
		"failureCategory", data.FailureCategory,
		"leaseState", data.LeaseState,
		"source", data.Source,
	)
	if m.EventHub() == nil {
		return
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorRuntimeDriveObserved,
		ModuleID,
		orchestratorevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		data,
	)
	bindOwnerCommandLane(event, orchestratorevents.Contracts(), orchestratorevents.TopicOrchestratorRuntimeDriveObserved, "runtime_drive", string(data.RunID), data.WorkspaceID, data.RunID)
	m.PostEvent(event)
}

func runtimeDriveLeaseStateForGeneration(generation uint64) runtimecontract.RuntimeDriveLeaseState {
	if generation == 0 {
		return ""
	}
	return runtimecontract.RuntimeDriveLeaseRunning
}

// runtimeDriveFailureObservedRevision extracts the typed revision supplied
// by the persistence owner. No error text is parsed: a generic conflict has
// no observation revision and cannot be reported as a CAS reload.
func runtimeDriveFailureObservedRevision(err error) (uint64, bool) {
	var cas *runtime.RuntimeCASConflictError
	if errors.As(err, &cas) {
		return cas.ObservedRevision, true
	}
	var semantic *runtime.RuntimeSemanticRejectionError
	if errors.As(err, &semantic) {
		return semantic.ObservedRevision, true
	}
	return 0, false
}
