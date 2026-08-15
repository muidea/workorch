package proof

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	facts = facts.Normalized()
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
	facts = facts.Normalized()
	return evidenceAppliesToCoverageWorkItemWithNormalizedFacts(facts, evidence, coverageID, lineage)
}

func evidenceAppliesToCoverageWorkItemWithNormalizedFacts(
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

// EvidenceAppliesToCoverageIDsWithLineage narrows attribution to a sealed
// action's explicit current-focus CWI set.
func EvidenceAppliesToCoverageIDsWithLineage(
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

// AcceptedEvidenceAppliesToCurrentFocusWithLineage retains the narrow
// non-credit current-state path while binding CWI-naming evidence to the
// current proof lineage.
func AcceptedEvidenceAppliesToCurrentFocusWithLineage(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact, lineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	if len(evidence.CoverageWorkItemIDs) == 0 {
		return runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence)
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
	facts = facts.Normalized()
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
