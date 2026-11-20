package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition     = "singleton_exact_owner_fact_acquisition"
	RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface = "singleton_unique_direct_validation_surface"
)

// RuntimeSemanticCompiledProofBatch records deterministic code origin for a
// compiled singleton proof batch, never provider provenance, owner feasibility,
// review approval, or execution/validation credit.
type RuntimeSemanticCompiledProofBatch struct {
	Rule         string
	ManifestHash string
	BatchID      string
	DocumentHash string
}

func (c RuntimeSemanticCompiledProofBatch) Normalized() RuntimeSemanticCompiledProofBatch {
	c.Rule = strings.TrimSpace(c.Rule)
	c.ManifestHash = strings.TrimSpace(c.ManifestHash)
	c.BatchID = strings.TrimSpace(c.BatchID)
	c.DocumentHash = strings.TrimSpace(c.DocumentHash)
	return c
}

func NewRuntimeSemanticCompiledProofBatch(m RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch, rule string, d RuntimeSemanticPlanDocument) *RuntimeSemanticCompiledProofBatch {
	return &RuntimeSemanticCompiledProofBatch{
		Rule:         strings.TrimSpace(rule),
		ManifestHash: strings.TrimSpace(m.Hash),
		BatchID:      strings.TrimSpace(batch.BatchID),
		DocumentHash: StableRuntimeSemanticPlanDocumentHash(d),
	}
}

func (c *RuntimeSemanticCompiledProofBatch) Validate(m *RuntimeSemanticPlanManifest, batch *RuntimeSemanticPlanBatch, d *RuntimeSemanticPlanDocument) error {
	if c == nil {
		return nil
	}
	if m == nil || batch == nil || d == nil {
		return fmt.Errorf("compiled semantic proof batch requires original manifest, batch and document")
	}
	normalized := c.Normalized()
	expected, expectedRule, ok := CompileRuntimeSingletonProofBatch(*m, *batch)
	if !ok || normalized.Rule != expectedRule || normalized.ManifestHash != m.Hash ||
		normalized.BatchID != batch.BatchID || normalized.DocumentHash != StableRuntimeSemanticPlanDocumentHash(expected) ||
		!reflect.DeepEqual(expected.Normalized(), d.Normalized()) {
		return fmt.Errorf("compiled semantic proof batch differs from exact singleton rule")
	}
	return nil
}

// CompileRuntimeSingletonProofBatch deterministically compiles a proof-batch
// document only when the active single-CWI batch has an unambiguous closed
// contract with zero semantic ambiguity and zero validation input review.
func CompileRuntimeSingletonProofBatch(m RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch) (RuntimeSemanticPlanDocument, string, bool) {
	m = m.Normalized()
	batch = batch.Normalized()
	if batch.BatchID == "" || batch.ManifestHash != m.Hash || batch.DefinitionSeedHash != m.DefinitionSeedHash ||
		len(batch.CoverageWorkItemIDs) != 1 || m.FailureInputReview != nil ||
		RuntimeSemanticPurposeChoice(m, RuntimeSemanticPlanStepProofBatch) {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	coverageID := batch.CoverageWorkItemIDs[0]
	var obligation RuntimeSemanticProofObligation
	obligationFound := false
	for _, candidate := range m.ProofObligations {
		if candidate.CoverageWorkItemID == coverageID {
			obligation = candidate.Normalized()
			obligationFound = true
			break
		}
	}
	if !obligationFound || obligation.ImplementationWork != nil || obligation.ValidationFailureRepair != nil || obligation.HistoricalAcceptance != nil {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	declarationRef := RuntimeSemanticCurrentDeclarationRef(obligation)
	if declarationRef == "" {
		return RuntimeSemanticPlanDocument{}, "", false
	}

	if obligation.OwnerFactAcquisition != nil {
		return compileRuntimeSingletonExactOwnerFactAcquisition(m, batch, obligation, declarationRef)
	}
	return compileRuntimeSingletonUniqueDirectValidationSurface(m, batch, obligation, declarationRef)
}

func compileRuntimeSingletonExactOwnerFactAcquisition(
	m RuntimeSemanticPlanManifest,
	batch RuntimeSemanticPlanBatch,
	obligation RuntimeSemanticProofObligation,
	declarationRef string,
) (RuntimeSemanticPlanDocument, string, bool) {
	acq := obligation.OwnerFactAcquisition.Normalized()
	if acq.AcquisitionTargetPolicy != RuntimeOperationTargetPolicyExistingOwnerScope ||
		!containsString(m.AllowedOwnerModuleIDs, acq.AcquisitionOwnerModuleID) ||
		!runtimeOperationKindsContain(m.AllowedOperationKinds, acq.AcquisitionOperationKind) ||
		!compiledProofBatchManifestAllowsTargetPolicy(m.AllowedTargetPolicies, acq.AcquisitionTargetPolicy) ||
		!RuntimeOperationKindSupportsCoverageEvidence(acq.AcquisitionOperationKind, EvidenceKindStateChange) {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	catalog, found := compiledProofBatchOwnerCatalog(m, acq.AcquisitionOwnerModuleID)
	if !found || !runtimeOperationKindsContain(catalog.OperationKinds, acq.AcquisitionOperationKind) {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	if err := validateRuntimeSemanticOwnerFactAcquisitionAgainstCatalogs(obligation.CoverageWorkItemID, acq, map[string]RuntimeOperationAdmissibility{catalog.OwnerModuleID: catalog}); err != nil {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	effectScope, ok := compileRuntimeSingletonDurableEffectScope(catalog, acq.AcquisitionOperationKind, acq.AcquisitionOperationVariantID)
	if !ok {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	target := RuntimeOperationTargetAuthorization{
		Policy:            RuntimeOperationTargetPolicyExistingOwnerScope,
		OwnerSelectorRefs: []string{acq.AcquisitionTargetSelectorRef},
	}
	if len(obligation.DeclarationTargetSelectorCandidateRefs) != 0 {
		if len(obligation.DeclarationTargetSelectorCandidateRefs) != 1 || !containsString(catalog.TargetRefs, obligation.DeclarationTargetSelectorCandidateRefs[0]) {
			return RuntimeSemanticPlanDocument{}, "", false
		}
		target.OwnerScopeTargetRefs = []string{obligation.DeclarationTargetSelectorCandidateRefs[0]}
	}
	doc := RuntimeSemanticPlanDocument{
		SchemaVersion:            RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:             m.Hash,
		DefinitionSeedHash:       m.DefinitionSeedHash,
		PlanStep:                 RuntimeSemanticPlanStepProofBatch,
		BatchID:                  batch.BatchID,
		BatchSequence:            batch.Sequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{obligation.CoverageWorkItemID},
		Proposals: []RuntimeSemanticProofProposal{{
			CoverageWorkItemID:    obligation.CoverageWorkItemID,
			OwnerModuleID:         acq.AcquisitionOwnerModuleID,
			OperationKind:         acq.AcquisitionOperationKind,
			OperationVariantID:    acq.AcquisitionOperationVariantID,
			Target:                target,
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			RequiredEffectScopes:  []RuntimeEffectScope{effectScope},
			DeclarationRef:        declarationRef,
		}},
	}.Normalized()
	if err := doc.ValidateAgainstManifest(m); err != nil {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	return doc, RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition, true
}

func compileRuntimeSingletonUniqueDirectValidationSurface(
	m RuntimeSemanticPlanManifest,
	batch RuntimeSemanticPlanBatch,
	obligation RuntimeSemanticProofObligation,
	declarationRef string,
) (RuntimeSemanticPlanDocument, string, bool) {
	if len(obligation.RequiredEvidenceKinds) != 1 || obligation.RequiredEvidenceKinds[0] != EvidenceKindValidation {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	var candidates []RuntimeSemanticProofProposal
	for _, catalog := range m.OwnerCatalogs {
		if !containsString(m.AllowedOwnerModuleIDs, catalog.OwnerModuleID) {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			if surface.Readiness != RuntimeValidationSurfaceReady ||
				surface.ProofApplicability != RuntimeValidationProofDirect ||
				surface.OwnerModuleID != catalog.OwnerModuleID ||
				!containsString(catalog.ValidationRecipeRefs, surface.ID) ||
				!runtimeOperationKindsContain(m.AllowedOperationKinds, surface.OperationKind) ||
				!runtimeOperationKindsContain(catalog.OperationKinds, surface.OperationKind) ||
				!RuntimeOperationKindSupportsCoverageEvidence(surface.OperationKind, EvidenceKindValidation) ||
				!RuntimeEffectIsNonDurable(surface.EffectScope) {
				continue
			}
			if len(obligation.DeclarationAcceptedStepKinds) != 0 && !runtimeOperationKindsContain(obligation.DeclarationAcceptedStepKinds, surface.OperationKind) {
				continue
			}
			if obligation.ValidationProofRequirement != nil {
				reqRecipeID := obligation.ValidationProofRequirement.Normalized().ValidationRecipeID
				if reqRecipeID != "" && surface.ID != reqRecipeID {
					continue
				}
			}
			if surface.OperationKind == RuntimeBlockKindManagedValidationSession {
				if obligation.ValidationProofRequirement == nil {
					continue
				}
				requirement := obligation.ValidationProofRequirement.Normalized()
				if requirement.ValidationRecipeID != "" && surface.ID != requirement.ValidationRecipeID {
					continue
				}
				if _, _, exact := RuntimeValidationSurfaceForProofRequirement([]RuntimeValidationSurface{surface}, requirement); !exact {
					continue
				}
			}
			for _, binding := range surface.ProofRequirementBindings {
				if binding.CoverageWorkItemID == obligation.CoverageWorkItemID && binding.DeclarationRef == declarationRef && binding.InputReview != nil {
					return RuntimeSemanticPlanDocument{}, "", false
				}
			}
			variantID, ok := compileRuntimeSingletonVariantID(catalog, surface.OperationKind)
			if !ok {
				return RuntimeSemanticPlanDocument{}, "", false
			}
			target, ok := compileRuntimeSingletonValidationTarget(m, catalog, obligation, surface.OperationKind, variantID)
			if !ok {
				return RuntimeSemanticPlanDocument{}, "", false
			}
			candidates = append(candidates, RuntimeSemanticProofProposal{
				CoverageWorkItemID:    obligation.CoverageWorkItemID,
				OwnerModuleID:         surface.OwnerModuleID,
				OperationKind:         surface.OperationKind,
				OperationVariantID:    variantID,
				Target:                target,
				RequiredEvidenceKinds: []string{EvidenceKindValidation},
				RequiredEffectScopes:  []RuntimeEffectScope{surface.EffectScope},
				ValidationRecipeID:    surface.ID,
				DeclarationRef:        declarationRef,
			})
		}
	}
	if len(candidates) != 1 {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	doc := RuntimeSemanticPlanDocument{
		SchemaVersion:            RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:             m.Hash,
		DefinitionSeedHash:       m.DefinitionSeedHash,
		PlanStep:                 RuntimeSemanticPlanStepProofBatch,
		BatchID:                  batch.BatchID,
		BatchSequence:            batch.Sequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{obligation.CoverageWorkItemID},
		Proposals:                candidates,
	}.Normalized()
	if err := doc.ValidateAgainstManifest(m); err != nil {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	return doc, RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface, true
}

func compiledProofBatchManifestAllowsTargetPolicy(allowed []RuntimeOperationTargetPolicy, want RuntimeOperationTargetPolicy) bool {
	for _, policy := range allowed {
		if policy == want {
			return true
		}
	}
	return false
}

func compiledProofBatchOwnerCatalog(m RuntimeSemanticPlanManifest, ownerModuleID string) (RuntimeOperationAdmissibility, bool) {
	for _, catalog := range m.OwnerCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog, true
		}
	}
	return RuntimeOperationAdmissibility{}, false
}

func compileRuntimeSingletonDurableEffectScope(catalog RuntimeOperationAdmissibility, kind, variantID string) (RuntimeEffectScope, bool) {
	var policyEffects []RuntimeEffectScope
	for _, policy := range catalog.OperationPolicies {
		if policy.OwnerModuleID != catalog.OwnerModuleID || policy.OperationKind != kind || len(policy.ParameterMatches) != 0 {
			continue
		}
		if !containsRuntimeEffectScope(policyEffects, policy.EffectScope) {
			policyEffects = append(policyEffects, policy.EffectScope)
		}
	}
	if len(policyEffects) != 0 {
		if len(policyEffects) != 1 || !RuntimeEffectIsDurable(policyEffects[0]) {
			return "", false
		}
		return policyEffects[0], true
	}
	if variantID != "" {
		if contract, found := RuntimeOperationContractForKind(kind); found {
			for _, variant := range contract.Variants {
				if variant.ID == variantID && variant.EffectScope != RuntimeEffectUnknown {
					return variant.EffectScope, RuntimeEffectIsDurable(variant.EffectScope)
				}
			}
		}
	}
	descriptor, found := RuntimeOperationDescriptorForKind(kind)
	if !found || descriptor.OwnerModuleID != catalog.OwnerModuleID || !RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
		return "", false
	}
	return descriptor.DefaultEffectScope, true
}

func containsRuntimeEffectScope(values []RuntimeEffectScope, want RuntimeEffectScope) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func compileRuntimeSingletonVariantID(catalog RuntimeOperationAdmissibility, kind string) (string, bool) {
	contract, found := RuntimeOperationContractForKind(kind)
	if !found || len(contract.Variants) == 0 {
		return "", true
	}
	var matching []string
	for _, variant := range catalog.OperationVariants {
		if variant.Kind == kind && variant.VariantID != "" && !containsString(matching, variant.VariantID) {
			matching = append(matching, variant.VariantID)
		}
	}
	if len(matching) != 1 {
		return "", false
	}
	return matching[0], true
}

func compileRuntimeSingletonValidationTarget(
	m RuntimeSemanticPlanManifest,
	catalog RuntimeOperationAdmissibility,
	obligation RuntimeSemanticProofObligation,
	kind string,
	variantID string,
) (RuntimeOperationTargetAuthorization, bool) {
	exactAllowed := compiledProofBatchManifestAllowsTargetPolicy(m.AllowedTargetPolicies, RuntimeOperationTargetPolicyExactTarget)
	if exactAllowed {
		catalogAllowed, _, _, _, err := runtimeOperationAdmissibilityAllowsTargetPolicyNormalized(catalog, kind, variantID, RuntimeOperationTargetPolicyExactTarget)
		exactAllowed = err == nil && catalogAllowed
	}
	existingAllowed := compiledProofBatchManifestAllowsTargetPolicy(m.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	var existingSelectorRefs []string
	if existingAllowed {
		catalogAllowed, selectors, _, _, err := runtimeOperationAdmissibilityAllowsTargetPolicyNormalized(catalog, kind, variantID, RuntimeOperationTargetPolicyExistingOwnerScope)
		existingAllowed = err == nil && catalogAllowed
		existingSelectorRefs = selectors
	}

	if len(obligation.DeclarationTargetSelectorCandidateRefs) != 0 {
		var intersection []string
		for _, candidate := range obligation.DeclarationTargetSelectorCandidateRefs {
			if containsString(catalog.TargetRefs, candidate) && !containsString(intersection, candidate) {
				intersection = append(intersection, candidate)
			}
		}
		if len(intersection) != 1 {
			return RuntimeOperationTargetAuthorization{}, false
		}
		if exactAllowed {
			return RuntimeOperationTargetAuthorization{
				Policy:          RuntimeOperationTargetPolicyExactTarget,
				ExactTargetRefs: intersection,
			}, true
		}
		if existingAllowed {
			return RuntimeOperationTargetAuthorization{
				Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
				OwnerScopeTargetRefs: intersection,
			}, true
		}
		return RuntimeOperationTargetAuthorization{}, false
	}

	if len(catalog.TargetRefs) == 0 && len(existingSelectorRefs) == 1 && existingAllowed {
		return RuntimeOperationTargetAuthorization{
			Policy:            RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerSelectorRefs: []string{existingSelectorRefs[0]},
		}, true
	}
	if len(catalog.TargetRefs) == 1 && len(existingSelectorRefs) == 0 {
		target := []string{catalog.TargetRefs[0]}
		if exactAllowed {
			return RuntimeOperationTargetAuthorization{
				Policy:          RuntimeOperationTargetPolicyExactTarget,
				ExactTargetRefs: target,
			}, true
		}
		if existingAllowed {
			return RuntimeOperationTargetAuthorization{
				Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
				OwnerScopeTargetRefs: target,
			}, true
		}
	}
	return RuntimeOperationTargetAuthorization{}, false
}
