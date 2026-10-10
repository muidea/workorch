package workorchruntimecontract

import "testing"

// 保留原 classifier 独立 reference；人工值覆盖布尔语义，不作为合法 owner producer。
func assessmentOwnerFactSatisfiedReference(requirement RuntimeOwnerFactRequirement, catalog RuntimeOperationAdmissibility) bool {
	if requirement.FactKind != RuntimeOwnerFactSelectableValidationSurface {
		return false
	}
	for _, surface := range catalog.ValidationSurfaces {
		if surface.OperationKind != requirement.OperationKind || surface.Readiness != RuntimeValidationSurfaceReady || surface.ProofApplicability != RuntimeValidationProofDirect {
			continue
		}
		published := false
		for _, ref := range catalog.ValidationRecipeRefs {
			published = published || ref == surface.ID
		}
		if !published {
			continue
		}
		if requirement.ValidationProofRequirementHash == "" {
			return true
		}
		for _, binding := range surface.ProofRequirementBindings {
			if binding.RequirementHash == requirement.ValidationProofRequirementHash {
				return true
			}
		}
	}
	return false
}

func FuzzRuntimeOwnerFactObservationPreservesClassifier(f *testing.F) {
	f.Add(uint8(0), true, true, true, true, true)
	f.Add(uint8(1), false, false, false, false, false)
	f.Fuzz(func(t *testing.T, readiness uint8, direct, published, bound, required, second bool) {
		requirement := RuntimeOwnerFactRequirement{FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
		if required {
			requirement.ValidationProofRequirementHash = "requirement"
		}
		states := []RuntimeValidationSurfaceReadiness{RuntimeValidationSurfaceReady, RuntimeValidationSurfacePrerequisiteRequired, RuntimeValidationSurfaceUnavailable}
		surface := RuntimeValidationSurface{ID: "surface", OperationKind: requirement.OperationKind, Readiness: states[int(readiness)%len(states)], ProofApplicability: RuntimeValidationProofAuxiliary}
		if direct {
			surface.ProofApplicability = RuntimeValidationProofDirect
		}
		if bound {
			surface.ProofRequirementBindings = []RuntimeValidationProofRequirementBinding{{RequirementHash: "requirement"}}
		}
		catalog := RuntimeOperationAdmissibility{ValidationSurfaces: []RuntimeValidationSurface{surface}}
		if published {
			catalog.ValidationRecipeRefs = []string{surface.ID}
		}
		if second {
			catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, RuntimeValidationSurface{ID: "later", OperationKind: requirement.OperationKind, Readiness: RuntimeValidationSurfaceReady, ProofApplicability: RuntimeValidationProofDirect, ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{{RequirementHash: "requirement"}}})
			catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, "later")
		}
		_, actual := observeRuntimeOwnerFactRequirementCatalog(requirement, catalog)
		if actual != assessmentOwnerFactSatisfiedReference(requirement, catalog) {
			t.Fatal("shared observation changed original classifier")
		}
	})
}
