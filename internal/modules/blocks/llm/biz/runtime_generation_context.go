package llm

import (
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeValidationGenerationContextProjection struct {
	OwnerModuleID      string                                              `json:"ownerModuleId"`
	CatalogSnapshotID  string                                              `json:"catalogSnapshotId"`
	ValidationRecipeID string                                              `json:"validationRecipeId"`
	RequirementHashes  []string                                            `json:"requirementHashes"`
	Context            *runtimecontract.RuntimeValidationGenerationContext `json:"context"`
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
			for _, p := range surface.ProofRequirementPreparations {
				if !selected[p.CoverageWorkItemID] || (len(targets) != 0 && !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(targets, p.ProspectiveTargetSatisfactionConstraint)) {
					continue
				}
				hashes = append(hashes, p.RequirementHash)
			}
			if len(hashes) == 0 {
				continue
			}
			sort.Strings(hashes)
			result = append(result, runtimeValidationGenerationContextProjection{OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, ValidationRecipeID: surface.ID, RequirementHashes: hashes, Context: surface.GenerationContext.Clone()})
		}
	}
	return result
}
