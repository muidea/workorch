package proof

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This pure predicate reads only these fields. Project before normalization so
// testing one evidence/CWI pair does not copy unrelated provider diagnostics,
// catalogs, work/Definition declarations or the entire evidence ledger. The
// actual owner admission still validates its complete raw record separately.
// Keep the original normalizers and all current-binding/applicability checks;
// this projection is neither a validated aggregate nor a retained result.
func normalizedEvidenceLineageFacts(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	return (runtimecontract.RuntimeFacts{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		EvidenceRevision:       facts.EvidenceRevision,
		CWILedgerSchemaVersion: facts.CWILedgerSchemaVersion,
		Coverage:               facts.Coverage, Focus: facts.Focus,
		EvidenceApplicabilityObservations: facts.EvidenceApplicabilityObservations,
	}).Normalized()
}

// CoverageBindingForCoverage returns the one activation-sealed proof binding
// for a CWI in the supplied focus. It does not derive or repair a binding.
func CoverageBindingForCoverage(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	return coverageBindingForNormalizedFocus(focus.Normalized(), coverageID)
}

func coverageBindingForNormalizedFocus(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	for _, binding := range focus.CoverageEvidenceBindings {
		if binding.CoverageWorkItemID == coverageID {
			return binding.Clone(), true
		}
	}
	return runtimecontract.CoverageEvidenceBinding{}, false
}

// CurrentCWIBindingForFocus is the pure canonical binding fence shared by
// admission and evidence attribution. It accepts only a current-schema ledger
// row whose persisted binding identity exactly matches the focus binding.
func CurrentCWIBindingForFocus(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeCoverageItemFact, bool) {
	facts = normalizedEvidenceLineageFacts(facts)
	return currentCWIBindingForNormalizedFacts(facts, coverageID)
}

func currentCWIBindingForNormalizedFacts(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeCoverageItemFact, bool) {
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
	binding, found := coverageBindingForNormalizedFocus(facts.Focus, coverageID)
	if !found {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	wanted := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, current.SemanticManifestHash)
	if !runtimecontract.SameRuntimeCWIBindingRef(current, wanted) {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	return item, true
}

// EvidenceAppliesToCoverageWorkItemWithLineage recognizes old-binding evidence
// only after an equivalent resolution to the exact current binding has been
// persisted. It returns a proof finding and never grants CWI credit itself.
func EvidenceAppliesToCoverageWorkItemWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = normalizedEvidenceLineageFacts(facts)
	return evidenceAppliesToCoverageWorkItemWithNormalizedFacts(facts, evidence, coverageID, lineage)
}

func evidenceAppliesToCoverageWorkItemWithNormalizedFacts(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	if !evidenceHasCurrentProofApplicability(facts, evidence) {
		return false
	}
	return evidenceMatchesCurrentCoverageBinding(facts, evidence, coverageID, lineage)
}

func evidenceHasCurrentProofApplicability(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact) bool {
	return runtimecontract.RuntimeEvidenceIsHistoricalEffect(evidence) || !runtimecontract.RuntimeEvidenceHasStaleApplicability(evidence, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision)
}

func evidenceMatchesCurrentCoverageBinding(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	item, bound := currentCWIBindingForNormalizedFacts(facts, coverageID)
	if !bound || (item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound &&
		item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled) {
		return false
	}
	if runtimecontract.EvidenceAppliesToCoverageWorkItem(facts.Focus, evidence, coverageID) {
		if source, found := CoverageProofLineageSourceForEvidence(lineage, facts, evidence, coverageID); found {
			binding, bindingFound := coverageBindingForNormalizedFocus(facts.Focus, coverageID)
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
	return evidenceHasEquivalentCoverageProofLineage(facts, evidence, coverageID, lineage)
}

// EvidenceAppliesToAnyFocusWithLineage checks the same attribution predicate
// across the original current/remaining/completed focus order. The common
// ledger and observations are normalized once for this call; each focus is
// normalized once, without changing its original member iteration. This is a
// pure finding, not owner admission, credit or a result retained across calls.
func EvidenceAppliesToAnyFocusWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	commonFacts := facts
	commonFacts.Focus = runtimecontract.CoverageFocus{}
	var normalized *runtimecontract.RuntimeFacts
	applicable := false
	check := func(focus runtimecontract.CoverageFocus) bool {
		if len(focus.CoverageWorkItemIDs) == 0 {
			return false
		}
		if normalized == nil {
			value := normalizedEvidenceLineageFacts(commonFacts)
			normalized = &value
			// Focus-independent, including the original historical-purpose
			// exception. Compare the same normalized revision/observations as
			// the single-member predicate, without reviving any stale source.
			applicable = evidenceHasCurrentProofApplicability(value, evidence)
		}
		if !applicable {
			return false
		}
		probe := *normalized
		probe.Focus = focus.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if evidenceMatchesCurrentCoverageBinding(probe, evidence, coverageID, lineage) {
				return true
			}
		}
		return false
	}
	if check(facts.Focus) {
		return true
	}
	for _, focuses := range [][]runtimecontract.CoverageFocus{facts.RemainingFocuses, facts.CompletedFocuses} {
		for _, focus := range focuses {
			if check(focus) {
				return true
			}
		}
	}
	return false
}

// EvidenceAppliesToCoverageIDsWithLineage narrows attribution to a sealed
// action's explicit current-focus CWI set.
func EvidenceAppliesToCoverageIDsWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageIDs []common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = normalizedEvidenceLineageFacts(facts)
	if len(coverageIDs) == 0 || facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent {
		return false
	}
	for _, coverageID := range coverageIDs {
		if !CoverageWorkItemIncluded(facts.Focus.CoverageWorkItemIDs, coverageID) {
			continue
		}
		if evidenceAppliesToCoverageWorkItemWithNormalizedFacts(facts, evidence, coverageID, lineage) {
			return true
		}
	}
	return false
}

// EvidenceAppliesToCurrentCoverageWithLineage applies the same proof gate to
// the current focus's complete CWI set.
func EvidenceAppliesToCurrentCoverageWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	return EvidenceAppliesToCoverageIDsWithLineage(facts, evidence, facts.Focus.CoverageWorkItemIDs, lineage)
}

// AcceptedEvidenceCreditsCurrentFocusWithLineage is the single
// coverage-sensitive gate for accepted ordinary evidence. A fact without
// reducer-derived CWI credit remains diagnostic lineage only and cannot
// satisfy a required kind, stale an assessment, or close a repair directive.
func AcceptedEvidenceCreditsCurrentFocusWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	if !runtimecontract.RuntimeEvidenceHasProofApplicabilityAtRevision(evidence, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
		return false
	}
	return EvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, lineage)
}

// EvidenceHasEquivalentCoverageProofLineage reports whether old-binding
// evidence has an exact persisted equivalent resolution to the current proof.
func EvidenceHasEquivalentCoverageProofLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	facts = normalizedEvidenceLineageFacts(facts)
	return evidenceHasEquivalentCoverageProofLineage(facts, evidence, coverageID, lineage)
}

func evidenceHasEquivalentCoverageProofLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	if len(lineage) == 0 || evidence.ID == "" || evidence.Kind == "" {
		return false
	}
	currentBinding, found := coverageBindingForNormalizedFocus(facts.Focus, coverageID)
	if !found {
		return false
	}
	currentIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(currentBinding, evidence.Kind)
	if err != nil {
		return false
	}
	if source, found := CoverageProofLineageSourceForEvidence(lineage, facts, evidence, coverageID); found {
		return SameProofLineageResolution(lineage, source.ID, currentBinding, currentIdentity)
	}
	return false
}

// CoverageProofLineageSourceForEvidence returns the exact aggregate-local
// source row for one evidence/CWI/binding identity.
func CoverageProofLineageSourceForEvidence(
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
