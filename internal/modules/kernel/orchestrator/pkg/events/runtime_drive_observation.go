package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeDriveObservationKind is a process-observability event emitted by
// the Orchestrator owner. It does not express a transition command or a
// second recovery state machine.
type RuntimeDriveObservationKind string

const (
	RuntimeDriveObservationScheduled           RuntimeDriveObservationKind = "drive_scheduled"
	RuntimeDriveObservationRunning             RuntimeDriveObservationKind = "drive_running"
	RuntimeDriveObservationRetryTimerScheduled RuntimeDriveObservationKind = "retry_timer_scheduled"
	RuntimeDriveObservationRetryTimerFired     RuntimeDriveObservationKind = "retry_timer_fired"
	RuntimeDriveObservationCASReload           RuntimeDriveObservationKind = "cas_reload"
	RuntimeDriveObservationSemanticRejection   RuntimeDriveObservationKind = "semantic_rejection"
	RuntimeDriveObservationDeliveryPending     RuntimeDriveObservationKind = "delivery_pending"
	RuntimeDriveObservationStateCommitted      RuntimeDriveObservationKind = "state_committed"
)

func (k RuntimeDriveObservationKind) Valid() bool {
	switch k {
	case RuntimeDriveObservationScheduled,
		RuntimeDriveObservationRunning,
		RuntimeDriveObservationRetryTimerScheduled,
		RuntimeDriveObservationRetryTimerFired,
		RuntimeDriveObservationCASReload,
		RuntimeDriveObservationSemanticRejection,
		RuntimeDriveObservationDeliveryPending:
		return true
	case RuntimeDriveObservationStateCommitted:
		return true
	default:
		return false
	}
}

// RuntimeDriveObservedData is immutable, bounded telemetry for one
// owner-local SM-4 observation. AggregateRevision may be zero only before an
// aggregate is readable (for example, activation scheduling); the explicit
// field still distinguishes that case from an omitted revision. It is not
// persisted and can never be used to drive a transition.
type RuntimeDriveObservedData struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	Kind               RuntimeDriveObservationKind
	AggregateRevision  uint64
	RecoveryGeneration uint64
	FailureCategory    string
	LeaseState         workorchruntimecontract.RuntimeDriveLeaseState
	Source             string
}

func (d RuntimeDriveObservedData) Normalized() RuntimeDriveObservedData {
	d.Kind = RuntimeDriveObservationKind(strings.TrimSpace(string(d.Kind)))
	d.FailureCategory = strings.TrimSpace(d.FailureCategory)
	d.LeaseState = workorchruntimecontract.RuntimeDriveLeaseState(strings.TrimSpace(string(d.LeaseState)))
	d.Source = strings.TrimSpace(d.Source)
	return d
}

func (d RuntimeDriveObservedData) Validate() error {
	d = d.Normalized()
	if d.WorkspaceID == "" || d.RunID == "" || !d.Kind.Valid() || d.Source == "" {
		return fmt.Errorf("%w: runtime drive observation requires scope, kind, and source", common.ErrInvalidArgument)
	}
	return nil
}
