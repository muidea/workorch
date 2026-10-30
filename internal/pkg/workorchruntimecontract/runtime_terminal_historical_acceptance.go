package workorchruntimecontract

import (
	"reflect"
	"slices"
)

// This is a read-only material consumer, not another settlement authority.
// Every mapped endpoint is reconstructed from the immutable original sources;
// the ordinary consumer retains its own native checks, receipt and binding.
func runtimeTerminalHistoricalAcceptance(source RuntimeStateRecord, coverage RuntimeCoverageItemFact) (RuntimeHistoricalAcceptanceProof, CoverageEvidenceBinding, bool) {
	if coverage.HistoricalBehavior == nil || coverage.State != RuntimeCoverageItemStateSatisfied || coverage.CWIConvergenceState != RuntimeCWIConvergenceStateSettled || len(coverage.HistoricalAcceptanceProofs) == 0 {
		return RuntimeHistoricalAcceptanceProof{}, CoverageEvidenceBinding{}, false
	}
	proof := coverage.HistoricalAcceptanceProofs[len(coverage.HistoricalAcceptanceProofs)-1]
	sources := runtimeLineageSourcesFromRecord(source)
	if validateRuntimeHistoricalAcceptanceProofs(sources, coverage) != nil {
		return RuntimeHistoricalAcceptanceProof{}, CoverageEvidenceBinding{}, false
	}
	compiled, err := CompileRuntimeHistoricalAcceptanceProof(sources, coverage.ID, proof.AssessmentDispatchID)
	if err != nil || !reflect.DeepEqual(proof, compiled) {
		return RuntimeHistoricalAcceptanceProof{}, CoverageEvidenceBinding{}, false
	}
	var consumer RuntimeCoverageItemFact
	matches := 0
	for _, row := range source.Facts.Coverage {
		if row.ID == proof.Source.AcceptanceCoverageWorkItemID {
			consumer, matches = row, matches+1
		}
	}
	binding, found := runtimeTerminalCurrentCoverageBinding(source.Facts, consumer)
	if matches != 1 || !found {
		return RuntimeHistoricalAcceptanceProof{}, CoverageEvidenceBinding{}, false
	}
	for _, ref := range proof.AcceptanceEvidenceRefs {
		matches = 0
		for _, fact := range source.Facts.Evidence.Accepted {
			if string(fact.ID) == ref && runtimeTerminalEvidenceMatchesCurrentProof(source, fact, binding) {
				matches++
			}
		}
		if matches != 1 {
			return RuntimeHistoricalAcceptanceProof{}, CoverageEvidenceBinding{}, false
		}
	}
	return proof, binding, true
}

func runtimeTerminalEvidenceMatchesCoverageProof(source RuntimeStateRecord, fact EvidenceFact, coverage RuntimeCoverageItemFact, binding CoverageEvidenceBinding) bool {
	if coverage.HistoricalBehavior == nil {
		return runtimeTerminalEvidenceMatchesCurrentProof(source, fact, binding)
	}
	proof, ordinary, valid := runtimeTerminalHistoricalAcceptance(source, coverage)
	if !valid {
		return false
	}
	if slices.Contains(proof.BehaviorEvidenceRefs, string(fact.ID)) {
		return RuntimeEvidenceIsHistoricalEffect(fact) && runtimeTerminalEvidenceMatchesProofSource(source, fact, binding)
	}
	return slices.Contains(proof.AcceptanceEvidenceRefs, string(fact.ID)) && !RuntimeEvidenceIsHistoricalEffect(fact) && runtimeTerminalEvidenceMatchesCurrentProof(source, fact, ordinary)
}

// Retain the historical check identity in public material, but consume only
// its independently derived consumer check. Exact source fields, not similar
// prose or reused native names, establish this mapping.
func runtimeTerminalHistoricalSourceCheck(source RuntimeStateRecord, coverage RuntimeCoverageItemFact, original RuntimeSourceObligationCheck) (RuntimeSourceObligationCheck, bool) {
	endpoints, err := ResolveRuntimeEvidencePurposeEndpoints(source.Definition, source.Facts.Activation.ExecutionContract, source.Facts.DefinitionSeed, coverage.ID)
	if err != nil {
		return RuntimeSourceObligationCheck{}, false
	}
	var check RuntimeSourceObligationCheck
	matches := 0
	for _, value := range endpoints.Acceptance.SourceObligationChecks {
		comparison := value
		comparison.CheckRef = original.CheckRef
		if comparison == original && value.CheckRef != original.CheckRef {
			check, matches = value, matches+1
		}
	}
	return check, matches == 1
}
