package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func profileIdentityFixture() []RuntimeProfileContract {
	return []RuntimeProfileContract{{
		ID:                      "profile",
		AcceptanceDimensions:    []RuntimeAcceptanceDimension{{ID: "behavior", Required: true}},
		DeliverableExpectations: []RuntimeDeliverableExpectation{{ID: "source", Required: true}},
		EvidenceRequirements: []RuntimeProfileEvidenceRequirement{
			{ID: "behavior", Scope: "acceptanceDimension", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}},
			{ID: "source", Scope: "deliverable", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}},
		},
		BootstrapRecipes: []RuntimeBootstrapRecipe{{ID: "bootstrap", RequiredGoalTerms: []string{"deliver"}, Artifacts: []RuntimeBootstrapArtifact{{ID: "source", TargetRole: "implementation", AllowedOperationKinds: []string{StepKindFileWrite}}}}},
	}}
}

func TestRuntimeProfileIdentityCannotEraseAcceptanceBeforeAdmission(t *testing.T) {
	base := profileIdentityFixture()
	if err := ValidateRuntimeProfileContracts(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]RuntimeProfileContract) []RuntimeProfileContract{
		"incomplete owner fact": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].EvidenceRequirements[0].RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: RuntimeModuleShell}}
			return p
		},
		"duplicate owner fact": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			f := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
			p[0].EvidenceRequirements[0].RequiredOwnerFacts = []RuntimeOwnerFactRequirement{f, f}
			p[0].EvidenceRequirements[0].OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: f, RequirePreexisting: true}}
			return p
		},
		"incomplete operation variant": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].EvidenceRequirements[1].AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: StepKindFileWrite}}
			return p
		},
		"duplicate operation variant": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			v := RuntimeOperationVariantRef{Kind: StepKindFileWrite, VariantID: "write"}
			p[0].EvidenceRequirements[1].AcceptedOperationVariants = []RuntimeOperationVariantRef{v, v}
			return p
		},
		"duplicate profile hides all required work": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			return append(p, RuntimeProfileContract{ID: " profile "})
		},
		"empty profile": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			return append(p, RuntimeProfileContract{ID: " "})
		},
		"duplicate acceptance weakens required": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].AcceptanceDimensions = append(p[0].AcceptanceDimensions, RuntimeAcceptanceDimension{ID: " behavior ", Required: false})
			return p
		},
		"empty acceptance": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].AcceptanceDimensions = append(p[0].AcceptanceDimensions, RuntimeAcceptanceDimension{Required: true})
			return p
		},
		"duplicate deliverable weakens required": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].DeliverableExpectations = append(p[0].DeliverableExpectations, RuntimeDeliverableExpectation{ID: "source", Required: false})
			return p
		},
		"empty deliverable": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].DeliverableExpectations = append(p[0].DeliverableExpectations, RuntimeDeliverableExpectation{Required: true})
			return p
		},
		"duplicate bootstrap recipe": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].BootstrapRecipes = append(p[0].BootstrapRecipes, p[0].BootstrapRecipes[0])
			return p
		},
		"empty bootstrap recipe": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].BootstrapRecipes = append(p[0].BootstrapRecipes, RuntimeBootstrapRecipe{})
			return p
		},
		"duplicate artifact hides operation": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			a := p[0].BootstrapRecipes[0].Artifacts[0]
			a.TargetRole = "different"
			p[0].BootstrapRecipes[0].Artifacts = append(p[0].BootstrapRecipes[0].Artifacts, a)
			return p
		},
		"empty artifact": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].BootstrapRecipes[0].Artifacts = append(p[0].BootstrapRecipes[0].Artifacts, RuntimeBootstrapArtifact{})
			return p
		},
		"incomplete capability state": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			p[0].EvidenceRequirements[1].CapabilityStateRequirements = []RuntimeProfileCapabilityStateRequirement{{OwnerModuleID: RuntimeModuleFile}}
			return p
		},
		"duplicate capability state": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			c := RuntimeProfileCapabilityStateRequirement{OwnerModuleID: RuntimeModuleFile, CapabilityStateKind: "manifest", SubjectRef: "delivery"}
			p[0].EvidenceRequirements[1].CapabilityStateRequirements = []RuntimeProfileCapabilityStateRequirement{c, c}
			return p
		},
		"duplicate dependency weakens admission": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			d := RuntimeCoverageDependency{ID: "source-first", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "acceptanceDimension", DependentRequirementID: "behavior", RequireSourceSettled: true}
			p[0].CoverageDependencies = []RuntimeCoverageDependency{d, d}
			p[0].CoverageDependencies[1].RequireSourceSettled = false
			p[0].CoverageDependencies[1].TriggerEvidenceKinds = []string{EvidenceKindStateChange}
			return p
		},
		"dependency identity reused for another edge": func(p []RuntimeProfileContract) []RuntimeProfileContract {
			d := RuntimeCoverageDependency{ID: "edge", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "acceptanceDimension", DependentRequirementID: "behavior", RequireSourceSettled: true}
			p[0].CoverageDependencies = []RuntimeCoverageDependency{d, d}
			p[0].CoverageDependencies[1].DependentRequirementID = "other"
			return p
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := mutate(CloneRuntimeProfileContracts(base))
			original := CloneRuntimeProfileContracts(invalid)
			for pass := 0; pass < 3; pass++ {
				if err := ValidateRuntimeProfileContracts(invalid); err == nil {
					t.Fatalf("pass %d: invalid declaration admitted", pass)
				}
				if pass == 0 && !reflect.DeepEqual(invalid, original) {
					t.Fatal("validation modified caller declarations")
				}
				normalized := NormalizeRuntimeProfileContracts(invalid)
				if SameRuntimeProfileContracts(normalized, base) {
					t.Fatal("invalid declaration disappeared from profile comparison")
				}
				if StableRuntimeProfileContractsHash(normalized) == StableRuntimeProfileContractsHash(base) {
					t.Fatal("invalid declaration disappeared from sealed identity")
				}
				data, err := json.Marshal(normalized)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &invalid); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRuntimeProfileIdentityComparisonRetainsDuplicateDeclarations(t *testing.T) {
	base := profileIdentityFixture()
	fact := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	base[0].EvidenceRequirements[0].RequiredOwnerFacts = []RuntimeOwnerFactRequirement{fact}
	base[0].EvidenceRequirements[0].OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: fact, RequirePreexisting: true}}
	variant := RuntimeOperationVariantRef{Kind: StepKindFileWrite, VariantID: "write"}
	base[0].EvidenceRequirements[1].AcceptedOperationVariants = []RuntimeOperationVariantRef{variant}
	if err := ValidateRuntimeProfileContracts(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]RuntimeProfileContract){
		"owner fact": func(p []RuntimeProfileContract) {
			p[0].EvidenceRequirements[0].RequiredOwnerFacts = append(p[0].EvidenceRequirements[0].RequiredOwnerFacts, fact)
		},
		"operation variant": func(p []RuntimeProfileContract) {
			p[0].EvidenceRequirements[1].AcceptedOperationVariants = append(p[0].EvidenceRequirements[1].AcceptedOperationVariants, variant)
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := CloneRuntimeProfileContracts(base)
			mutate(invalid)
			if err := ValidateRuntimeProfileContracts(invalid); err == nil {
				t.Fatal("duplicate declaration admitted")
			}
			if SameRuntimeProfileContracts(base, invalid) || SameRuntimeProfileContracts(invalid, base) {
				t.Fatal("profile comparison erased a duplicate declaration")
			}
			if StableRuntimeProfileContractsHash(base) == StableRuntimeProfileContractsHash(invalid) {
				t.Fatal("profile hash erased a duplicate declaration")
			}
		})
	}
}

func TestRuntimeProfileIdentityCanonicalizationIsDetachedAndIdempotent(t *testing.T) {
	profiles := profileIdentityFixture()
	profiles = append(profiles, RuntimeProfileContract{ID: " another "})
	profiles[0].AcceptanceDimensions[0].ID = " behavior "
	before := CloneRuntimeProfileContracts(profiles)
	normalized := NormalizeRuntimeProfileContracts(profiles)
	if err := ValidateRuntimeProfileContracts(normalized); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized, NormalizeRuntimeProfileContracts(normalized)) {
		t.Fatal("normalization is not idempotent")
	}
	if !reflect.DeepEqual(profiles, before) {
		t.Fatal("normalization mutated input")
	}
	reversed := []RuntimeProfileContract{profiles[1], profiles[0]}
	if !SameRuntimeProfileContracts(profiles, reversed) || StableRuntimeProfileContractsHash(profiles) != StableRuntimeProfileContractsHash(reversed) {
		t.Fatal("valid ordering changed identity")
	}
	normalized[1].BootstrapRecipes[0].Artifacts[0].AllowedOperationKinds[0] = "changed"
	if !reflect.DeepEqual(profiles, before) {
		t.Fatal("normalized policy aliases caller")
	}
}

func TestRuntimeProfileProofConstraintsEnterSealedIdentity(t *testing.T) {
	base := profileIdentityFixture()
	fact := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	base[0].EvidenceRequirements[0].RequiredOwnerFacts = []RuntimeOwnerFactRequirement{fact}
	base[0].EvidenceRequirements[0].OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: fact, RequirePreexisting: true}}
	if err := ValidateRuntimeProfileContracts(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeProfileEvidenceRequirement){
		"recipe":         func(r *RuntimeProfileEvidenceRequirement) { r.ValidationRecipeID = "recipe" },
		"required check": func(r *RuntimeProfileEvidenceRequirement) { r.RequiredCheckRefs = []string{"original-check"} },
		"metric":         func(r *RuntimeProfileEvidenceRequirement) { r.MetricConstraints = []string{"required-bound"} },
		"artifact":       func(r *RuntimeProfileEvidenceRequirement) { r.ArtifactRefs = []string{"required-artifact"} },
		"owner fact proof identity": func(r *RuntimeProfileEvidenceRequirement) {
			r.RequiredOwnerFacts[0].ValidationProofRequirementHash = RuntimeValidationProofRequirementHashPrefix + "bound-proof"
			r.OwnerFactConvergences[0].Requirement.ValidationProofRequirementHash = r.RequiredOwnerFacts[0].ValidationProofRequirementHash
		},
		"capability state": func(r *RuntimeProfileEvidenceRequirement) {
			r.CapabilityStateRequirements = []RuntimeProfileCapabilityStateRequirement{{OwnerModuleID: RuntimeModuleShell, CapabilityStateKind: "readiness", SubjectRef: "delivery"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := CloneRuntimeProfileContracts(base)
			mutate(&changed[0].EvidenceRequirements[0])
			if err := ValidateRuntimeProfileContracts(changed); err != nil {
				t.Fatal(err)
			}
			if SameRuntimeProfileContracts(base, changed) {
				t.Fatal("changed proof constraint compared equal")
			}
			if StableRuntimeProfileContractsHash(base) == StableRuntimeProfileContractsHash(changed) {
				t.Fatal("changed proof constraint has same admission hash")
			}
			if StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, RuntimeExecutionContract{ProfileContracts: base}) == StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, RuntimeExecutionContract{ProfileContracts: changed}) {
				t.Fatal("changed proof constraint has same request prefix")
			}
		})
	}
}
