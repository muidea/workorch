package proof

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// ReconciliationBindingFacts is a detached comparison result. It records the
// exact owner observations for one binding without choosing retain, reopen,
// prerequisite, or any other SM-2 transition.
type ReconciliationBindingFacts struct {
	Observations       []runtimecontract.RuntimeCWIReconciliationObservation
	AllOwnersSatisfied bool
	ObservationRef     string
	UnsatisfiedReason  string
}

// ReconciliationHistoryContainsExactBatch recognizes an already persisted
// owner batch only when its hash and request identity both match. It is a pure
// duplicate-delivery fence and does not reactivate a historical generation.
func ReconciliationHistoryContainsExactBatch(history []runtimecontract.RuntimeCWIReconciliationGeneration, batch runtimecontract.RuntimeCWIReconciliationBatch) bool {
	batch = batch.Normalized()
	if batch.Hash == "" {
		return false
	}
	for _, generation := range history {
		generation = generation.Normalized()
		if generation.Batch == nil || generation.Batch.Hash != batch.Hash {
			continue
		}
		if batch.ValidateForRequest(generation.Request) == nil {
			return true
		}
	}
	return false
}

// CompareReconciliationBinding gathers the complete factual observation set
// for a sealed binding. The caller remains responsible for checking that the
// batch is valid for the request before mapping these facts to an SM-2 result.
func CompareReconciliationBinding(binding runtimecontract.RuntimeCWIReconciliationBindingRef, batch runtimecontract.RuntimeCWIReconciliationBatch) ReconciliationBindingFacts {
	binding = binding.Normalized()
	batch = batch.Normalized()
	observations := make([]runtimecontract.RuntimeCWIReconciliationObservation, 0, len(binding.OwnerModuleIDs))
	for _, observation := range batch.Observations {
		if observation.CoverageWorkItemID == binding.CoverageWorkItemID && observation.BindingID == binding.BindingID {
			observations = append(observations, observation.Clone())
		}
	}
	allSatisfied := len(observations) == len(binding.OwnerModuleIDs)
	for _, observation := range observations {
		if observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
			allSatisfied = false
		}
	}
	return ReconciliationBindingFacts{
		Observations:       observations,
		AllOwnersSatisfied: allSatisfied,
		ObservationRef:     runtimecontract.StableRuntimeCWIReconciliationObservationSetRef(observations),
		UnsatisfiedReason:  ReconciliationObservationReason(observations),
	}
}

// ReconciliationObservationReason summarizes owner observation facts without
// selecting whether an already satisfied CWI is reopened.
func ReconciliationObservationReason(observations []runtimecontract.RuntimeCWIReconciliationObservation) string {
	for _, observation := range observations {
		if observation.Status == runtimecontract.RuntimeCWIReconciliationObservationStale {
			return "current-run owner observation is stale"
		}
		if observation.Status == runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
			return "current-run owner observation is unverifiable"
		}
	}
	return "current-run owner observation no longer satisfies the sealed proof"
}
