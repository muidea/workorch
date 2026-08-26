package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAdvanceRuntimeSemanticPlanBatchOwnsForwardLifecycle(t *testing.T) {
	t.Parallel()

	forward := []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchPending,
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete,
		runtimecontract.RuntimeSemanticPlanBatchAccepted,
	}
	batch := runtimecontract.RuntimeSemanticPlanBatch{BatchID: "batch-1", State: forward[0]}
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range forward[1:] {
		advanced, err := advanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			t.Fatalf("advance %q -> %q: %v", batch.State, next, err)
		}
		if advanced.State != next || advanced.Hash != runtimecontract.StableRuntimeSemanticPlanBatchHash(advanced) {
			t.Fatalf("advanced batch = %+v", advanced)
		}
		batch = advanced
	}
	if _, err := advanceRuntimeSemanticPlanBatch(batch, runtimecontract.RuntimeSemanticPlanBatchFailed); err == nil {
		t.Fatal("accepted batch transitioned to failed")
	}
}

func TestAdvanceRuntimeSemanticPlanBatchRejectsSkippedBackAndUnknownTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current runtimecontract.RuntimeSemanticPlanBatchState
		next    runtimecontract.RuntimeSemanticPlanBatchState
	}{
		{name: "skip", current: runtimecontract.RuntimeSemanticPlanBatchPending, next: runtimecontract.RuntimeSemanticPlanBatchAccepted},
		{name: "back edge", current: runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending, next: runtimecontract.RuntimeSemanticPlanBatchRequesting},
		{name: "failed terminal", current: runtimecontract.RuntimeSemanticPlanBatchFailed, next: runtimecontract.RuntimeSemanticPlanBatchRequesting},
		{name: "unknown current", current: "unknown", next: runtimecontract.RuntimeSemanticPlanBatchPending},
		{name: "unknown next", current: runtimecontract.RuntimeSemanticPlanBatchPending, next: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := runtimecontract.RuntimeSemanticPlanBatch{BatchID: "batch-1", State: tt.current}
			batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
			if _, err := advanceRuntimeSemanticPlanBatch(batch, tt.next); err == nil {
				t.Fatalf("transition %q -> %q succeeded", tt.current, tt.next)
			}
		})
	}
}

func TestAdvanceRuntimeSemanticPlanBatchAllowsFailureFromEveryOpenState(t *testing.T) {
	t.Parallel()

	open := []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchPending,
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete,
	}
	for _, current := range open {
		batch := runtimecontract.RuntimeSemanticPlanBatch{BatchID: "batch-1", State: current}
		batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
		advanced, err := advanceRuntimeSemanticPlanBatch(batch, runtimecontract.RuntimeSemanticPlanBatchFailed)
		if err != nil || advanced.State != runtimecontract.RuntimeSemanticPlanBatchFailed {
			t.Fatalf("fail from %q: batch=%+v err=%v", current, advanced, err)
		}
	}
}
