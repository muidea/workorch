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
	return CompileRuntimeSemanticBindingsWithSourceFocuses(manifest, document, feasibilities, nil)
}

// CompileRuntimeSemanticBindingsWithSourceFocuses compiles one complete
// semantic document with the persisted source focus declarations that remain
// relevant to its eligible CWI rows. Semantic proposals choose only within the
// sealed manifest; source declarations supply bounded selector replay data
// that capability owners require after the source focus is replaced by the
// newly sealed semantic focus.
func CompileRuntimeSemanticBindingsWithSourceFocuses(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, feasibilities []RuntimeSemanticProofFeasibility, sourceFocuses []CoverageFocus) ([]CoverageFocus, RuntimeCWIReconciliationContract, error) {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	if err := validateRuntimeSemanticFeasibilitySet(manifest, document, feasibilities, true); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	sourceByCoverage, err := runtimeSemanticBindingSourceFocuses(sourceFocuses)
	if err != nil {
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
		facts[fact.ProposalHash] = fact
	}
	proofSets, err := OrderedRuntimeSemanticCWIProofSets(document)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	focuses := make([]CoverageFocus, 0, len(proofSets))
	for _, proofSet := range proofSets {
		item, found := items[string(proofSet.CoverageWorkItemID)]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding references unknown coverage %q", proofSet.CoverageWorkItemID)
		}
		proofFacts := make(map[string]RuntimeSemanticProofFeasibility, len(proofSet.Proposals))
		for _, proposal := range proofSet.Proposals {
			proposalHash := StableRuntimeSemanticProofProposalHash(proposal)
			fact, found := facts[proposalHash]
			if !found {
				return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding lacks owner fact for proof %q", proposalHash)
			}
			proofFacts[proposalHash] = fact
		}
		var source *CoverageFocus
		if value, found := sourceByCoverage[proofSet.CoverageWorkItemID]; found {
			value := value
			source = &value
		}
		if err := validateRuntimeSemanticProofSetSource(proofSet, source); err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		focus, err := compileRuntimeSemanticFocus(manifest, item, proofSet, proofFacts, source)
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

func runtimeSemanticBindingSourceFocuses(values []CoverageFocus) (map[common.CoverageWorkItemID]CoverageFocus, error) {
	ret := make(map[common.CoverageWorkItemID]CoverageFocus, len(values))
	for _, value := range values {
		value = value.Normalized()
		for _, coverageID := range value.CoverageWorkItemIDs {
			if coverageID == "" {
				return nil, fmt.Errorf("runtime semantic source focus %q has an empty coverage item", value.ID)
			}
			if prior, duplicate := ret[coverageID]; duplicate && prior.ID != value.ID {
				return nil, fmt.Errorf("runtime semantic source focuses duplicate coverage %q", coverageID)
			}
			ret[coverageID] = value
		}
	}
	return ret, nil
}

// validateRuntimeSemanticProofSetSource ensures semantic planning cannot
// weaken the exact evidence contract that activation/profile already declared
// for a CWI. It is a pure SM-2 pre-seal check; it does not choose a proof.
func validateRuntimeSemanticProofSetSource(proofSet RuntimeSemanticCWIProofSet, source *CoverageFocus) error {
	if source == nil {
		return nil
	}
	declaration := source.Normalized()
	if len(declaration.RequiredEvidenceKinds) != 0 && !sameStrings(proofSet.EvidenceKinds(), declaration.RequiredEvidenceKinds) {
		return fmt.Errorf("runtime semantic proof set for coverage %q does not cover source required evidence kinds", proofSet.CoverageWorkItemID)
	}
	for _, proposal := range proofSet.Proposals {
		proposal = proposal.Normalized()
		if len(proposal.RequiredEvidenceKinds) != 1 || proposal.RequiredEvidenceKinds[0] != EvidenceKindValidation {
			continue
		}
		if declaration.ValidationRecipeID != "" && proposal.ValidationRecipeID != declaration.ValidationRecipeID {
			return fmt.Errorf("runtime semantic validation proof for coverage %q does not match source recipe", proofSet.CoverageWorkItemID)
		}
	}
	return nil
}

func compileRuntimeSemanticFocus(manifest RuntimeSemanticPlanManifest, item RuntimeCoverageItem, proofSet RuntimeSemanticCWIProofSet, facts map[string]RuntimeSemanticProofFeasibility, source *CoverageFocus) (CoverageFocus, error) {
	manifest = manifest.Normalized()
	proofSetHash := StableRuntimeSemanticCWIProofSetHash(proofSet.CoverageWorkItemID, proofSet.Proposals)
	idSuffix := strings.TrimPrefix(proofSetHash, "runtime-semantic-cwi-proof-set-")
	manifestSuffix := strings.TrimPrefix(strings.TrimSpace(manifest.Hash), "runtime-semantic-manifest-")
	if manifestSuffix == "" {
		return CoverageFocus{}, fmt.Errorf("runtime semantic focus requires a manifest generation identity")
	}
	// The proposal set is CWI-scoped. A generation-specific focus/binding ID
	// therefore cannot be reused after catalog re-acquisition, even when every
	// provider proof text happens to be identical.
	focusID := "semantic-focus-" + idSuffix + "-" + manifestSuffix
	bindingID := "semantic-binding-" + idSuffix + "-" + manifestSuffix

	proofs := make([]CoverageEvidenceProof, 0, len(proofSet.Proposals))
	acceptedKinds := make([]string, 0, len(proofSet.Proposals))
	acceptedVariants := make([]RuntimeOperationVariantRef, 0, len(proofSet.Proposals))
	metricConstraints := make([]string, 0)
	artifactRefs := make([]string, 0)
	prerequisiteRefs := make([]string, 0, len(proofSet.Proposals)*2)
	var targetRefs []string
	var selectorRef, prospectiveTargetRef, declarationRef, selectorOwner string
	var validationRecipeID string
	validationChecks := make([]string, 0)
	for proofIndex, rawProposal := range proofSet.Proposals {
		proposal := rawProposal.Normalized()
		proposal.RequiredEffectScopes = deriveRuntimeSemanticProposalEffects(manifest, runtimeSemanticProposalPayload{
			OwnerModuleID: proposal.OwnerModuleID, OperationKind: proposal.OperationKind,
			OperationVariantID: proposal.OperationVariantID, RequiredEvidenceKinds: proposal.RequiredEvidenceKinds,
			ValidationRecipeID: proposal.ValidationRecipeID,
		})
		proposalHash := StableRuntimeSemanticProofProposalHash(rawProposal)
		fact, found := facts[proposalHash]
		if !found {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof set for coverage %q lacks fact %q", proofSet.CoverageWorkItemID, proposalHash)
		}
		if err := fact.ValidateForProposal(manifest, rawProposal); err != nil {
			return CoverageFocus{}, err
		}
		evidenceKind := proposal.RequiredEvidenceKinds[0]
		proof := CoverageEvidenceProof{
			EvidenceKind: evidenceKind, RequiredEffectScopes: semanticProofEffectsForEvidenceKind(proposal.RequiredEffectScopes, evidenceKind),
			AllowedOperationKinds: []string{proposal.OperationKind}, MetricConstraints: append([]string(nil), proposal.MetricConstraints...),
			ArtifactRefs: append([]string(nil), proposal.ArtifactRefs...),
		}
		if proposal.OperationVariantID != "" {
			proof.AllowedOperationVariants = []RuntimeOperationVariantRef{{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}}
			acceptedVariants = append(acceptedVariants, proof.AllowedOperationVariants[0])
		}
		if evidenceKind == EvidenceKindValidation {
			proof.ValidationRecipeID = fact.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), fact.RequiredCheckRefs...)
			validationRecipeID = fact.ValidationRecipeID
			validationChecks = append(validationChecks, fact.RequiredCheckRefs...)
		}
		if err := proof.Validate(); err != nil {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof %q for coverage %q: %w", proposalHash, proofSet.CoverageWorkItemID, err)
		}
		currentTargets, currentSelector, currentProspective, err := semanticBindingTarget(proposal, fact)
		if err != nil {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof %q target: %w", proposalHash, err)
		}
		currentSelectorOwner := proposal.OwnerModuleID
		if source != nil && currentSelector != "" {
			declaration := source.Normalized()
			if declaration.TargetSelectorRef == currentSelector && declaration.TargetSelectorOwnerRef != "" {
				currentSelectorOwner = declaration.TargetSelectorOwnerRef
			}
		}
		if proofIndex == 0 {
			targetRefs, selectorRef, prospectiveTargetRef = currentTargets, currentSelector, currentProspective
			declarationRef, selectorOwner = proposal.DeclarationRef, currentSelectorOwner
		} else {
			if declarationRef != proposal.DeclarationRef {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
			targetRefs, selectorRef, prospectiveTargetRef, selectorOwner, err = mergeRuntimeSemanticBindingTarget(
				targetRefs, selectorRef, prospectiveTargetRef, selectorOwner,
				currentTargets, currentSelector, currentProspective, currentSelectorOwner,
			)
			if err != nil {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
		}
		acceptedKinds = append(acceptedKinds, proposal.OperationKind)
		metricConstraints = append(metricConstraints, proposal.MetricConstraints...)
		artifactRefs = append(artifactRefs, proposal.ArtifactRefs...)
		prerequisiteRefs = append(prerequisiteRefs, fact.CatalogSnapshotID)
		prerequisiteRefs = append(prerequisiteRefs, "semantic-feasibility/"+strings.TrimPrefix(StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}), "runtime-semantic-feasibility-set-"))
		proofs = append(proofs, proof)
	}
	binding := CoverageEvidenceBinding{
		ID: bindingID, CoverageWorkItemID: proofSet.CoverageWorkItemID, FocusID: focusID, DefinitionRevision: manifest.DefinitionRevision,
		Proofs: proofs, TargetRefs: targetRefs, TargetSelectorRef: selectorRef, DeclarationRef: declarationRef,
		ProspectiveTargetRef: prospectiveTargetRef, PrerequisiteRefs: prerequisiteRefs,
	}
	if selectorRef != "" {
		binding.TargetSelectorOwnerRef = selectorOwner
		binding.TargetSelectorSubjectRef = "workspace_deliverable"
		if source != nil {
			declaration := source.Normalized()
			if declaration.TargetSelectorRef == selectorRef {
				binding.TargetSelectorCandidateRefs = append([]string(nil), declaration.TargetSelectorCandidateRefs...)
			}
		}
	}
	binding = binding.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		return CoverageFocus{}, err
	}
	focus := CoverageFocus{
		ID: focusID, SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{proofSet.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
		EvidenceDeclarationRef: declarationRef, RequiredEvidenceKinds: binding.EvidenceKinds(),
		TargetRefs: targetRefs, TargetRole: "semantic_proof_target", TargetSelectorRef: selectorRef,
		TargetSelectorOwnerRef: binding.TargetSelectorOwnerRef, TargetSelectorSubjectRef: binding.TargetSelectorSubjectRef,
		ProspectiveTargetRef: prospectiveTargetRef, RequireTargetGrounding: true,
		AcceptedStepKinds: acceptedKinds, AcceptedOperationVariants: acceptedVariants, RequireDirectEvidence: true, RequireAssessment: true,
		ValidationRecipeID: validationRecipeID, RequiredCheckRefs: validationChecks,
		MetricConstraints: metricConstraints, ArtifactRefs: artifactRefs,
	}
	if source != nil {
		declaration := source.Normalized()
		if selectorRef != "" && declaration.TargetSelectorRef == selectorRef {
			focus.TargetRole = declaration.TargetRole
			focus.TargetDiscoverySources = append([]string(nil), declaration.TargetDiscoverySources...)
			focus.TargetSelectorOwnerRef = declaration.TargetSelectorOwnerRef
			focus.TargetSelectorSubjectRef = declaration.TargetSelectorSubjectRef
			focus.TargetSelectorCandidateRefs = append([]string(nil), declaration.TargetSelectorCandidateRefs...)
			focus.RequireTargetGrounding = declaration.RequireTargetGrounding
		}
	}
	focus = focus.Normalized()
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, manifest.DefinitionRevision); err != nil {
		return CoverageFocus{}, err
	}
	return focus, nil
}

// mergeRuntimeSemanticBindingTarget preserves one logical target surface for
// an independent proof set. A prospective state change and a later validation
// necessarily use different owner policies: File authorizes creation while a
// validation owner observes the resulting exact path. They may share one CWI
// binding only when both facts name that exact same logical target.
func mergeRuntimeSemanticBindingTarget(
	targets []string, selector, prospective, selectorOwner string,
	currentTargets []string, currentSelector, currentProspective, currentOwner string,
) ([]string, string, string, string, error) {
	if prospective != "" {
		if currentProspective == "" && currentSelector == "" && sameStrings(currentTargets, []string{prospective}) {
			return targets, selector, prospective, selectorOwner, nil
		}
		if currentProspective == prospective && currentSelector == selector && sameStrings(currentTargets, targets) && currentOwner == selectorOwner {
			return targets, selector, prospective, selectorOwner, nil
		}
		return nil, "", "", "", fmt.Errorf("semantic prospective target surface mismatch")
	}
	if currentProspective != "" {
		if selector == "" && sameStrings(targets, []string{currentProspective}) {
			return currentTargets, currentSelector, currentProspective, currentOwner, nil
		}
		return nil, "", "", "", fmt.Errorf("semantic prospective target surface mismatch")
	}
	if !sameStrings(targets, currentTargets) || selector != currentSelector {
		return nil, "", "", "", fmt.Errorf("semantic target surface mismatch")
	}
	if selector != "" && selectorOwner != currentOwner {
		return nil, "", "", "", fmt.Errorf("semantic selector owner mismatch")
	}
	return targets, selector, prospective, selectorOwner, nil
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
