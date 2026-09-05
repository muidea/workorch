package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func runtimeExactProofCloneConstraintFixture(recipeID string) RuntimeProspectiveTargetSatisfactionConstraint {
	return RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion:                      " " + RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent + " ",
		OwnerModuleID:                      " " + RuntimeModuleShell + " ",
		ProspectiveSatisfactionContractRef: " shell/prospective-validation/v1 ",
		ValidationSurfaceID:                " " + recipeID + " ",
		ValidationRecipeID:                 " " + recipeID + " ",
		InputSchemaHash:                    " schema/go-test ",
		CatalogObservedRevision:            " shell-policy-v1 ",
		AllOf: []RuntimeProspectiveTargetClause{
			{
				ID: " package-tests ",
				AllOf: []RuntimeProspectiveTargetPredicate{
					{Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: " _test.go "},
					{Kind: RuntimeProspectiveTargetPredicatePathPrefix, Value: ` internal\pkg `},
				},
			},
			{
				ID: " root-module ",
				AllOf: []RuntimeProspectiveTargetPredicate{
					{Kind: RuntimeProspectiveTargetPredicatePathExact, Value: " ./go.mod "},
				},
			},
		},
	}
}

func runtimeExactProofCloneBindingFixture(recipeID string) RuntimeValidationProofRequirementBinding {
	return RuntimeValidationProofRequirementBinding{
		RequirementHash:    " requirement-hash-1 ",
		DefinitionRevision: " definition-revision-1 ",
		CoverageWorkItemID: " cwi-1 ",
		DeclarationRef:     " definition/dodItem/dod-1 ",
		ValidationRecipeID: " " + recipeID + " ",
		RequiredCheckRefs:  []string{" check/unit ", " check/module "},
		CheckAttestations: []RuntimeValidationCheckAttestationBinding{
			{
				RequiredCheckRef:       " check/unit ",
				ObservationProtocolRef: " shell/managed-validation/v1 ",
				NativeCheckRef:         " go-test/unit ",
			},
			{
				RequiredCheckRef:       " check/module ",
				ObservationProtocolRef: " shell/managed-validation/v1 ",
				NativeCheckRef:         " go-test/module ",
			},
		},
	}
}

func runtimeExactProofClonePreparationFixture(recipeID string) RuntimeValidationProofPreparation {
	return RuntimeValidationProofPreparation{
		RequirementHash:    " requirement-hash-1 ",
		DefinitionRevision: " definition-revision-1 ",
		CoverageWorkItemID: " cwi-1 ",
		DeclarationRef:     " definition/dodItem/dod-1 ",
		ValidationRecipeID: " " + recipeID + " ",
		TargetRef:          " go.mod ",
		CheckAttestations: []RuntimeValidationCheckAttestationBinding{
			{
				RequiredCheckRef:       " check/unit ",
				ObservationProtocolRef: " shell/managed-validation/v1 ",
				NativeCheckRef:         " go-test/unit ",
			},
			{
				RequiredCheckRef:       " check/module ",
				ObservationProtocolRef: " shell/managed-validation/v1 ",
				NativeCheckRef:         " go-test/module ",
			},
		},
		ProspectiveTargetSatisfactionConstraint: runtimeExactProofCloneConstraintFixture(recipeID),
	}
}

func runtimeExactProofCloneReadySurfaceFixture(recipeID string) RuntimeValidationSurface {
	return RuntimeValidationSurface{
		ID: " " + recipeID + " ", OwnerModuleID: " " + RuntimeModuleShell + " ", OperationKind: " " + RuntimeBlockKindManagedValidationSession + " ",
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: " schema/go-test ", InputBindingRef: " input/go-test ",
		CheckRefs: []string{" check/unit ", " check/module "}, ProofApplicability: RuntimeValidationProofDirect,
		ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{runtimeExactProofCloneBindingFixture(recipeID)},
		EffectScope:              RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
}

func runtimeExactProofClonePreparingSurfaceFixture(recipeID string) RuntimeValidationSurface {
	constraint := runtimeExactProofCloneConstraintFixture(recipeID)
	return RuntimeValidationSurface{
		ID: " " + recipeID + " ", OwnerModuleID: " " + RuntimeModuleShell + " ", OperationKind: " " + RuntimeBlockKindManagedValidationSession + " ",
		Readiness: RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: " owner input required ",
		InputSchemaHash: " schema/go-test ", PrerequisiteRef: " workspace/validation-input ",
		ProspectiveSatisfactionContractRef: " shell/prospective-validation/v1 ", ProspectiveTargetSatisfactionConstraint: &constraint,
		ProofApplicability:           RuntimeValidationProofDirect,
		ProofRequirementPreparations: []RuntimeValidationProofPreparation{runtimeExactProofClonePreparationFixture(recipeID)},
		EffectScope:                  RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
}

func TestRuntimeExactProofSinglePassClonesMatchNormalizedAndDetachNestedStorage(t *testing.T) {
	const recipeID = "recipe/go-test"

	t.Run("clause", func(t *testing.T) {
		original := runtimeExactProofCloneConstraintFixture(recipeID).AllOf[0]
		wantOriginal := runtimeExactProofCloneConstraintFixture(recipeID).AllOf[0]
		if err := original.Validate(); err != nil {
			t.Fatalf("valid clause fixture: %v", err)
		}
		clone := original.Clone()
		if normalized := original.Normalized(); !reflect.DeepEqual(clone, normalized) {
			t.Fatalf("Clone() = %+v, want Normalized() %+v", clone, normalized)
		}

		clone.AllOf[0].Value = "changed-target"
		clone.AllOf = append(clone.AllOf, RuntimeProspectiveTargetPredicate{Kind: RuntimeProspectiveTargetPredicatePathExact, Value: "changed.go"})
		if !reflect.DeepEqual(original, wantOriginal) {
			t.Fatalf("clause clone aliases original predicates: original=%+v clone=%+v", original, clone)
		}
	})

	t.Run("constraint", func(t *testing.T) {
		original := runtimeExactProofCloneConstraintFixture(recipeID)
		wantOriginal := runtimeExactProofCloneConstraintFixture(recipeID)
		if err := original.Validate(); err != nil {
			t.Fatalf("valid constraint fixture: %v", err)
		}
		clone := original.Clone()
		if normalized := original.Normalized(); !reflect.DeepEqual(clone, normalized) {
			t.Fatalf("Clone() = %+v, want Normalized() %+v", clone, normalized)
		}

		clone.AllOf[0].ID = "changed-clause"
		clone.AllOf[0].AllOf[0].Value = "changed-target"
		clone.AllOf = append(clone.AllOf, RuntimeProspectiveTargetClause{ID: "changed", AllOf: []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathExact, Value: "changed.go"}}})
		if !reflect.DeepEqual(original, wantOriginal) {
			t.Fatalf("constraint clone aliases original clauses or targets: original=%+v clone=%+v", original, clone)
		}
	})

	t.Run("binding", func(t *testing.T) {
		original := runtimeExactProofCloneBindingFixture(recipeID)
		wantOriginal := runtimeExactProofCloneBindingFixture(recipeID)
		if err := original.Validate(); err != nil {
			t.Fatalf("valid binding fixture: %v", err)
		}
		clone := original.Clone()
		if normalized := original.Normalized(); !reflect.DeepEqual(clone, normalized) {
			t.Fatalf("Clone() = %+v, want Normalized() %+v", clone, normalized)
		}

		clone.RequiredCheckRefs[0] = "changed-check"
		clone.CheckAttestations[0].NativeCheckRef = "changed-native-check"
		clone.CheckAttestations = append(clone.CheckAttestations, RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "changed", ObservationProtocolRef: "changed", NativeCheckRef: "changed"})
		if !reflect.DeepEqual(original, wantOriginal) {
			t.Fatalf("binding clone aliases original checks or attestations: original=%+v clone=%+v", original, clone)
		}
	})

	t.Run("preparation", func(t *testing.T) {
		original := runtimeExactProofClonePreparationFixture(recipeID)
		wantOriginal := runtimeExactProofClonePreparationFixture(recipeID)
		if err := original.Validate(); err != nil {
			t.Fatalf("valid preparation fixture: %v", err)
		}
		clone := original.Clone()
		if normalized := original.Normalized(); !reflect.DeepEqual(clone, normalized) {
			t.Fatalf("Clone() = %+v, want Normalized() %+v", clone, normalized)
		}

		clone.TargetRef = "changed.go"
		clone.CheckAttestations[0].NativeCheckRef = "changed-native-check"
		clone.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed-target"
		if !reflect.DeepEqual(original, wantOriginal) {
			t.Fatalf("preparation clone aliases original attestations, clauses or targets: original=%+v clone=%+v", original, clone)
		}
	})

	t.Run("surface", func(t *testing.T) {
		readyOriginal := runtimeExactProofCloneReadySurfaceFixture(recipeID)
		readyWantOriginal := runtimeExactProofCloneReadySurfaceFixture(recipeID)
		if err := readyOriginal.Validate(); err != nil {
			t.Fatalf("valid ready surface fixture: %v", err)
		}
		readyClone := readyOriginal.Clone()
		if normalized := readyOriginal.Normalized(); !reflect.DeepEqual(readyClone, normalized) {
			t.Fatalf("ready Clone() = %+v, want Normalized() %+v", readyClone, normalized)
		}
		readyClone.CheckRefs[0] = "changed-check"
		readyClone.ProofRequirementBindings[0].RequiredCheckRefs[0] = "changed-binding-check"
		readyClone.ProofRequirementBindings[0].CheckAttestations[0].NativeCheckRef = "changed-native-check"
		if !reflect.DeepEqual(readyOriginal, readyWantOriginal) {
			t.Fatalf("surface clone aliases original binding storage: original=%+v clone=%+v", readyOriginal, readyClone)
		}

		preparingOriginal := runtimeExactProofClonePreparingSurfaceFixture(recipeID)
		preparingWantOriginal := runtimeExactProofClonePreparingSurfaceFixture(recipeID)
		if err := preparingOriginal.Validate(); err != nil {
			t.Fatalf("valid preparing surface fixture: %v", err)
		}
		preparingClone := preparingOriginal.Clone()
		if normalized := preparingOriginal.Normalized(); !reflect.DeepEqual(preparingClone, normalized) {
			t.Fatalf("preparing Clone() = %+v, want Normalized() %+v", preparingClone, normalized)
		}
		preparingClone.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed-surface-target"
		preparingClone.ProofRequirementPreparations[0].CheckAttestations[0].NativeCheckRef = "changed-preparation-attestation"
		preparingClone.ProofRequirementPreparations[0].ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed-preparation-target"
		if !reflect.DeepEqual(preparingOriginal, preparingWantOriginal) {
			t.Fatalf("surface clone aliases original preparations, clauses or targets: original=%+v clone=%+v", preparingOriginal, preparingClone)
		}
	})
}
