package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// advanceRuntimeSemanticPlanBatch is the sole SM-3 reducer for the bounded
// semantic-plan batch lifecycle. The shared Runtime contract deliberately
// exposes only the persisted DTO, validation, and stable hashing; it does not
// authorize a transition.
func advanceRuntimeSemanticPlanBatch(batch runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (runtimecontract.RuntimeSemanticPlanBatch, error) {
	batch = batch.Normalized()
	if !runtimeSemanticPlanBatchTransitionAllowed(batch.State, next) {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic plan batch transition %q -> %q is not allowed", batch.State, next)
	}
	advanced := batch
	advanced.State = next
	advanced.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(advanced)
	return advanced, nil
}

func runtimeSemanticPlanBatchTransitionAllowed(current, next runtimecontract.RuntimeSemanticPlanBatchState) bool {
	switch current {
	case runtimecontract.RuntimeSemanticPlanBatchPending:
		return next == runtimecontract.RuntimeSemanticPlanBatchRequesting || next == runtimecontract.RuntimeSemanticPlanBatchFailed
	case runtimecontract.RuntimeSemanticPlanBatchRequesting:
		return next == runtimecontract.RuntimeSemanticPlanBatchDocumentComplete || next == runtimecontract.RuntimeSemanticPlanBatchFailed
	case runtimecontract.RuntimeSemanticPlanBatchDocumentComplete:
		return next == runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending || next == runtimecontract.RuntimeSemanticPlanBatchFailed
	case runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending:
		return next == runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete || next == runtimecontract.RuntimeSemanticPlanBatchFailed
	case runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete:
		return next == runtimecontract.RuntimeSemanticPlanBatchAccepted || next == runtimecontract.RuntimeSemanticPlanBatchFailed
	case runtimecontract.RuntimeSemanticPlanBatchAccepted, runtimecontract.RuntimeSemanticPlanBatchFailed:
		return false
	default:
		return false
	}
}
