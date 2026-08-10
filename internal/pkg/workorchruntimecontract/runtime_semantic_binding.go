package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// CompileRuntimeSemanticBindings is a pure all-or-nothing conversion from one
// complete semantic document plus matching owner facts to immutable focus
// bindings and a reconciliation contract. It does not mutate an aggregate or
// choose a Runtime action; only the SM-2 reducer may persist its output.
func CompileRuntimeSemanticBindings(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, feasibilities []RuntimeSemanticProofFeasibility) ([]CoverageFocus, RuntimeCWIReconciliationContract, error) {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	if err := validateRuntimeSemanticFeasibilitySet(manifest, document, feasibilities, true); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	items := make(map[string]RuntimeCoverageItem, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[string(item.ID)] = item
	}
	facts := make(map[string]RuntimeSemanticProofFeasibility, len(feasibilities))
	for _, fact := range feasibilities {
		fact = fact.Clone()
		if fact.Status != RuntimeSemanticFeasibilityFeasible {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires a complete feasible owner batch")
		}
		facts[string(fact.CoverageWorkItemID)] = fact
	}
	proposals, err := OrderedRuntimeSemanticProofProposals(document)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	focuses := make([]CoverageFocus, 0, len(proposals))
	for _, proposal := range proposals {
		proposal = proposal.Normalized()
		// Effect/idempotency authority is compiled from the sealed owner catalog
		// at the SM-2 binding boundary. Any provider-shaped or stale intermediate
		// value is ignored here; the resulting binding is the canonical source
		// consumed by SM-3 dispatch and SM-4 evidence admission.
		proposal.RequiredEffectScopes = deriveRuntimeSemanticProposalEffects(manifest, runtimeSemanticProposalPayload{
			OwnerModuleID: proposal.OwnerModuleID, OperationKind: proposal.OperationKind,
			OperationVariantID: proposal.OperationVariantID, RequiredEvidenceKinds: proposal.RequiredEvidenceKinds,
			ValidationRecipeID: proposal.ValidationRecipeID,
		})
		item, found := items[string(proposal.CoverageWorkItemID)]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding references unknown coverage %q", proposal.CoverageWorkItemID)
		}
		fact, found := facts[string(proposal.CoverageWorkItemID)]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding lacks owner fact for coverage %q", proposal.CoverageWorkItemID)
		}
		focus, err := compileRuntimeSemanticFocus(manifest.DefinitionRevision, item, proposal, fact)
		if err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		focuses = append(focuses, focus)
	}
	// Do not normalize/sort this queue: batch membership is the persisted
	// provider order constrained by the active focus dependency closure.
	// Sorting by CWI ID here would silently replace that semantic order.
	contract, err := CompileRuntimeCWIReconciliationContract(manifest.DefinitionRevision, focuses)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	return focuses, contract, nil
}

// OrderedRuntimeSemanticProofProposals returns the proof proposals in the
// exact persisted batch order when present.  A full legacy document without
// batch membership remains deterministically ordered by its normalized
// proposal list.  This helper is pure and never treats provider order as
// execution authority; the Orchestrator alone seals the resulting focuses.
func OrderedRuntimeSemanticProofProposals(document RuntimeSemanticPlanDocument) ([]RuntimeSemanticProofProposal, error) {
	document = document.Normalized()
	if len(document.BatchCoverageWorkItemIDs) == 0 {
		return append([]RuntimeSemanticProofProposal(nil), document.Proposals...), nil
	}
	byCoverage := make(map[common.CoverageWorkItemID]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Proposals {
		if _, duplicate := byCoverage[proposal.CoverageWorkItemID]; duplicate {
			return nil, fmt.Errorf("runtime semantic proposal order duplicates coverage %q", proposal.CoverageWorkItemID)
		}
		byCoverage[proposal.CoverageWorkItemID] = proposal.Clone()
	}
	ordered := make([]RuntimeSemanticProofProposal, 0, len(document.BatchCoverageWorkItemIDs))
	for _, id := range document.BatchCoverageWorkItemIDs {
		proposal, found := byCoverage[id]
		if !found {
			return nil, fmt.Errorf("runtime semantic proposal order has no coverage %q", id)
		}
		ordered = append(ordered, proposal)
		delete(byCoverage, id)
	}
	if len(byCoverage) != 0 {
		return nil, fmt.Errorf("runtime semantic proposal order contains coverage outside batch")
	}
	return ordered, nil
}

func compileRuntimeSemanticFocus(definitionRevision string, item RuntimeCoverageItem, proposal RuntimeSemanticProofProposal, fact RuntimeSemanticProofFeasibility) (CoverageFocus, error) {
	proposalHash := StableRuntimeSemanticProofProposalHash(proposal)
	idSuffix := strings.TrimPrefix(proposalHash, "runtime-semantic-proposal-")
	focusID := "semantic-focus-" + idSuffix
	bindingID := "semantic-binding-" + idSuffix
	proofs := make([]CoverageEvidenceProof, 0, len(proposal.RequiredEvidenceKinds))
	for _, evidenceKind := range proposal.RequiredEvidenceKinds {
		effects := semanticProofEffectsForEvidenceKind(proposal.RequiredEffectScopes, evidenceKind)
		proof := CoverageEvidenceProof{
			EvidenceKind: evidenceKind, RequiredEffectScopes: effects, AllowedOperationKinds: []string{proposal.OperationKind},
			MetricConstraints: append([]string(nil), proposal.MetricConstraints...), ArtifactRefs: append([]string(nil), proposal.ArtifactRefs...),
		}
		if proposal.OperationVariantID != "" {
			proof.AllowedOperationVariants = []RuntimeOperationVariantRef{{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}}
		}
		if evidenceKind == EvidenceKindValidation {
			proof.ValidationRecipeID = fact.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), fact.RequiredCheckRefs...)
		}
		if err := proof.Validate(); err != nil {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proposal %q proof: %w", proposal.CoverageWorkItemID, err)
		}
		proofs = append(proofs, proof)
	}
	targetRefs, selectorRef, prospectiveTargetRef, err := semanticBindingTarget(proposal, fact)
	if err != nil {
		return CoverageFocus{}, fmt.Errorf("runtime semantic proposal %q target: %w", proposal.CoverageWorkItemID, err)
	}
	binding := CoverageEvidenceBinding{
		ID: bindingID, CoverageWorkItemID: proposal.CoverageWorkItemID, FocusID: focusID, DefinitionRevision: definitionRevision,
		Proofs: proofs, TargetRefs: targetRefs, TargetSelectorRef: selectorRef, DeclarationRef: proposal.DeclarationRef,
		ProspectiveTargetRef: prospectiveTargetRef,
		PrerequisiteRefs:     []string{fact.CatalogSnapshotID, "semantic-feasibility/" + strings.TrimPrefix(StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}), "runtime-semantic-feasibility-set-")},
	}
	if selectorRef != "" {
		binding.TargetSelectorOwnerRef = proposal.OwnerModuleID
		binding.TargetSelectorSubjectRef = "workspace_deliverable"
	}
	binding = binding.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		return CoverageFocus{}, err
	}
	focus := CoverageFocus{
		ID: focusID, SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{proposal.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
		EvidenceDeclarationRef: proposal.DeclarationRef, RequiredEvidenceKinds: append([]string(nil), proposal.RequiredEvidenceKinds...),
		TargetRefs: targetRefs, TargetRole: "semantic_proof_target", TargetSelectorRef: selectorRef,
		TargetSelectorOwnerRef: binding.TargetSelectorOwnerRef, TargetSelectorSubjectRef: binding.TargetSelectorSubjectRef,
		ProspectiveTargetRef: prospectiveTargetRef, RequireTargetGrounding: true,
		AcceptedStepKinds: []string{proposal.OperationKind}, RequireDirectEvidence: true, RequireAssessment: true,
		ValidationRecipeID: fact.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), fact.RequiredCheckRefs...),
		MetricConstraints: append([]string(nil), proposal.MetricConstraints...), ArtifactRefs: append([]string(nil), proposal.ArtifactRefs...),
	}
	if proposal.OperationVariantID != "" {
		focus.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}}
	}
	focus = focus.Normalized()
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, definitionRevision); err != nil {
		return CoverageFocus{}, err
	}
	return focus, nil
}

func semanticProofEffectsForEvidenceKind(values []RuntimeEffectScope, evidenceKind string) []RuntimeEffectScope {
	ret := make([]RuntimeEffectScope, 0, len(values))
	for _, value := range NormalizeRuntimeEffectScopes(values) {
		if evidenceKind == EvidenceKindStateChange && RuntimeEffectIsDurable(value) {
			ret = append(ret, value)
		}
		if evidenceKind == EvidenceKindValidation && RuntimeEffectIsNonDurable(value) {
			ret = append(ret, value)
		}
	}
	return NormalizeRuntimeEffectScopes(ret)
}

func semanticBindingTarget(proposal RuntimeSemanticProofProposal, fact RuntimeSemanticProofFeasibility) ([]string, string, string, error) {
	target := proposal.Target.Normalized()
	factTargets := normalizeStrings(fact.TargetRefs)
	switch target.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		if !sameStrings(factTargets, target.ExactTargetRefs) {
			return nil, "", "", fmt.Errorf("runtime semantic exact target owner fact does not match proposal")
		}
		return factTargets, "", "", nil
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) != 0 && !sameStrings(factTargets, target.OwnerScopeTargetRefs) {
			return nil, "", "", fmt.Errorf("runtime semantic owner scope fact does not match proposal")
		}
		selector := ""
		if len(target.OwnerSelectorRefs) == 1 {
			selector = target.OwnerSelectorRefs[0]
		}
		return factTargets, selector, "", nil
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		// A prospective target is not required to exist in the owner inventory.
		// Feasibility may therefore legitimately return no TargetRefs; when it
		// does return one, it must agree with the sealed prospective ref.
		if len(factTargets) != 0 && !sameStrings(factTargets, []string{target.ProspectiveTargetRef}) {
			return nil, "", "", fmt.Errorf("runtime semantic prospective target owner fact does not match proposal")
		}
		return nil, target.OwnerSelectorRefs[0], target.ProspectiveTargetRef, nil
	default:
		return nil, "", "", fmt.Errorf("runtime semantic binding has unsupported target policy %q", target.Policy)
	}
}
