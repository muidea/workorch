package workspaceviewcore

import "testing"

func TestReduceProjectionLifecycleStateOwnsTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current projectionLifecycleState
		event   projectionLifecycleInternalEvent
		want    projectionLifecycleState
	}{
		{
			name:    "initial rebuild remains building",
			current: StateBuilding,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildRequested},
			want:    StateBuilding,
		},
		{
			name:    "rebuild with prior projection becomes stale",
			current: StateReady,
			event: projectionLifecycleInternalEvent{
				Kind:              projectionLifecycleRebuildRequested,
				HasProjectionData: true,
			},
			want: StateStale,
		},
		{
			name:    "completed rebuild recovers failed projection",
			current: StateFailed,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted},
			want:    StateReady,
		},
		{
			name:    "successful owner mutation recovers failed lifecycle",
			current: StateFailed,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied},
			want:    StateReady,
		},
		{
			name:    "degraded fact preserves initial build",
			current: StateBuilding,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleDegradedObserved},
			want:    StateBuilding,
		},
		{
			name:    "failure moves active projection to failed",
			current: StateReady,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleFailureObserved},
			want:    StateFailed,
		},
		{
			name:    "stop is terminal for late rebuild completion",
			current: StateStopped,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted},
			want:    StateStopped,
		},
		{
			name:    "stop is terminal for late failure",
			current: StateStopped,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleFailureObserved},
			want:    StateStopped,
		},
		{
			name:    "stop event is idempotent",
			current: StateStopped,
			event:   projectionLifecycleInternalEvent{Kind: projectionLifecycleStopObserved},
			want:    StateStopped,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := reduceProjectionLifecycleState(test.current, test.event)
			if err != nil {
				t.Fatalf("reduce lifecycle state: %v", err)
			}
			if got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReduceProjectionLifecycleStateRejectsUnknownVocabulary(t *testing.T) {
	if _, err := reduceProjectionLifecycleState("unknown", projectionLifecycleInternalEvent{Kind: projectionLifecycleStopObserved}); err == nil {
		t.Fatal("expected unknown stored state to be rejected")
	}
	if _, err := reduceProjectionLifecycleState(StateReady, projectionLifecycleInternalEvent{Kind: "set_ready"}); err == nil {
		t.Fatal("expected target-state event vocabulary to be rejected")
	}
}
