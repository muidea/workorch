package runtime

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// prospectiveOwnerFactAcquisitionReceiptCommitted recognizes the exact
// one-shot boundary between a prospective owner-fact acquisition and its
// required owner re-observation. The accepted evidence and source proof
// lineage are both persisted in the Orchestrator aggregate by the receipt CAS;
// together with the current binding they are durable proof that the
// create-only target selection has already been consumed.
//
// This is deliberately narrower than "some state-change evidence exists".
// The proof must be the current CWI's singleton acquisition kind, owner,
// selector and prospective target set. A stale/forged lineage therefore cannot
// suppress ordinary target grounding.
func prospectiveOwnerFactAcquisitionReceiptCommitted(
	facts runtimecontract.RuntimeFacts,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	_, committed := committedProspectiveOwnerFactAcquisition(facts, lineage)
	return committed
}

func committedProspectiveOwnerFactAcquisition(
	facts runtimecontract.RuntimeFacts,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) (runtimecontract.RuntimeOwnerFactConvergence, bool) {
	facts = facts.Normalized()
	lineage = runtimecontract.NormalizeRuntimeCoverageProofLineage(lineage)
	focus := facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 || len(focus.ProspectiveTargetRefs) != 1 || focus.TargetSelectorRef == "" {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	item, bound := currentRuntimeCWIBindingForFocus(facts, coverageID)
	if !bound || item.CurrentBindingRef == nil {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}
	binding, found := runtimecontract.CoverageEvidenceBindingForRef(focus.CoverageEvidenceBindings, item.CurrentBindingRef.BindingID)
	if !found || binding.CoverageWorkItemID != coverageID {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}
	proofIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(binding, runtimecontract.EvidenceKindStateChange)
	if err != nil || proofIdentity.TargetPolicy != runtimecontract.CoverageEvidenceProofTargetPolicyProspectiveOwnerTarget ||
		!sameRuntimeTargetRefs(proofIdentity.ProspectiveTargetRefs, focus.ProspectiveTargetRefs) ||
		proofIdentity.TargetSelectorRef != focus.TargetSelectorRef ||
		proofIdentity.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef ||
		proofIdentity.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}
	convergence, found := prospectiveOwnerFactAcquisitionForProof(focus, proofIdentity)
	if !found {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}

	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind != runtimecontract.EvidenceKindStateChange || evidence.FocusID != focus.ID ||
			len(evidence.BindingRefs) != 1 || evidence.BindingRefs[0] != binding.ID ||
			len(evidence.CoverageWorkItemIDs) != 1 || evidence.CoverageWorkItemIDs[0] != coverageID ||
			!sameRuntimeTargetRefs(evidence.TargetRefs, focus.ProspectiveTargetRefs) || weakRuntimeEvidence(evidence) ||
			!runtimeEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, lineage) {
			continue
		}
		source, sourceFound := proof.CoverageProofLineageSourceForEvidence(lineage, facts, evidence, coverageID)
		if !sourceFound || source.Validate() != nil || source.SourceDispatchID != evidence.DispatchID ||
			source.SourceOperationID != evidence.OperationID || source.SourceBindingRef != binding.ID ||
			source.SourceBindingHash != binding.Hash ||
			!runtimecontract.SameCoverageEvidenceProofIdentity(source.SourceProofIdentity, proofIdentity) {
			continue
		}
		if source.SourceProofIdentity.OwnerModuleID == convergence.AcquisitionOwnerModuleID {
			return convergence, true
		}
	}
	return runtimecontract.RuntimeOwnerFactConvergence{}, false
}

func prospectiveOwnerFactAcquisitionForProof(
	focus runtimecontract.CoverageFocus,
	identity runtimecontract.CoverageEvidenceProofIdentity,
) (runtimecontract.RuntimeOwnerFactConvergence, bool) {
	focus = focus.Normalized()
	identity = identity.Normalized()
	if identity.Validate() != nil || len(identity.AllowedOperationKinds) != 1 {
		return runtimecontract.RuntimeOwnerFactConvergence{}, false
	}
	for _, raw := range focus.OwnerFactConvergences {
		convergence := raw.Normalized()
		if convergence.Validate() != nil || convergence.RequirePreexisting ||
			convergence.AcquisitionTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
			convergence.AcquisitionOwnerModuleID != identity.OwnerModuleID ||
			convergence.AcquisitionOperationKind != identity.AllowedOperationKinds[0] ||
			convergence.AcquisitionTargetSelectorRef != identity.TargetSelectorRef ||
			convergence.ReobserveOwnerModuleID != convergence.Requirement.OwnerModuleID ||
			convergence.ReobserveFactKind != convergence.Requirement.FactKind ||
			!runtimeOwnerFactRequirementDeclared(focus.RequiredOwnerFacts, convergence.Requirement) ||
			!runtimeAcquisitionVariantMatchesProof(convergence, identity.AllowedOperationVariants) {
			continue
		}
		return convergence, true
	}
	return runtimecontract.RuntimeOwnerFactConvergence{}, false
}

func runtimeOwnerFactRequirementDeclared(values []runtimecontract.RuntimeOwnerFactRequirement, wanted runtimecontract.RuntimeOwnerFactRequirement) bool {
	wantedRef := runtimecontract.RuntimeOwnerFactRequirementRef(wanted)
	if wantedRef == "" {
		return false
	}
	for _, value := range values {
		if runtimecontract.RuntimeOwnerFactRequirementRef(value) == wantedRef {
			return true
		}
	}
	return false
}

func runtimeAcquisitionVariantMatchesProof(convergence runtimecontract.RuntimeOwnerFactConvergence, variants []runtimecontract.RuntimeOperationVariantRef) bool {
	variants = runtimecontract.NormalizeRuntimeOperationVariantRefs(variants)
	wanted := strings.TrimSpace(convergence.AcquisitionOperationVariantID)
	if wanted == "" {
		return len(variants) == 0
	}
	return len(variants) == 1 && variants[0].Kind == convergence.AcquisitionOperationKind && variants[0].VariantID == wanted
}

// retireConsumedProspectiveTargetSelection removes only the exact focus-scoped
// create selection which the accepted acquisition receipt consumed. Other
// invalidated owner facts remain pending and must be re-observed normally.
// Keeping the prerequisite in Retired preserves its lifecycle identity without
// allowing a later reducer pass or process restart to make the create-only
// query current again.
func retireConsumedProspectiveTargetSelection(
	facts runtimecontract.RuntimeFacts,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) (runtimecontract.RuntimeFacts, bool) {
	facts = facts.Normalized()
	convergence, committed := committedProspectiveOwnerFactAcquisition(facts, lineage)
	if !committed {
		return facts, false
	}
	focus := facts.Focus.Normalized()
	if len(focus.ProspectiveTargetRefs) != 1 {
		return facts, false
	}
	retired := false
	pending := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Pending))
	for _, prerequisite := range facts.Prerequisites.Pending {
		if !runtimeConsumedTargetSelectionMatchesFocus(prerequisite, focus, convergence.AcquisitionOwnerModuleID) {
			pending = append(pending, prerequisite)
			continue
		}
		facts.Prerequisites.Retired = append(facts.Prerequisites.Retired, prerequisite)
		retired = true
	}
	if !retired {
		return facts, false
	}
	facts.Prerequisites.Pending = pending
	if facts.Resources.TargetSelection != nil && runtimeConsumedTargetSelectionMatchesFocus(runtimecontract.RuntimePrerequisite{
		ID:                                   facts.Resources.TargetSelection.PrerequisiteID,
		Kind:                                 runtimecontract.PrerequisiteKindTargetSelection,
		Scope:                                runtimecontract.RuntimePrerequisiteScopeFocus,
		FocusID:                              focus.ID,
		OwnerModuleID:                        facts.Resources.TargetSelection.OwnerModuleID,
		SubjectRef:                           facts.Resources.TargetSelection.TargetRole,
		TargetSelectorRef:                    facts.Resources.TargetSelection.SelectorRef,
		TargetSelectorOwnerRef:               facts.Resources.TargetSelection.SelectorOwnerRef,
		TargetSelectorSubjectRef:             facts.Resources.TargetSelection.SelectorSubjectRef,
		TargetSelectorCandidateRefs:          facts.Resources.TargetSelection.SelectorCandidateRefs,
		ProspectiveTargetAllOfConstraintRefs: facts.Resources.TargetSelection.ProspectiveTargetAllOfConstraintRefs,
		ProspectiveTargetRef:                 facts.Resources.TargetSelection.ProspectiveTargetRef,
	}, focus, convergence.AcquisitionOwnerModuleID) {
		facts.Resources.TargetSelection = nil
	}
	return facts.Normalized(), true
}

func runtimeConsumedTargetSelectionMatchesFocus(prerequisite runtimecontract.RuntimePrerequisite, focus runtimecontract.CoverageFocus, ownerModuleID string) bool {
	focus = focus.Normalized()
	return prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection &&
		prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == focus.ID &&
		prerequisite.OwnerModuleID == strings.TrimSpace(ownerModuleID) &&
		prerequisite.SubjectRef == focus.TargetRole && prerequisite.TargetSelectorRef == focus.TargetSelectorRef &&
		prerequisite.TargetSelectorOwnerRef == focus.TargetSelectorOwnerRef &&
		prerequisite.TargetSelectorSubjectRef == focus.TargetSelectorSubjectRef &&
		runtimecontract.SameRuntimeTargetSelectorCandidateRefs(prerequisite.TargetSelectorCandidateRefs, focus.TargetSelectorCandidateRefs) &&
		runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(prerequisite.ProspectiveTargetAllOfConstraintRefs, focus.ProspectiveTargetAllOfConstraintRefs) &&
		len(focus.ProspectiveTargetRefs) == 1 && prerequisite.ProspectiveTargetRef == focus.ProspectiveTargetRefs[0]
}
