package workorchruntimecontract

import "fmt"

// Accepted semantic inputs are original authority, even while their generation
// is current. New generations may append/retire them, but cannot rewrite them
// together with a new self hash. This is preservation, not source re-admission.
func validateCommittedSemanticSources(before, after RuntimeSemanticPlanningLedger) error {
	if before.Manifest == nil {
		return nil
	}
	for _, batch := range before.Batches {
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			continue
		}
		document, facts, ok := committedSemanticBatchSource(before, batch)
		if !ok {
			// Accepted document bookkeeping can precede the owner feasibility
			// result. It has not produced binding authority yet.
			continue
		}
		matches := func(l RuntimeSemanticPlanningLedger) bool {
			if l.Manifest == nil || !runtimeRawEqual(*l.Manifest, *before.Manifest) {
				return false
			}
			for _, next := range l.Batches {
				if !runtimeRawEqual(batch, next) {
					continue
				}
				d, f, found := committedSemanticBatchSource(l, next)
				if found && runtimeRawEqual(document, d) && runtimeRawEqual(facts, f) {
					return true
				}
			}
			return false
		}
		found := matches(after)
		if !found {
			for _, g := range after.AllHistory() {
				if matches(g.Snapshot) {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("runtime transition rewrites accepted semantic source")
		}
	}
	return nil
}

func committedSemanticBatchSource(l RuntimeSemanticPlanningLedger, b RuntimeSemanticPlanBatch) (RuntimeSemanticPlanDocument, []RuntimeSemanticProofFeasibility, bool) {
	var d RuntimeSemanticPlanDocument
	documents := 0
	for _, v := range l.BatchDocuments {
		if v.BatchID == b.BatchID {
			d = v
			documents++
		}
	}
	if documents == 0 && l.Document != nil && l.Document.BatchID == b.BatchID {
		d = *l.Document
		documents = 1
	}
	if documents != 1 {
		return d, nil, false
	}
	var facts []RuntimeSemanticProofFeasibility
	results := 0
	for _, v := range l.BatchFeasibilityResults {
		if v.BatchID == b.BatchID {
			facts = v.Feasibilities
			results++
		}
	}
	if results == 0 && l.Document != nil && l.Document.BatchID == b.BatchID {
		facts = l.Feasibilities
		results = 1
	}
	return d, facts, results == 1
}
