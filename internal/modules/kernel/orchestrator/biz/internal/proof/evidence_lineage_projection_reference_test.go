package proof

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// referenceLineageBeforeProjectionCoverageBindingForCoverage returns the one activation-sealed proof binding
// for a CWI in the supplied focus. It does not derive or repair a binding.
func referenceLineageBeforeProjectionCoverageBindingForCoverage(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	return referenceLineageBeforeProjectioncoverageBindingForNormalizedFocus(focus.Normalized(), coverageID)
}

func referenceLineageBeforeProjectioncoverageBindingForNormalizedFocus(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	for _, binding := range focus.CoverageEvidenceBindings {
		if binding.CoverageWorkItemID == coverageID {
			return binding.Clone(), true
		}
	}
	return runtimecontract.CoverageEvidenceBinding{}, false
}

// referenceLineageBeforeProjectionCurrentCWIBindingForFocus is the pure canonical binding fence shared by
// admission and evidence attribution. It accepts only a current-schema ledger
// row whose persisted binding identity exactly matches the focus binding.
func referenceLineageBeforeProjectionCurrentCWIBindingForFocus(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeCoverageItemFact, bool) {
	facts = facts.Normalized()
	return referenceLineageBeforeProjectioncurrentCWIBindingForNormalizedFacts(facts, coverageID)
}

func referenceLineageBeforeProjectioncurrentCWIBindingForNormalizedFacts(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeCoverageItemFact, bool) {
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent || coverageID == "" {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	var item runtimecontract.RuntimeCoverageItemFact
	found := false
	for _, candidate := range facts.Coverage {
		if candidate.ID == coverageID {
			item = candidate
			found = true
			break
		}
	}
	if !found || item.CurrentBindingRef == nil {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	current := item.CurrentBindingRef.Normalized()
	if err := current.Validate(); err != nil {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	binding, found := referenceLineageBeforeProjectioncoverageBindingForNormalizedFocus(facts.Focus, coverageID)
	if !found {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	wanted := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, current.SemanticManifestHash)
	if !runtimecontract.SameRuntimeCWIBindingRef(current, wanted) {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	return item, true
}

// referenceLineageBeforeProjectionEvidenceAppliesToCoverageWorkItemWithLineage recognizes old-binding evidence
// only after an equivalent resolution to the exact current binding has been
// persisted. It returns a proof finding and never grants CWI credit itself.
func referenceLineageBeforeProjectionEvidenceAppliesToCoverageWorkItemWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = facts.Normalized()
	return referenceLineageBeforeProjectionevidenceAppliesToCoverageWorkItemWithNormalizedFacts(facts, evidence, coverageID, lineage)
}

func referenceLineageBeforeProjectionevidenceAppliesToCoverageWorkItemWithNormalizedFacts(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	if !runtimecontract.RuntimeEvidenceIsHistoricalEffect(evidence) && runtimecontract.RuntimeEvidenceHasStaleApplicability(evidence, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
		return false
	}
	item, bound := referenceLineageBeforeProjectioncurrentCWIBindingForNormalizedFacts(facts, coverageID)
	if !bound || (item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound &&
		item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled) {
		return false
	}
	if runtimecontract.EvidenceAppliesToCoverageWorkItem(facts.Focus, evidence, coverageID) {
		if source, found := referenceLineageBeforeProjectionCoverageProofLineageSourceForEvidence(lineage, facts, evidence, coverageID); found {
			binding, bindingFound := referenceLineageBeforeProjectioncoverageBindingForNormalizedFocus(facts.Focus, coverageID)
			if bindingFound && source.SourceBindingRef == binding.ID && source.SourceBindingHash == binding.Hash {
				return true
			}
		} else {
			return true
		}
	}
	if !CoverageWorkItemIncluded(evidence.CoverageWorkItemIDs, coverageID) || WeakEvidence(evidence) {
		return false
	}
	return referenceLineageBeforeProjectionevidenceHasEquivalentCoverageProofLineage(facts, evidence, coverageID, lineage)
}

// referenceLineageBeforeProjectionEvidenceAppliesToCoverageIDsWithLineage narrows attribution to a sealed
// action's explicit current-focus CWI set.
func referenceLineageBeforeProjectionEvidenceAppliesToCoverageIDsWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageIDs []common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = facts.Normalized()
	if len(coverageIDs) == 0 || facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent {
		return false
	}
	for _, coverageID := range coverageIDs {
		if !CoverageWorkItemIncluded(facts.Focus.CoverageWorkItemIDs, coverageID) {
			continue
		}
		if referenceLineageBeforeProjectionevidenceAppliesToCoverageWorkItemWithNormalizedFacts(facts, evidence, coverageID, lineage) {
			return true
		}
	}
	return false
}

// referenceLineageBeforeProjectionEvidenceAppliesToCurrentCoverageWithLineage applies the same proof gate to
// the current focus's complete CWI set.
func referenceLineageBeforeProjectionEvidenceAppliesToCurrentCoverageWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	return referenceLineageBeforeProjectionEvidenceAppliesToCoverageIDsWithLineage(facts, evidence, facts.Focus.CoverageWorkItemIDs, lineage)
}

// referenceLineageBeforeProjectionAcceptedEvidenceCreditsCurrentFocusWithLineage is the single
// coverage-sensitive gate for accepted ordinary evidence. A fact without
// reducer-derived CWI credit remains diagnostic lineage only and cannot
// satisfy a required kind, stale an assessment, or close a repair directive.
func referenceLineageBeforeProjectionAcceptedEvidenceCreditsCurrentFocusWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	if !runtimecontract.RuntimeEvidenceHasProofApplicabilityAtRevision(evidence, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
		return false
	}
	return referenceLineageBeforeProjectionEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, lineage)
}

// referenceLineageBeforeProjectionEvidenceHasEquivalentCoverageProofLineage reports whether old-binding
// evidence has an exact persisted equivalent resolution to the current proof.
func referenceLineageBeforeProjectionEvidenceHasEquivalentCoverageProofLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = facts.Normalized()
	return referenceLineageBeforeProjectionevidenceHasEquivalentCoverageProofLineage(facts, evidence, coverageID, lineage)
}

func referenceLineageBeforeProjectionevidenceHasEquivalentCoverageProofLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	if len(lineage) == 0 || evidence.ID == "" || evidence.Kind == "" {
		return false
	}
	currentBinding, found := referenceLineageBeforeProjectioncoverageBindingForNormalizedFocus(facts.Focus, coverageID)
	if !found {
		return false
	}
	currentIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(currentBinding, evidence.Kind)
	if err != nil {
		return false
	}
	if source, found := referenceLineageBeforeProjectionCoverageProofLineageSourceForEvidence(lineage, facts, evidence, coverageID); found {
		return SameProofLineageResolution(lineage, source.ID, currentBinding, currentIdentity)
	}
	return false
}

// referenceLineageBeforeProjectionCoverageProofLineageSourceForEvidence returns the exact aggregate-local
// source row for one evidence/CWI/binding identity.
func referenceLineageBeforeProjectionCoverageProofLineageSourceForEvidence(
	lineage []runtimecontract.RuntimeCoverageProofLineage,
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
) (runtimecontract.RuntimeCoverageProofLineage, bool) {
	for _, source := range lineage {
		source = source.Normalized()
		if source.Kind != runtimecontract.RuntimeCoverageProofLineageKindSource ||
			source.WorkspaceID != facts.WorkspaceID || source.RunID != facts.RunID ||
			source.SourceEvidenceID != evidence.ID || source.CoverageWorkItemID != coverageID ||
			source.EvidenceKind != evidence.Kind || !EvidenceReferencesBinding(evidence, source.SourceBindingRef) {
			continue
		}
		return source, true
	}
	return runtimecontract.RuntimeCoverageProofLineage{}, false
}
