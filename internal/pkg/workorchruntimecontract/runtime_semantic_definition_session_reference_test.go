package workorchruntimecontract

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Frozen complete standalone manifest-to-Definition source gate.
func referenceSemanticManifestDefinitionGate(manifest RuntimeSemanticPlanManifest, seed RuntimeDefinitionSeed, definitionFocuses []RuntimeDefinitionFocus) error {
	validated, err := validateRuntimeSemanticPlanManifest(manifest)
	if err != nil {
		return err
	}
	if err := seed.Validate(); err != nil {
		return fmt.Errorf("runtime semantic manifest authoritative Definition seed: %w", err)
	}
	manifest = validated.manifest
	seed = seed.Clone()
	if manifest.DefinitionRevision != seed.DefinitionRevision || manifest.DefinitionSeedHash != seed.Hash {
		return fmt.Errorf("runtime semantic manifest does not match authoritative Definition seed identity")
	}
	manifestCoverage := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		manifestCoverage[item.ID] = struct{}{}
	}
	seedByCoverage := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		seedByCoverage[item.ID] = item
	}
	for _, actual := range manifest.CoverageItems {
		expected, found := seedByCoverage[actual.ID]
		if !found {
			return fmt.Errorf("runtime semantic manifest coverage %q is outside authoritative Definition seed", actual.ID)
		}
		filteredPrerequisites := make([]common.CoverageWorkItemID, 0, len(expected.PrerequisiteCoverageWorkItemIDs))
		for _, prerequisiteID := range expected.PrerequisiteCoverageWorkItemIDs {
			if _, retained := manifestCoverage[prerequisiteID]; retained {
				filteredPrerequisites = append(filteredPrerequisites, prerequisiteID)
			}
		}
		expected.PrerequisiteCoverageWorkItemIDs = filteredPrerequisites
		if !sameRuntimeCoverageItem(actual, expected) {
			return fmt.Errorf("runtime semantic manifest coverage %q does not match authoritative Definition seed body", actual.ID)
		}
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, definitionFocuses); err != nil {
		return fmt.Errorf("runtime semantic manifest Definition focuses: %w", err)
	}
	sourceByCoverage, err := runtimeSemanticBindingSourceFocuses(definitionFocuses)
	if err != nil {
		return fmt.Errorf("runtime semantic manifest Definition focuses: %w", err)
	}
	catalogs := append([]RuntimeOperationAdmissibility(nil), manifest.OwnerCatalogs...)
	seenCatalogOwners := make(map[string]struct{}, len(catalogs))
	for _, catalog := range catalogs {
		seenCatalogOwners[catalog.OwnerModuleID] = struct{}{}
	}
	for _, catalog := range manifest.OwnerFactDependencyCatalogs {
		if _, alreadyIncluded := seenCatalogOwners[catalog.OwnerModuleID]; alreadyIncluded {
			continue
		}
		seenCatalogOwners[catalog.OwnerModuleID] = struct{}{}
		catalogs = append(catalogs, catalog)
	}
	var proofCatalogs *runtimeSemanticProofCatalogs
	for _, actual := range manifest.ProofObligations {
		item, found := runtimeCoverageItemForID(manifest.CoverageItems, actual.CoverageWorkItemID)
		if !found {
			return fmt.Errorf("runtime semantic manifest proof obligation coverage %q is outside manifest", actual.CoverageWorkItemID)
		}
		source, found := sourceByCoverage[actual.CoverageWorkItemID]
		if !found {
			return fmt.Errorf("runtime semantic manifest proof obligation coverage %q has no immutable Definition focus", actual.CoverageWorkItemID)
		}
		if actual.ImplementationWork != nil {
			expectedBase, compileErr := compileRuntimeSemanticDefinitionProofObligation(manifest.DefinitionRevision, item, source)
			if compileErr != nil {
				return compileErr
			}
			actualBase := actual.Clone()
			actualBase.ImplementationWork = nil
			if !sameRuntimeSemanticProofObligations([]RuntimeSemanticProofObligation{actualBase}, []RuntimeSemanticProofObligation{expectedBase}) || source.ImplementationWorkPolicy == nil || actual.ImplementationWork.PolicyHash != StableRuntimeImplementationWorkPolicyHash(*source.ImplementationWorkPolicy) {
				return fmt.Errorf("runtime semantic work terminal declaration/policy differs from immutable Definition")
			}
			continue
		}
		if actual.ValidationFailureRepair == nil {
			// 同一 manifest 的各 CWI 共用本次完整 raw 门禁后的目录，不保存成功状态。
			if proofCatalogs == nil {
				validated, err := validateRuntimeSemanticProofCatalogs(catalogs)
				if err != nil {
					return fmt.Errorf("runtime semantic manifest Definition proof obligation coverage %q: %w", actual.CoverageWorkItemID, err)
				}
				proofCatalogs = &validated
			}
			recompiled, compileErr := compileRuntimeSemanticProofObligationsWithValidatedCatalogs(
				manifest.DefinitionRevision,
				map[common.CoverageWorkItemID]RuntimeDefinitionFocus{item.ID: source},
				[]RuntimeCoverageItem{item},
				*proofCatalogs,
			)
			if compileErr != nil {
				return fmt.Errorf("runtime semantic manifest Definition proof obligation coverage %q: %w", actual.CoverageWorkItemID, compileErr)
			}
			if len(recompiled) != 1 || !sameRuntimeSemanticProofObligations([]RuntimeSemanticProofObligation{actual}, recompiled) {
				return fmt.Errorf("runtime semantic manifest proof obligation coverage %q does not match immutable Definition focus", actual.CoverageWorkItemID)
			}
			continue
		}

		// Repair catalogs intentionally replace the terminal proof owner's
		// catalog for one bounded generation. Revalidate the immutable terminal
		// declaration directly and verify that the selected repair route comes
		// from the Profile convergence; never feed the obligation back into the
		// Definition compiler as self-attestation.
		expectedBase, compileErr := compileRuntimeSemanticDefinitionProofObligation(manifest.DefinitionRevision, item, source)
		if compileErr != nil {
			return fmt.Errorf("runtime semantic manifest repair Definition proof obligation coverage %q: %w", actual.CoverageWorkItemID, compileErr)
		}
		actualBase := actual.Clone()
		actualBase.OwnerFactAcquisition = nil
		actualBase.ValidationFailureRepair = nil
		if !sameRuntimeSemanticProofObligations([]RuntimeSemanticProofObligation{actualBase}, []RuntimeSemanticProofObligation{expectedBase}) {
			return fmt.Errorf("runtime semantic manifest repair terminal proof obligation coverage %q does not match immutable Definition focus", actual.CoverageWorkItemID)
		}
		repair := actual.ValidationFailureRepair.Normalized()
		if source.ValidationFailureRepairConvergence == nil || repair.FailedFocusID != source.ID {
			return fmt.Errorf("runtime semantic manifest repair coverage %q has no matching immutable Definition convergence", actual.CoverageWorkItemID)
		}
		convergence := source.ValidationFailureRepairConvergence.Clone()
		if repair.ConvergenceID != convergence.ID || repair.ConvergenceHash != StableRuntimeValidationFailureRepairConvergenceHash(convergence) {
			return fmt.Errorf("runtime semantic manifest repair coverage %q convergence does not match immutable Definition focus", actual.CoverageWorkItemID)
		}
		route, found := convergence.RouteForTrigger(repair.TriggerOwnerModuleID, repair.TriggerOperationKind, repair.TriggerDiagnosticClass, repair.TriggerEvidenceTargetType, repair.TriggerOwnerDiagnosticCode)
		if !found || route.ID != repair.RouteID || StableRuntimeValidationFailureRepairRouteHash(route) != repair.RouteHash {
			return fmt.Errorf("runtime semantic manifest repair coverage %q route does not match immutable Definition focus", actual.CoverageWorkItemID)
		}
	}
	return nil
}
