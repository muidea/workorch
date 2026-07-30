package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeV2DriveObservationKind is a process-observability event emitted by
// the Orchestrator owner. It does not express a transition command or a
// second recovery state machine.
type RuntimeV2DriveObservationKind string

const (
	RuntimeV2DriveObservationScheduled           RuntimeV2DriveObservationKind = "drive_scheduled"
	RuntimeV2DriveObservationRunning             RuntimeV2DriveObservationKind = "drive_running"
	RuntimeV2DriveObservationRetryTimerScheduled RuntimeV2DriveObservationKind = "retry_timer_scheduled"
	RuntimeV2DriveObservationRetryTimerFired     RuntimeV2DriveObservationKind = "retry_timer_fired"
	RuntimeV2DriveObservationCASReload           RuntimeV2DriveObservationKind = "cas_reload"
	RuntimeV2DriveObservationSemanticRejection   RuntimeV2DriveObservationKind = "semantic_rejection"
	RuntimeV2DriveObservationDeliveryPending     RuntimeV2DriveObservationKind = "delivery_pending"
)

func (k RuntimeV2DriveObservationKind) Valid() bool {
	switch k {
	case RuntimeV2DriveObservationScheduled,
		RuntimeV2DriveObservationRunning,
		RuntimeV2DriveObservationRetryTimerScheduled,
		RuntimeV2DriveObservationRetryTimerFired,
		RuntimeV2DriveObservationCASReload,
		RuntimeV2DriveObservationSemanticRejection,
		RuntimeV2DriveObservationDeliveryPending:
		return true
	default:
		return false
	}
}

// RuntimeV2DriveObservedData is immutable, bounded telemetry for one
// owner-local SM-4 observation. AggregateRevision may be zero only before an
// aggregate is readable (for example, activation scheduling); the explicit
// field still distinguishes that case from an omitted revision. It is not
// persisted and can never be used to drive a transition.
type RuntimeV2DriveObservedData struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	Kind               RuntimeV2DriveObservationKind
	AggregateRevision  uint64
	RecoveryGeneration uint64
	FailureCategory    string
	LeaseState         workorchruntimecontract.RuntimeDriveLeaseState
	Source             string
}

func (d RuntimeV2DriveObservedData) Normalized() RuntimeV2DriveObservedData {
	d.Kind = RuntimeV2DriveObservationKind(strings.TrimSpace(string(d.Kind)))
	d.FailureCategory = strings.TrimSpace(d.FailureCategory)
	d.LeaseState = workorchruntimecontract.RuntimeDriveLeaseState(strings.TrimSpace(string(d.LeaseState)))
	d.Source = strings.TrimSpace(d.Source)
	return d
}

func (d RuntimeV2DriveObservedData) Validate() error {
	d = d.Normalized()
	if d.WorkspaceID == "" || d.RunID == "" || !d.Kind.Valid() || d.Source == "" {
		return fmt.Errorf("%w: runtime drive observation requires scope, kind, and source", common.ErrInvalidArgument)
	}
	return nil
}
