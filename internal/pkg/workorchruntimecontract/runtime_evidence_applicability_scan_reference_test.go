package workorchruntimecontract

// Frozen pre-change predicates. Keep the original independent nested scans.
// referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasStaleApplicability is a pure audit predicate. Missing
// observations do not prove currentness. Once an exact source was observed
// stale, a later byte restoration cannot silently revive its old credit.
func referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasStaleApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoff string) bool {
	if fact.ApplicabilityDependencies == nil {
		return false
	}
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID != fact.ID || observation.Request.SourceEvidenceHash != StableRuntimeEvidenceFactHash(fact) || (cutoff != "" && observation.Request.EvidenceRevision > cutoff) {
			continue
		}
		if observation.Missing {
			return true
		}
		if observation.CurrentDependencies == nil {
			return true
		}
		matches, err := RuntimeOwnerRevisionSetMatchesCurrent(fact.ApplicabilityDependencies.OwnerRevisionSet, *observation.CurrentDependencies)
		if err != nil || !matches {
			return true
		}
	}
	return false
}

// Evidence with resource dependencies requires an actual matching SM-2 owner
// observation. Dependency-free evidence has no resource to reobserve; nil
// never fabricates an observation for dependent evidence.
func referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasObservedCurrentApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoff string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	if referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasStaleApplicability(fact, observations, cutoff) {
		return false
	}
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == StableRuntimeEvidenceFactHash(fact) &&
			(cutoff == "" || observation.Request.EvidenceRevision <= cutoff) && !observation.Missing && observation.CurrentDependencies != nil {
			return true
		}
	}
	return false
}

// referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability is the consumption fence, not the
// historical audit predicate above. A matching old observation cannot satisfy
// another evidence revision or Resume/reconciliation generation. Historical
// assessment consumers pass their immutable pre-seal generation and prefix.
func referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, evidenceRevision, reconciliationRequestHash string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	if evidenceRevision == "" || referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasStaleApplicability(fact, observations, evidenceRevision) {
		return false
	}
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == StableRuntimeEvidenceFactHash(fact) &&
			observation.Request.EvidenceRevision == evidenceRevision && observation.Request.ReconciliationRequestHash == reconciliationRequestHash &&
			!observation.Missing && observation.CurrentDependencies != nil {
			matches, err := RuntimeOwnerRevisionSetMatchesCurrent(fact.ApplicabilityDependencies.OwnerRevisionSet, *observation.CurrentDependencies)
			if err == nil && matches {
				return true
			}
		}
	}
	return false
}

// Facts-only proof predicates have no reconciliation ledger authority. Require
// an exact evidence revision here; aggregate admission additionally checks the
// exact reconciliation generation with referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability.
func referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasObservedApplicabilityAtRevision(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, evidenceRevision string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID == fact.ID && observation.Request.EvidenceRevision == evidenceRevision &&
			referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability(fact, observations, evidenceRevision, observation.Request.ReconciliationRequestHash) {
			return true
		}
	}
	return false
}
