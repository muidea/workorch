package llm

import (
	"slices"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RequiredCoverage is an exact inventory, not inferred semantic coverage.
type runtimeValidationGenerationCoverage struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	RequirementHash    string                    `json:"requirementHash"`
	RequiredCheckRefs  []string                  `json:"requiredCheckRefs"`
}

type runtimeValidationGenerationContextProjection struct {
	RequiredCoverage    []runtimeValidationGenerationCoverage               `json:"requiredCoverage"`
	CoverageInstruction string                                              `json:"coverageInstruction"`
	OwnerModuleID       string                                              `json:"ownerModuleId"`
	CatalogSnapshotID   string                                              `json:"catalogSnapshotId"`
	ValidationRecipeID  string                                              `json:"validationRecipeId"`
	RequirementHashes   []string                                            `json:"requirementHashes"`
	Context             *runtimecontract.RuntimeValidationGenerationContext `json:"context"`
}

func runtimePreparationSourceObligation(p runtimecontract.RuntimeValidationProofPreparation, check string) *runtimeSemanticSourceObligationCheckProjection {
	for _, source := range p.SourceObligationChecks {
		if source.CheckRef == check {
			values := runtimeSemanticSourceObligationChecksForProvider([]runtimecontract.RuntimeSourceObligationCheck{source})
			return &values[0]
		}
	}
	return nil
}

func runtimeValidationGenerationContexts(ids []common.CoverageWorkItemID, targets []string, catalogs []runtimecontract.RuntimeOperationAdmissibility) []runtimeValidationGenerationContextProjection {
	selected := make(map[common.CoverageWorkItemID]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	var result []runtimeValidationGenerationContextProjection
	for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(catalogs) {
		for _, surface := range catalog.ValidationSurfaces {
			if surface.GenerationContext == nil {
				continue
			}
			var hashes []string
			var coverage []runtimeValidationGenerationCoverage
			for _, p := range surface.ProofRequirementPreparations {
				if !selected[p.CoverageWorkItemID] || (len(targets) != 0 && !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(targets, p.ProspectiveTargetSatisfactionConstraint)) {
					continue
				}
				hashes = append(hashes, p.RequirementHash)
				checks := make([]string, 0, len(p.CheckAttestations))
				for _, attestation := range p.CheckAttestations {
					checks = append(checks, attestation.RequiredCheckRef)
				}
				coverage = append(coverage, runtimeValidationGenerationCoverage{CoverageWorkItemID: p.CoverageWorkItemID, RequirementHash: p.RequirementHash, RequiredCheckRefs: checks})
			}
			for _, binding := range surface.ProofRequirementBindings {
				if selected[binding.CoverageWorkItemID] {
					hashes = append(hashes, binding.RequirementHash)
					coverage = append(coverage, runtimeValidationGenerationCoverage{CoverageWorkItemID: binding.CoverageWorkItemID, RequirementHash: binding.RequirementHash, RequiredCheckRefs: slices.Clone(binding.RequiredCheckRefs)})
				}
			}
			if len(hashes) == 0 {
				continue
			}
			sort.Strings(hashes)
			hashes = slices.Compact(hashes)
			result = append(result, runtimeValidationGenerationContextProjection{OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, ValidationRecipeID: surface.ID, RequirementHashes: hashes, RequiredCoverage: coverage, CoverageInstruction: "Prepare actual assertions for every exact required check against its entire original C1 Goal/DoD criterion and source declaration, using the complete current owner corpus. Preserve all original conjuncts and existing compatible assertions. Source inventory, generic checks, compilation, self-claims and deferred receipt consumers do not establish behavioral coverage. If context or an assertion is missing, state that gap; do not claim complete coverage. Fresh independent input review and ordinary validation remain mandatory.", Context: surface.GenerationContext.Clone()})
		}
	}
	return result
}
