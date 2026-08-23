package workspaceviewcore

import "fmt"

// projectionLifecycleInternalEventKind is private to the Workorchd workspace
// projection owner. Callers report observed projection facts; they never set a
// lifecycle state directly.
type projectionLifecycleInternalEventKind string

const (
	projectionLifecycleRebuildRequested projectionLifecycleInternalEventKind = "rebuild_requested"
	projectionLifecycleRebuildCompleted projectionLifecycleInternalEventKind = "rebuild_completed"
	projectionLifecycleMutationApplied  projectionLifecycleInternalEventKind = "mutation_applied"
	projectionLifecycleDegradedObserved projectionLifecycleInternalEventKind = "degraded_observed"
	projectionLifecycleFailureObserved  projectionLifecycleInternalEventKind = "failure_observed"
	projectionLifecycleStopObserved     projectionLifecycleInternalEventKind = "stop_observed"
)

type projectionLifecycleInternalEvent struct {
	Kind              projectionLifecycleInternalEventKind
	HasProjectionData bool
}

// reduceProjectionLifecycleState is the sole lifecycle transition table for
// the derived workspace projection. It does not rebuild or mutate Runtime
// authority; Workorchd owns this state only for read readiness and health.
func reduceProjectionLifecycleState(current projectionLifecycleState, event projectionLifecycleInternalEvent) (projectionLifecycleState, error) {
	switch current {
	case StateBuilding, StateReady, StateStale, StateFailed, StateStopped:
		// Stored lifecycle states. StateDegraded is derived at read time.
	default:
		return current, fmt.Errorf("unknown stored projection lifecycle state %q", current)
	}
	switch event.Kind {
	case projectionLifecycleRebuildRequested:
		if current == StateStopped {
			return current, nil
		}
		if event.HasProjectionData {
			return StateStale, nil
		}
		return StateBuilding, nil
	case projectionLifecycleRebuildCompleted:
		if current == StateStopped {
			return current, nil
		}
		return StateReady, nil
	case projectionLifecycleMutationApplied:
		switch current {
		case StateStopped:
			return current, nil
		default:
			// A successfully committed owner mutation is positive recovery
			// evidence. It may recover failed projection readiness; the
			// caller still clears the matching failure record in the same
			// Store transaction.
			return StateReady, nil
		}
	case projectionLifecycleDegradedObserved:
		switch current {
		case StateBuilding, StateStopped, StateFailed:
			return current, nil
		default:
			// Degraded is derived from the current-generation failure set in
			// statusLocked; the lifecycle remains ready for partial reads.
			return StateReady, nil
		}
	case projectionLifecycleFailureObserved:
		if current == StateStopped {
			return current, nil
		}
		return StateFailed, nil
	case projectionLifecycleStopObserved:
		return StateStopped, nil
	default:
		return current, fmt.Errorf("unknown projection lifecycle internal event %q", event.Kind)
	}
}

// submitProjectionLifecycleInternalEventLocked is the only writer of
// Store.state after construction. The caller must hold s.mu.
func (s *Store) submitProjectionLifecycleInternalEventLocked(event projectionLifecycleInternalEvent) error {
	if s == nil {
		return nil
	}
	next, err := reduceProjectionLifecycleState(s.state, event)
	if err != nil {
		return err
	}
	s.state = next
	return nil
}
