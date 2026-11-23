package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition                 = "singleton_exact_owner_fact_acquisition"
	RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface             = "singleton_unique_direct_validation_surface"
	RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurfaceReusedReview = "singleton_unique_direct_validation_surface_reused_review"
)

// RuntimeSemanticCompiledProofBatch records deterministic code origin for a
// compiled singleton proof batch, never provider provenance, owner feasibility,
// review approval, or execution/validation credit.
type RuntimeSemanticCompiledProofBatch struct {
	Rule                          string
	ManifestHash                  string
	BatchID                       string
	DocumentHash                  string
	ReusedInputReviewManifestHash string `json:",omitempty"`
	ReusedInputReviewDocumentHash string `json:",omitempty"`
	ReusedInputReviewSubjectHash  string `json:",omitempty"`
}

func (c RuntimeSemanticCompiledProofBatch) Normalized() RuntimeSemanticCompiledProofBatch {
	c.Rule = strings.TrimSpace(c.Rule)
	c.ManifestHash = strings.TrimSpace(c.ManifestHash)
	c.BatchID = strings.TrimSpace(c.BatchID)
	c.DocumentHash = strings.TrimSpace(c.DocumentHash)
	c.ReusedInputReviewManifestHash = strings.TrimSpace(c.ReusedInputReviewManifestHash)
	c.ReusedInputReviewDocumentHash = strings.TrimSpace(c.ReusedInputReviewDocumentHash)
	c.ReusedInputReviewSubjectHash = strings.TrimSpace(c.ReusedInputReviewSubjectHash)
	return c
}

// CollectRuntimeSemanticPlanningHistory materializes a ledger's complete
// chronological generation lineage (archived segments followed by tail).
func CollectRuntimeSemanticPlanningHistory(ledger RuntimeSemanticPlanningLedger) []RuntimeSemanticPlanningGeneration {
	count := ledger.HistoryCount()
	if count == 0 {
		return nil
	}
	history := make([]RuntimeSemanticPlanningGeneration, 0, count)
	for _, generation := range ledger.AllHistory() {
		history = append(history, generation)
	}
	return history
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
	return c.ValidateWithHistory(m, batch, d, nil)
}

func (c *RuntimeSemanticCompiledProofBatch) ValidateWithHistory(
	m *RuntimeSemanticPlanManifest,
	batch *RuntimeSemanticPlanBatch,
	d *RuntimeSemanticPlanDocument,
	precedingHistory []RuntimeSemanticPlanningGeneration,
) error {
	if c == nil {
		return nil
	}
	if m == nil || batch == nil || d == nil {
		return fmt.Errorf("compiled semantic proof batch requires original manifest, batch and document")
	}
	normalized := c.Normalized()
	if normalized.Rule == RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurfaceReusedReview {
		if normalized.ReusedInputReviewManifestHash == "" ||
			normalized.ReusedInputReviewDocumentHash == "" ||
			normalized.ReusedInputReviewSubjectHash == "" {
			return fmt.Errorf("compiled semantic proof batch reused review requires complete source provenance")
		}
		sourceGen, found := findCompiledProofBatchSourceGeneration(
			precedingHistory,
			normalized.ReusedInputReviewManifestHash,
			normalized.ReusedInputReviewDocumentHash,
		)
		if !found {
			return fmt.Errorf("compiled semantic proof batch reused review lacks matching original generation")
		}
		expected, expectedCompiled, ok := CompileRuntimeSingletonProofBatchWithHistory(*m, *batch, []RuntimeSemanticPlanningGeneration{sourceGen})
		if !ok || expectedCompiled == nil ||
			*expectedCompiled != normalized ||
			!reflect.DeepEqual(expected.Normalized(), d.Normalized()) {
			return fmt.Errorf("compiled semantic proof batch differs from exact reused-review singleton rule")
		}
		return nil
	}
	if normalized.ReusedInputReviewManifestHash != "" ||
		normalized.ReusedInputReviewDocumentHash != "" ||
		normalized.ReusedInputReviewSubjectHash != "" {
		return fmt.Errorf("non-reused compiled semantic proof batch cannot carry reused review provenance")
	}
	expected, expectedCompiled, ok := CompileRuntimeSingletonProofBatchWithHistory(*m, *batch, nil)
	if !ok || expectedCompiled == nil ||
		*expectedCompiled != normalized ||
		!reflect.DeepEqual(expected.Normalized(), d.Normalized()) {
		return fmt.Errorf("compiled semantic proof batch differs from exact singleton rule")
	}
	return nil
}

// CompileRuntimeSingletonProofBatch deterministically compiles a proof-batch
// document only when the active single-CWI batch has an unambiguous closed
// contract with zero semantic ambiguity and zero validation input review.
func CompileRuntimeSingletonProofBatch(m RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch) (RuntimeSemanticPlanDocument, string, bool) {
	doc, compiled, ok := CompileRuntimeSingletonProofBatchWithHistory(m, batch, nil)
	if !ok || compiled == nil {
		return RuntimeSemanticPlanDocument{}, "", false
	}
	return doc, compiled.Rule, true
}

// CompileRuntimeSingletonProofBatchWithHistory deterministically compiles an
// unambiguous single-CWI proof batch, including reusing an already-committed
// original consistent ValidationInputReview from precedingHistory when the
// exact (CoverageWorkItemID, DeclarationRef, RequirementHash, SubjectHash) matches.
func CompileRuntimeSingletonProofBatchWithHistory(
	m RuntimeSemanticPlanManifest,
	batch RuntimeSemanticPlanBatch,
	precedingHistory []RuntimeSemanticPlanningGeneration,
) (RuntimeSemanticPlanDocument, *RuntimeSemanticCompiledProofBatch, bool) {
	m = m.Normalized()
	batch = batch.Normalized()
	if batch.BatchID == "" || batch.ManifestHash != m.Hash || batch.DefinitionSeedHash != m.DefinitionSeedHash ||
		len(batch.CoverageWorkItemIDs) != 1 || m.FailureInputReview != nil ||
		RuntimeSemanticPurposeChoice(m, RuntimeSemanticPlanStepProofBatch) {
		return RuntimeSemanticPlanDocument{}, nil, false
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
		return RuntimeSemanticPlanDocument{}, nil, false
	}
	declarationRef := RuntimeSemanticCurrentDeclarationRef(obligation)
	if declarationRef == "" {
		return RuntimeSemanticPlanDocument{}, nil, false
	}

	if obligation.OwnerFactAcquisition != nil {
		doc, rule, ok := compileRuntimeSingletonExactOwnerFactAcquisition(m, batch, obligation, declarationRef)
		if !ok {
			return RuntimeSemanticPlanDocument{}, nil, false
		}
		return doc, NewRuntimeSemanticCompiledProofBatch(m, batch, rule, doc), true
	}
	return compileRuntimeSingletonUniqueDirectValidationSurface(m, batch, obligation, declarationRef, precedingHistory)
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
	precedingHistory []RuntimeSemanticPlanningGeneration,
) (RuntimeSemanticPlanDocument, *RuntimeSemanticCompiledProofBatch, bool) {
	if len(obligation.RequiredEvidenceKinds) != 1 || obligation.RequiredEvidenceKinds[0] != EvidenceKindValidation {
		return RuntimeSemanticPlanDocument{}, nil, false
	}
	var candidates []RuntimeSemanticProofProposal
	var reusedCompiled []*RuntimeSemanticCompiledProofBatch
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
			var matchedReview *RuntimeValidationInputReview
			var sourceManifestHash, sourceDocHash string
			matchedBindings := 0
			for _, binding := range surface.ProofRequirementBindings {
				if binding.CoverageWorkItemID != obligation.CoverageWorkItemID || binding.DeclarationRef != declarationRef {
					continue
				}
				matchedBindings++
				if binding.InputReview == nil {
					continue
				}
				if matchedReview != nil || len(precedingHistory) == 0 {
					return RuntimeSemanticPlanDocument{}, nil, false
				}
				review, srcManifest, srcDoc, found := findCommittedConsistentValidationInputReview(
					precedingHistory,
					obligation.CoverageWorkItemID,
					declarationRef,
					binding.RequirementHash,
					binding.InputReview.Hash,
				)
				if !found || review.ValidateAgainstBinding(binding) != nil {
					return RuntimeSemanticPlanDocument{}, nil, false
				}
				matchedReview = review
				sourceManifestHash = srcManifest
				sourceDocHash = srcDoc
			}
			if matchedReview != nil && matchedBindings > 1 {
				return RuntimeSemanticPlanDocument{}, nil, false
			}
			variantID, ok := compileRuntimeSingletonVariantID(catalog, surface.OperationKind)
			if !ok {
				return RuntimeSemanticPlanDocument{}, nil, false
			}
			target, ok := compileRuntimeSingletonValidationTarget(m, catalog, obligation, surface.OperationKind, variantID)
			if !ok {
				return RuntimeSemanticPlanDocument{}, nil, false
			}
			proposal := RuntimeSemanticProofProposal{
				CoverageWorkItemID:    obligation.CoverageWorkItemID,
				OwnerModuleID:         surface.OwnerModuleID,
				OperationKind:         surface.OperationKind,
				OperationVariantID:    variantID,
				Target:                target,
				RequiredEvidenceKinds: []string{EvidenceKindValidation},
				RequiredEffectScopes:  []RuntimeEffectScope{surface.EffectScope},
				ValidationRecipeID:    surface.ID,
				DeclarationRef:        declarationRef,
			}
			var reusedMeta *RuntimeSemanticCompiledProofBatch
			if matchedReview != nil {
				clonedReview := matchedReview.Clone()
				proposal.ValidationInputReview = &clonedReview
				reusedMeta = &RuntimeSemanticCompiledProofBatch{
					Rule:                          RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurfaceReusedReview,
					ReusedInputReviewManifestHash: sourceManifestHash,
					ReusedInputReviewDocumentHash: sourceDocHash,
					ReusedInputReviewSubjectHash:  matchedReview.SubjectHash,
				}
			}
			candidates = append(candidates, proposal)
			reusedCompiled = append(reusedCompiled, reusedMeta)
		}
	}
	if len(candidates) != 1 {
		return RuntimeSemanticPlanDocument{}, nil, false
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
		return RuntimeSemanticPlanDocument{}, nil, false
	}
	if meta := reusedCompiled[0]; meta != nil {
		meta.ManifestHash = strings.TrimSpace(m.Hash)
		meta.BatchID = strings.TrimSpace(batch.BatchID)
		meta.DocumentHash = StableRuntimeSemanticPlanDocumentHash(doc)
		return doc, meta, true
	}
	return doc, NewRuntimeSemanticCompiledProofBatch(m, batch, RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface, doc), true
}

func findCompiledProofBatchSourceGeneration(
	history []RuntimeSemanticPlanningGeneration,
	manifestHash string,
	docHash string,
) (RuntimeSemanticPlanningGeneration, bool) {
	if manifestHash == "" || docHash == "" {
		return RuntimeSemanticPlanningGeneration{}, false
	}
	for _, generation := range history {
		snap := generation.Snapshot
		if snap.Manifest == nil || snap.Manifest.Hash != manifestHash ||
			snap.Phase == RuntimeSemanticPlanningRequired || snap.Phase == RuntimeSemanticPlanningFailed {
			continue
		}
		compiledBatchIDs := make(map[string]struct{}, len(snap.CompiledProofBatches))
		for _, compiled := range snap.CompiledProofBatches {
			compiledBatchIDs[compiled.BatchID] = struct{}{}
		}
		for _, doc := range snap.BatchDocuments {
			if _, isCompiled := compiledBatchIDs[doc.BatchID]; isCompiled {
				continue
			}
			if StableRuntimeSemanticPlanDocumentHash(doc) == docHash {
				return generation, true
			}
		}
	}
	return RuntimeSemanticPlanningGeneration{}, false
}

func findCommittedConsistentValidationInputReview(
	history []RuntimeSemanticPlanningGeneration,
	coverageID common.CoverageWorkItemID,
	declarationRef string,
	requirementHash string,
	subjectHash string,
) (*RuntimeValidationInputReview, string, string, bool) {
	if coverageID == "" || declarationRef == "" || requirementHash == "" || subjectHash == "" {
		return nil, "", "", false
	}
	for _, generation := range history {
		snap := generation.Snapshot
		if snap.Manifest == nil || snap.Phase == RuntimeSemanticPlanningRequired || snap.Phase == RuntimeSemanticPlanningFailed {
			continue
		}
		compiledBatchIDs := make(map[string]struct{}, len(snap.CompiledProofBatches))
		for _, compiled := range snap.CompiledProofBatches {
			compiledBatchIDs[compiled.BatchID] = struct{}{}
		}
		for _, doc := range snap.BatchDocuments {
			if _, isCompiled := compiledBatchIDs[doc.BatchID]; isCompiled {
				continue
			}
			batch, batchFound := runtimeSemanticPlanningBatch(snap.Batches, doc.BatchID)
			if !batchFound ||
				batch.State == RuntimeSemanticPlanBatchPending ||
				batch.State == RuntimeSemanticPlanBatchRequesting ||
				batch.State == RuntimeSemanticPlanBatchFailed {
				continue
			}
			docHash := StableRuntimeSemanticPlanDocumentHash(doc)
			if batch.DocumentHash != docHash {
				continue
			}
			for _, proposal := range doc.Proposals {
				if proposal.CoverageWorkItemID != coverageID ||
					proposal.DeclarationRef != declarationRef ||
					proposal.ValidationInputReview == nil ||
					proposal.ValidationInputReview.SubjectHash != subjectHash {
					continue
				}
				if RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != RuntimeValidationInputConsistent {
					continue
				}
				sourceBinding, bindingFound := RuntimeSemanticValidationInputReviewBinding(proposal, snap.Manifest.OwnerCatalogs)
				if !bindingFound ||
					sourceBinding.RequirementHash != requirementHash ||
					sourceBinding.InputReview == nil ||
					sourceBinding.InputReview.Hash != subjectHash ||
					proposal.ValidationInputReview.ValidateAgainstBinding(sourceBinding) != nil {
					continue
				}
				cloned := proposal.ValidationInputReview.Clone()
				return &cloned, snap.Manifest.Hash, docHash, true
			}
		}
	}
	return nil, "", "", false
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
