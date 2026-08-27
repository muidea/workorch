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
func CompileRuntimeSemanticBindings(manifest RuntimeSemanticPlanManifest, logicalRequestID string, document RuntimeSemanticPlanDocument, feasibilities []RuntimeSemanticProofFeasibility) ([]CoverageFocus, RuntimeCWIReconciliationContract, error) {
	manifest = manifest.Normalized()
	document = document.Normalized()
	generationID := StableRuntimeSemanticGenerationID(manifest.Hash, logicalRequestID)
	if generationID == "" {
		return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires manifest and logical-request generation identity")
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	if err := validateRuntimeSemanticFeasibilitySet(manifest, document, feasibilities, true); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	sourceByCoverage := make(map[common.CoverageWorkItemID]CoverageFocus, len(manifest.ProofObligations))
	obligationByCoverage := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		sourceByCoverage[obligation.CoverageWorkItemID] = runtimeSemanticProofObligationSourceFocus(obligation)
		obligationByCoverage[obligation.CoverageWorkItemID] = obligation.Clone()
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
	if err := validateRuntimeSemanticPlanProofObligations(document, manifest); err != nil {
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
		obligation, found := obligationByCoverage[proofSet.CoverageWorkItemID]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding has no proof obligation for coverage %q", proofSet.CoverageWorkItemID)
		}
		evidenceObligation, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
		if err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		if err := validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet, source, evidenceObligation.CurrentRequiredEvidenceKinds); err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		focus, err := compileRuntimeSemanticFocus(manifest, generationID, item, proofSet, proofFacts, source, evidenceObligation)
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

// validateRuntimeSemanticPlanProofObligations constrains each provider proof
// to the exact per-CWI obligation already sealed in the manifest. It performs
// no aggregate read and therefore cannot create a second Definition authority.
func validateRuntimeSemanticPlanProofObligations(document RuntimeSemanticPlanDocument, manifest RuntimeSemanticPlanManifest) error {
	document = document.Normalized()
	manifest = manifest.Normalized()
	obligations := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		obligations[obligation.CoverageWorkItemID] = obligation
	}
	proofSets, err := OrderedRuntimeSemanticCWIProofSets(document)
	if err != nil {
		return err
	}
	for _, proofSet := range proofSets {
		obligation, found := obligations[proofSet.CoverageWorkItemID]
		if !found {
			return fmt.Errorf("runtime semantic proof for coverage %q has no sealed obligation", proofSet.CoverageWorkItemID)
		}
		evidenceObligation, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
		if err != nil {
			return err
		}
		sourceValue := runtimeSemanticProofObligationSourceFocus(obligation)
		source := &sourceValue
		if err := validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet, source, evidenceObligation.CurrentRequiredEvidenceKinds); err != nil {
			return err
		}
		proposalIndex := -1
		for index, proposal := range document.Proposals {
			if proposal.CoverageWorkItemID == proofSet.CoverageWorkItemID {
				proposalIndex = index
				break
			}
		}
		if err := validateRuntimeSemanticOwnerFactAcquisition(proofSet, obligation.OwnerFactAcquisition, proposalIndex); err != nil {
			return err
		}
		if err := validateRuntimeSemanticValidationFailureRepair(proofSet, obligation.ValidationFailureRepair, proposalIndex); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeSemanticOwnerFactAcquisition(proofSet RuntimeSemanticCWIProofSet, acquisition *RuntimeSemanticOwnerFactAcquisition, proposalIndex int) error {
	if acquisition == nil {
		return nil
	}
	sealed := acquisition.Normalized()
	if len(proofSet.Proposals) != 1 {
		return &RuntimeSemanticProposalViolationError{ViolationFact: RuntimeSemanticContractViolation{
			Kind: RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals",
			CoverageWorkItemID: proofSet.CoverageWorkItemID, ProposalIndex: proposalIndex,
			Rule: RuntimeSemanticConstraintRelational, Actual: fmt.Sprintf("%d proposals", len(proofSet.Proposals)),
			Required: []string{"exactly one sealed owner-fact acquisition"},
		}, Cause: fmt.Errorf("runtime semantic proof for coverage %q must contain exactly one sealed owner-fact acquisition", proofSet.CoverageWorkItemID)}
	}
	proposal := proofSet.Proposals[0].Normalized()
	if proposal.OwnerModuleID != sealed.AcquisitionOwnerModuleID {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "ownerModuleId", RuntimeSemanticConstraintAllowedValue, proposal.OwnerModuleID, nil, []string{sealed.AcquisitionOwnerModuleID}, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition owner", proofSet.CoverageWorkItemID))
	}
	if proposal.OperationKind != sealed.AcquisitionOperationKind {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "operationKind", RuntimeSemanticConstraintAllowedValue, proposal.OperationKind, nil, []string{sealed.AcquisitionOperationKind}, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition operation", proofSet.CoverageWorkItemID))
	}
	if proposal.OperationVariantID != sealed.AcquisitionOperationVariantID {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "operationVariantId", RuntimeSemanticConstraintAllowedValue, proposal.OperationVariantID, nil, []string{sealed.AcquisitionOperationVariantID}, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition variant", proofSet.CoverageWorkItemID))
	}
	if len(proposal.RequiredEvidenceKinds) != 1 || proposal.RequiredEvidenceKinds[0] != EvidenceKindStateChange {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "requiredEvidenceKinds", RuntimeSemanticConstraintAllowedValue, strings.Join(proposal.RequiredEvidenceKinds, ","), nil, []string{EvidenceKindStateChange}, fmt.Errorf("runtime semantic proof for coverage %q owner-fact acquisition must produce state-change evidence", proofSet.CoverageWorkItemID))
	}
	if proposal.Target.Policy != sealed.AcquisitionTargetPolicy {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "target.policy", RuntimeSemanticConstraintAllowedValue, string(proposal.Target.Policy), nil, []string{string(sealed.AcquisitionTargetPolicy)}, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition policy", proofSet.CoverageWorkItemID))
	}
	if len(proposal.Target.OwnerSelectorRefs) != 1 || proposal.Target.OwnerSelectorRefs[0] != sealed.AcquisitionTargetSelectorRef {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "target.ownerSelectorRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(proposal.Target.OwnerSelectorRefs, ","), nil, []string{sealed.AcquisitionTargetSelectorRef}, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition selector", proofSet.CoverageWorkItemID))
	}
	if !sameStrings(proposal.Target.ProspectiveConstraintRefs, sealed.AcquisitionConstraintRefs) {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "target.prospectiveConstraintRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(proposal.Target.ProspectiveConstraintRefs, ","), nil, sealed.AcquisitionConstraintRefs, fmt.Errorf("runtime semantic proof for coverage %q does not match sealed owner-fact acquisition constraints", proofSet.CoverageWorkItemID))
	}
	if sealed.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
		(sealed.ProspectiveTargetSatisfactionConstraint == nil ||
			!RuntimeProspectiveTargetsSatisfyConstraint(proposal.Target.ProspectiveTargetRefs, *sealed.ProspectiveTargetSatisfactionConstraint)) {
		required := []string(nil)
		if sealed.ProspectiveTargetSatisfactionConstraint != nil {
			required = []string{StableRuntimeProspectiveTargetSatisfactionConstraintHash(*sealed.ProspectiveTargetSatisfactionConstraint)}
		}
		return runtimeSemanticProposalViolation(
			proposal,
			proposalIndex,
			"target.prospectiveTargetRefs",
			RuntimeSemanticConstraintRelational,
			strings.Join(proposal.Target.ProspectiveTargetRefs, ","),
			required,
			nil,
			fmt.Errorf("runtime semantic proof for coverage %q prospective target does not satisfy the sealed owner all-of constraint", proofSet.CoverageWorkItemID),
		)
	}
	return nil
}

func runtimeSemanticBindingSourceFocuses(values []RuntimeDefinitionFocus) (map[common.CoverageWorkItemID]RuntimeDefinitionFocus, error) {
	ret := make(map[common.CoverageWorkItemID]RuntimeDefinitionFocus, len(values))
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
	var requiredEvidenceKinds []string
	if source != nil {
		requiredEvidenceKinds = source.Normalized().RequiredEvidenceKinds
	}
	return validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet, source, requiredEvidenceKinds)
}

func validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet RuntimeSemanticCWIProofSet, source *CoverageFocus, requiredEvidenceKinds []string) error {
	if source == nil {
		return nil
	}
	declaration := source.Normalized()
	requiredEvidenceKinds = normalizeStrings(requiredEvidenceKinds)
	if len(requiredEvidenceKinds) != 0 && !sameStrings(proofSet.EvidenceKinds(), requiredEvidenceKinds) {
		return fmt.Errorf("runtime semantic proof set for coverage %q does not cover source required evidence kinds", proofSet.CoverageWorkItemID)
	}
	for proposalIndex, proposal := range proofSet.Proposals {
		proposal = proposal.Normalized()
		if proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(declaration.TargetSelectorCandidateRefs) != 0 &&
			!runtimeSemanticStringsWithin(proposal.Target.ProspectiveTargetRefs, declaration.TargetSelectorCandidateRefs) {
			return runtimeSemanticProposalViolation(
				proposal,
				proposalIndex,
				"target.prospectiveTargetRefs",
				RuntimeSemanticConstraintAllowedValue,
				strings.Join(proposal.Target.ProspectiveTargetRefs, ","),
				nil,
				declaration.TargetSelectorCandidateRefs,
				fmt.Errorf("runtime semantic proof for coverage %q prospective targets are outside source selector candidates", proofSet.CoverageWorkItemID),
			)
		}
		if len(declaration.AcceptedStepKinds) != 0 && !runtimeOperationKindsContain(declaration.AcceptedStepKinds, proposal.OperationKind) {
			return fmt.Errorf("runtime semantic proof for coverage %q operation kind %q is outside source declaration", proofSet.CoverageWorkItemID, proposal.OperationKind)
		}
		declaredVariants := make([]RuntimeOperationVariantRef, 0)
		for _, variant := range declaration.AcceptedOperationVariants {
			if CanonicalRuntimeBlockKind(variant.Kind) == CanonicalRuntimeBlockKind(proposal.OperationKind) {
				declaredVariants = append(declaredVariants, variant)
			}
		}
		if len(declaredVariants) != 0 {
			matched := false
			for _, variant := range declaredVariants {
				if variant.VariantID == proposal.OperationVariantID {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("runtime semantic proof for coverage %q operation variant %q is outside source declaration", proofSet.CoverageWorkItemID, proposal.OperationVariantID)
			}
		}
		if len(proposal.RequiredEvidenceKinds) != 1 || proposal.RequiredEvidenceKinds[0] != EvidenceKindValidation {
			continue
		}
		if declaration.ValidationRecipeID != "" && proposal.ValidationRecipeID != declaration.ValidationRecipeID {
			return fmt.Errorf("runtime semantic validation proof for coverage %q does not match source recipe", proofSet.CoverageWorkItemID)
		}
	}
	return nil
}

func compileRuntimeSemanticFocus(manifest RuntimeSemanticPlanManifest, generationID string, item RuntimeCoverageItem, proofSet RuntimeSemanticCWIProofSet, facts map[string]RuntimeSemanticProofFeasibility, source *CoverageFocus, evidenceObligation RuntimeSemanticEvidenceObligation) (CoverageFocus, error) {
	manifest = manifest.Normalized()
	proofSetHash := StableRuntimeSemanticCWIProofSetHash(proofSet.CoverageWorkItemID, proofSet.Proposals)
	idSuffix := strings.TrimPrefix(proofSetHash, "runtime-semantic-cwi-proof-set-")
	generationSuffix := strings.TrimPrefix(strings.TrimSpace(generationID), "runtime-semantic-generation-")
	if generationSuffix == "" {
		return CoverageFocus{}, fmt.Errorf("runtime semantic focus requires an exact semantic generation identity")
	}
	// The proposal set is CWI-scoped. A generation-specific focus/binding ID
	// therefore cannot be reused after catalog re-acquisition, even when every
	// provider proof text happens to be identical.
	focusID := "semantic-focus-" + idSuffix + "-" + generationSuffix
	bindingID := "semantic-binding-" + idSuffix + "-" + generationSuffix

	proofs := make([]CoverageEvidenceProof, 0, len(proofSet.Proposals))
	acceptedKinds := make([]string, 0, len(proofSet.Proposals))
	acceptedVariants := make([]RuntimeOperationVariantRef, 0, len(proofSet.Proposals))
	prerequisiteRefs := make([]string, 0, len(proofSet.Proposals)*2)
	var targetRefs []string
	var prospectiveTargetRefs []string
	var selectorRef, declarationRef, selectorOwner string
	var validationRecipeID string
	validationChecks := make([]string, 0)
	var validationRepair *RuntimeSemanticValidationFailureRepair
	for _, obligation := range manifest.ProofObligations {
		if obligation.CoverageWorkItemID == proofSet.CoverageWorkItemID && obligation.ValidationFailureRepair != nil {
			value := obligation.ValidationFailureRepair.Clone()
			validationRepair = &value
			break
		}
	}
	proofRequirementIndex := -1
	if evidenceObligation.Purpose == RuntimeSemanticEvidenceObligationDefinitionProof {
		var err error
		proofRequirementIndex, err = runtimeSemanticSourceProofRequirementIndex(proofSet, source)
		if err != nil {
			return CoverageFocus{}, err
		}
	}
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
		if validationRepair != nil && proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
			!sameStrings(fact.TargetRefs, proposal.Target.ProspectiveTargetRefs) {
			return CoverageFocus{}, fmt.Errorf("runtime semantic validation repair prospective target lacks exact capability-owner confirmation")
		}
		evidenceKind := proposal.RequiredEvidenceKinds[0]
		proof := CoverageEvidenceProof{
			EvidenceKind: evidenceKind, RequiredEffectScopes: semanticProofEffectsForEvidenceKind(proposal.RequiredEffectScopes, evidenceKind),
			AllowedOperationKinds: []string{proposal.OperationKind},
		}
		if source != nil && proofIndex == proofRequirementIndex {
			declaration := source.Normalized()
			proof.MetricConstraints = append([]string(nil), declaration.MetricConstraints...)
			proof.ArtifactRefs = append([]string(nil), declaration.ArtifactRefs...)
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
			targetRefs, selectorRef, prospectiveTargetRefs = currentTargets, currentSelector, currentProspective
			declarationRef, selectorOwner = proposal.DeclarationRef, currentSelectorOwner
		} else {
			if declarationRef != proposal.DeclarationRef {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
			targetRefs, selectorRef, prospectiveTargetRefs, selectorOwner, err = mergeRuntimeSemanticBindingTarget(
				targetRefs, selectorRef, prospectiveTargetRefs, selectorOwner,
				currentTargets, currentSelector, currentProspective, currentSelectorOwner,
			)
			if err != nil {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
		}
		acceptedKinds = append(acceptedKinds, proposal.OperationKind)
		prerequisiteRefs = append(prerequisiteRefs, fact.CatalogSnapshotID)
		prerequisiteRefs = append(prerequisiteRefs, "semantic-feasibility/"+strings.TrimPrefix(StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}), "runtime-semantic-feasibility-set-"))
		proofs = append(proofs, proof)
	}
	binding := CoverageEvidenceBinding{
		ID: bindingID, CoverageWorkItemID: proofSet.CoverageWorkItemID, FocusID: focusID, DefinitionRevision: manifest.DefinitionRevision,
		Proofs: proofs, TargetRefs: targetRefs, TargetSelectorRef: selectorRef, DeclarationRef: declarationRef,
		ProspectiveTargetRefs: prospectiveTargetRefs, PrerequisiteRefs: prerequisiteRefs,
	}
	if validationRepair != nil {
		repair := validationRepair.Normalized()
		binding.PrerequisiteRefs = append(binding.PrerequisiteRefs,
			"semantic-purpose/"+string(RuntimeSemanticEvidenceObligationValidationFailureRepair),
			"validation-repair-convergence/"+repair.ConvergenceHash,
			"validation-repair-failure/"+repair.FailureSignature,
			"validation-repair-failed-focus/"+repair.FailedFocusID,
			"validation-repair-owner-catalog/"+repair.OwnerCatalogSnapshotID,
		)
	}
	if selectorRef != "" {
		binding.TargetSelectorOwnerRef = selectorOwner
		binding.TargetSelectorSubjectRef = "workspace_deliverable"
		if source != nil {
			declaration := source.Normalized()
			if declaration.TargetSelectorRef == selectorRef {
				binding.TargetSelectorOwnerRef = declaration.TargetSelectorOwnerRef
				binding.TargetSelectorSubjectRef = declaration.TargetSelectorSubjectRef
				binding.TargetSelectorCandidateRefs = append([]string(nil), declaration.TargetSelectorCandidateRefs...)
			}
		}
		if validationRepair != nil {
			repair := validationRepair.Normalized()
			binding.TargetSelectorOwnerRef = repair.RepairDeclarationTargetSelectorOwnerRef
			binding.TargetSelectorSubjectRef = repair.RepairDeclarationTargetSelectorSubjectRef
			binding.TargetSelectorCandidateRefs = append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...)
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
		ProspectiveTargetRefs: prospectiveTargetRefs, RequireTargetGrounding: true,
		AcceptedStepKinds: acceptedKinds, AcceptedOperationVariants: acceptedVariants, RequireDirectEvidence: true, RequireAssessment: true,
		ValidationRecipeID: validationRecipeID, RequiredCheckRefs: validationChecks,
	}
	if source != nil {
		declaration := source.Normalized()
		focus.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), declaration.RequiredOwnerFacts...)
		focus.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(declaration.OwnerFactConvergences)
		focus.MetricConstraints = append([]string(nil), declaration.MetricConstraints...)
		focus.ArtifactRefs = append([]string(nil), declaration.ArtifactRefs...)
	}
	if validationRepair != nil {
		repair := validationRepair.Normalized()
		focus.TargetRole = repair.RepairTargetRole
		focus.TargetSelectorRef = repair.RepairOperationTargetSelectorRef
		focus.TargetSelectorOwnerRef = repair.RepairDeclarationTargetSelectorOwnerRef
		focus.TargetSelectorSubjectRef = repair.RepairDeclarationTargetSelectorSubjectRef
		focus.TargetSelectorCandidateRefs = append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...)
		focus.RequireTargetGrounding = true
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

// runtimeSemanticSourceProofRequirementIndex assigns profile-sealed metric and
// artifact requirements to one proof owner. Provider output cannot create or
// route these requirements. A validation proof is preferred because metrics
// and formal artifacts are observations; otherwise the proof set must contain
// exactly one proof so ownership remains unambiguous.
func runtimeSemanticSourceProofRequirementIndex(proofSet RuntimeSemanticCWIProofSet, source *CoverageFocus) (int, error) {
	if source == nil {
		return -1, nil
	}
	declaration := source.Normalized()
	if len(declaration.MetricConstraints) == 0 && len(declaration.ArtifactRefs) == 0 {
		return -1, nil
	}
	validationIndex := -1
	for index, proposal := range proofSet.Proposals {
		proposal = proposal.Normalized()
		if len(proposal.RequiredEvidenceKinds) != 1 || proposal.RequiredEvidenceKinds[0] != EvidenceKindValidation {
			continue
		}
		if validationIndex >= 0 {
			return -1, fmt.Errorf("runtime semantic proof set for coverage %q cannot uniquely assign source metric or artifact requirements", proofSet.CoverageWorkItemID)
		}
		validationIndex = index
	}
	if validationIndex >= 0 {
		return validationIndex, nil
	}
	if len(proofSet.Proposals) == 1 {
		return 0, nil
	}
	return -1, fmt.Errorf("runtime semantic proof set for coverage %q cannot uniquely assign source metric or artifact requirements", proofSet.CoverageWorkItemID)
}

// mergeRuntimeSemanticBindingTarget preserves one logical target surface for
// an independent proof set. A prospective state change and a later validation
// necessarily use different owner policies: File authorizes creation while a
// validation owner observes the resulting exact path. They may share one CWI
// binding only when both facts name that exact same logical target.
func mergeRuntimeSemanticBindingTarget(
	targets []string, selector string, prospective []string, selectorOwner string,
	currentTargets []string, currentSelector string, currentProspective []string, currentOwner string,
) ([]string, string, []string, string, error) {
	if len(prospective) != 0 {
		if len(currentProspective) == 0 && currentSelector == "" && sameStrings(currentTargets, prospective) {
			return targets, selector, prospective, selectorOwner, nil
		}
		if sameStrings(currentProspective, prospective) && currentSelector == selector && sameStrings(currentTargets, targets) && currentOwner == selectorOwner {
			return targets, selector, prospective, selectorOwner, nil
		}
		return nil, "", nil, "", fmt.Errorf("semantic prospective target surface mismatch")
	}
	if len(currentProspective) != 0 {
		if selector == "" && sameStrings(targets, currentProspective) {
			return currentTargets, currentSelector, currentProspective, currentOwner, nil
		}
		return nil, "", nil, "", fmt.Errorf("semantic prospective target surface mismatch")
	}
	if !sameStrings(targets, currentTargets) || selector != currentSelector {
		return nil, "", nil, "", fmt.Errorf("semantic target surface mismatch")
	}
	if selector != "" && selectorOwner != currentOwner {
		return nil, "", nil, "", fmt.Errorf("semantic selector owner mismatch")
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

func semanticBindingTarget(proposal RuntimeSemanticProofProposal, fact RuntimeSemanticProofFeasibility) ([]string, string, []string, error) {
	target := proposal.Target.Normalized()
	factTargets := normalizeStrings(fact.TargetRefs)
	switch target.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		if !sameStrings(factTargets, target.ExactTargetRefs) {
			return nil, "", nil, fmt.Errorf("runtime semantic exact target owner fact does not match proposal")
		}
		return factTargets, "", nil, nil
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) != 0 && !sameStrings(factTargets, target.OwnerScopeTargetRefs) {
			return nil, "", nil, fmt.Errorf("runtime semantic owner scope fact does not match proposal")
		}
		selector := ""
		if len(target.OwnerSelectorRefs) == 1 {
			selector = target.OwnerSelectorRefs[0]
		}
		return factTargets, selector, nil, nil
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		// A prospective target is not required to exist in the owner inventory.
		// Feasibility may therefore legitimately return no TargetRefs; when it
		// does return one, it must agree with the sealed prospective ref.
		if len(factTargets) != 0 && !sameStrings(factTargets, target.ProspectiveTargetRefs) {
			return nil, "", nil, fmt.Errorf("runtime semantic prospective target owner fact does not match proposal")
		}
		return nil, target.OwnerSelectorRefs[0], append([]string(nil), target.ProspectiveTargetRefs...), nil
	default:
		return nil, "", nil, fmt.Errorf("runtime semantic binding has unsupported target policy %q", target.Policy)
	}
}
