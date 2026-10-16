package workorchruntimecontract

import "fmt"

// CompileRuntimeTerminalOwnerReadDependencies keeps historical reads on each
// evidence item, but unions only SM-2-observed applicability for versioned
// evidence. It never promotes mutation inputs to current postconditions or
// picks a latest conflicting version. Full source validation remains required.
func CompileRuntimeTerminalOwnerReadDependencies(source RuntimeStateRecord) (RuntimeOwnerRevisionSet, error) {
	var sets []RuntimeOwnerRevisionSet
	selected := map[string]bool{}
	current := map[string]CoverageEvidenceBinding{}
	for _, coverage := range source.Facts.Coverage {
		binding, found := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage)
		if !found {
			continue
		}
		current[string(coverage.ID)] = binding
		for _, fact := range source.Facts.Evidence.Accepted {
			if !runtimeTerminalEvidenceMatchesCurrentProof(source, fact, binding) {
				continue
			}
			selected[string(fact.ID)] = true
		}
	}
	for _, fact := range source.Facts.Evidence.Accepted {
		if !selected[string(fact.ID)] || fact.OwnerRevisionSet == nil {
			continue
		}
		if RuntimeEvidenceIsHistoricalEffect(fact) {
			// Both input reads and original postcondition remain on the formal
			// item. Neither is a dependency of the current acceptance resource.
			continue
		}
		if fact.ApplicabilityDependencies != nil {
			// The current proof predicate above already requires a matching owner
			// observation. Use its actual set, not a newly fabricated readback.
			var observed *RuntimeOwnerRevisionSet
			for _, observation := range source.Facts.EvidenceApplicabilityObservations {
				if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == StableRuntimeEvidenceFactHash(fact) && observation.Request.EvidenceRevision == source.Facts.EvidenceRevision && observation.Request.ReconciliationRequestHash == source.Reconciliation.RequestHash && !observation.Missing {
					observed = observation.CurrentDependencies
				}
			}
			if observed == nil {
				return RuntimeOwnerRevisionSet{}, fmt.Errorf("terminal current evidence lacks original SM-2 applicability observation")
			}
			sets = append(sets, *CloneRuntimeOwnerRevisionSet(observed))
		} else {
			sets = append(sets, *CloneRuntimeOwnerRevisionSet(fact.OwnerRevisionSet))
		}
	}
	// Latest original assessment per CWI, with all selected proofs still
	// applicable. Never choose an older satisfied assessment after a failure.
	latest := map[string]string{}
	for _, fact := range source.Facts.Assessments {
		for _, id := range fact.CoverageWorkItemIDs {
			latest[string(id)] = fact.ID
		}
	}
	for _, fact := range source.Facts.Assessments {
		if fact.Disposition != AssessmentDispositionSatisfied {
			continue
		}
		applicable := len(fact.CoverageWorkItemIDs) > 0
		for _, id := range fact.CoverageWorkItemIDs {
			_, exists := current[string(id)]
			applicable = applicable && exists && latest[string(id)] == fact.ID
		}
		if !applicable {
			continue
		}
		for _, dispatch := range source.CompletedDispatches {
			adjudication := dispatch.AssessmentAdjudication
			if dispatch.State != RuntimeDispatchStateCommitted || adjudication == nil || dispatch.AssessmentAdjudicationHash != fact.AdjudicationHash || fact.AdjudicationHash == "" {
				continue
			}
			if err := adjudication.ValidateRaw(); err != nil {
				return RuntimeOwnerRevisionSet{}, err
			}
			if StableRuntimeAssessmentAdjudicationHash(*adjudication) != fact.AdjudicationHash {
				return RuntimeOwnerRevisionSet{}, fmt.Errorf("terminal assessment read source hash mismatch")
			}
			for _, proof := range adjudication.Proofs {
				if !selected[string(proof.EvidenceFactID)] {
					applicable = false
				}
			}
			if !applicable {
				continue
			}
			for _, observed := range adjudication.OwnerFactSources {
				if observed.OwnerRevisionSet != nil {
					sets = append(sets, *CloneRuntimeOwnerRevisionSet(observed.OwnerRevisionSet))
				}
			}
			for _, proof := range adjudication.Proofs {
				// Original mutation inputs are historical, not terminal current
				// dependencies. Their exact pair remains in the formal proof item.
				if proof.OwnerRevisionSet != nil && proof.ApplicabilityDependencies == nil {
					sets = append(sets, *CloneRuntimeOwnerRevisionSet(proof.OwnerRevisionSet))
				}
			}
		}
	}
	return CompileRuntimeOwnerRevisionSetUnion(source.Facts.DefinitionRevision, sets...)
}
