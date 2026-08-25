package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestRuntimeProspectiveTargetSatisfactionConstraintMatchesAllOfOnly(t *testing.T) {
	constraint := RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion:                      RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID:                      RuntimeModuleShell,
		ProspectiveSatisfactionContractRef: "shell/prospective-validation/v1",
		ValidationSurfaceID:                "recipe/go-test",
		ValidationRecipeID:                 "recipe/go-test",
		InputSchemaHash:                    "schema/go-test",
		CatalogObservedRevision:            "shell-policy-v1",
		AllOf: []RuntimeProspectiveTargetPredicate{
			{Kind: RuntimeProspectiveTargetPredicatePathPrefix, Value: "internal/pkg"},
			{Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go"},
			{Kind: RuntimeProspectiveTargetPredicatePathSegmentNotEqual, Value: "vendor"},
		},
	}
	if err := constraint.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"main_test.go",
		"internal/pkg/main.go",
		"internal/pkg/vendor/main_test.go",
		"internal/pkg/../main_test.go",
		`internal\pkg\main_test.go`,
		"/internal/pkg/main_test.go",
	} {
		if RuntimeProspectiveTargetSatisfiesConstraint(target, constraint) {
			t.Fatalf("target %q satisfied only a subset or escaped the workspace-relative all-of contract", target)
		}
	}
	if !RuntimeProspectiveTargetSatisfiesConstraint("internal/pkg/main_test.go", constraint) {
		t.Fatal("target satisfying every all-of predicate was rejected")
	}
}

func TestResolveRuntimeProspectiveTargetSatisfactionConstraintRejectsImplicitAnyOf(t *testing.T) {
	const contractRef = "shell/prospective-validation/v1"
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell,
		FactKind:      RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	newSurface := func(id, suffix string) RuntimeValidationSurface {
		surface := RuntimeValidationSurface{
			ID: id, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "workspace input required",
			InputSchemaHash: "schema/" + id, PrerequisiteRef: "workspace/validation-input",
			ProspectiveSatisfactionContractRef: contractRef,
			CheckRefs:                          []string{"check/" + id}, ProofApplicability: RuntimeValidationProofDirect,
			EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		}
		constraint := RuntimeProspectiveTargetSatisfactionConstraint{
			SchemaVersion: RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
			OwnerModuleID: RuntimeModuleShell, ProspectiveSatisfactionContractRef: contractRef,
			ValidationSurfaceID: id, ValidationRecipeID: id, InputSchemaHash: surface.InputSchemaHash,
			CatalogObservedRevision: "shell-policy-v1",
			AllOf:                   []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: suffix}},
		}
		surface.ProspectiveTargetSatisfactionConstraint = &constraint
		return surface
	}
	newCatalog := func(surfaces ...RuntimeValidationSurface) RuntimeOperationAdmissibility {
		catalog := RuntimeOperationAdmissibility{
			PrerequisiteID: "shell-policy", OwnerModuleID: RuntimeModuleShell,
			OperationKinds:     []string{RuntimeBlockKindManagedValidationSession},
			ValidationSurfaces: surfaces, ObservedRevision: "shell-policy-v1",
		}
		catalog = catalog.Normalized()
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		return catalog
	}

	resolved, err := ResolveRuntimeProspectiveTargetSatisfactionConstraint(newCatalog(newSurface("recipe/go-test", "_test.go")), requirement, contractRef)
	if err != nil || len(resolved.AllOf) != 1 || resolved.AllOf[0].Value != "_test.go" {
		t.Fatalf("single all-of constraint was not resolved: constraint=%+v err=%v", resolved, err)
	}
	_, err = ResolveRuntimeProspectiveTargetSatisfactionConstraint(
		newCatalog(newSurface("recipe/go-test", "_test.go"), newSurface("recipe/spec", ".spec")),
		requirement,
		contractRef,
	)
	if err == nil || !strings.Contains(err.Error(), "requires exactly one all-of surface, got 2") {
		t.Fatalf("multiple prospective surfaces were accepted as an implicit any-of: %v", err)
	}
}
