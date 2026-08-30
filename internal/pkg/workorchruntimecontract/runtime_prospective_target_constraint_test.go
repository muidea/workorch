package workorchruntimecontract

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeProspectiveTargetSatisfactionConstraintMatchesBoundedAllOfTargetSet(t *testing.T) {
	constraint := RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion:                      RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID:                      RuntimeModuleShell,
		ProspectiveSatisfactionContractRef: "shell/prospective-validation/v1",
		ValidationSurfaceID:                "recipe/go-test",
		ValidationRecipeID:                 "recipe/go-test",
		InputSchemaHash:                    "schema/go-test",
		CatalogObservedRevision:            "shell-policy-v1",
		AllOf: []RuntimeProspectiveTargetClause{
			{ID: "root-module", AllOf: []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathExact, Value: "go.mod"}}},
			{ID: "package-test", AllOf: []RuntimeProspectiveTargetPredicate{
				{Kind: RuntimeProspectiveTargetPredicatePathPrefix, Value: "internal/pkg"},
				{Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go"},
				{Kind: RuntimeProspectiveTargetPredicatePathSegmentNotEqual, Value: "vendor"},
			}},
		},
	}
	if err := constraint.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, targets := range [][]string{
		{"go.mod"},
		{"internal/pkg/main_test.go"},
		{"nested/go.mod", "internal/pkg/main_test.go"},
		{"go.mod", "internal/pkg/vendor/main_test.go"},
		{"go.mod", "internal/pkg/main_test.go", "README.md"},
		{"go.mod", "internal/pkg/../main_test.go"},
		{"go.mod", `internal\pkg\main_test.go`},
		{"go.mod", "/internal/pkg/main_test.go"},
	} {
		if RuntimeProspectiveTargetsSatisfyConstraint(targets, constraint) {
			t.Fatalf("targets %q satisfied only a subset, added an unrelated target or escaped the workspace-relative all-of contract", targets)
		}
	}
	if !RuntimeProspectiveTargetsSatisfyConstraint([]string{"go.mod", "internal/pkg/main_test.go"}, constraint) {
		t.Fatal("target set covering every all-of target clause was rejected")
	}
	if RuntimeProspectiveTargetSatisfiesConstraint("nested/go.mod", RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion: RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID: RuntimeModuleShell, ProspectiveSatisfactionContractRef: "shell/prospective-validation/v1",
		ValidationSurfaceID: "recipe/go-test", ValidationRecipeID: "recipe/go-test", InputSchemaHash: "schema/go-test",
		CatalogObservedRevision: "shell-policy-v1",
		AllOf:                   []RuntimeProspectiveTargetClause{{ID: "root-module", AllOf: []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathExact, Value: "go.mod"}}}},
	}) {
		t.Fatal("path_exact accepted a nested module")
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
			AllOf: []RuntimeProspectiveTargetClause{{ID: "required-input", AllOf: []RuntimeProspectiveTargetPredicate{{
				Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: suffix,
			}}}},
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
	if err != nil || len(resolved.AllOf) != 1 || resolved.AllOf[0].ID != "required-input" || resolved.AllOf[0].AllOf[0].Value != "_test.go" {
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

func TestRuntimeProspectiveTargetSatisfactionConstraintEnforcesSharedBoundAndRelevantTargets(t *testing.T) {
	constraint := RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion: RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID: RuntimeModuleShell, ProspectiveSatisfactionContractRef: "shell/prospective-validation/v1",
		ValidationSurfaceID: "recipe/go-test", ValidationRecipeID: "recipe/go-test", InputSchemaHash: "schema/go-test",
		CatalogObservedRevision: "shell-policy-v1",
		AllOf: []RuntimeProspectiveTargetClause{{ID: "test-input", AllOf: []RuntimeProspectiveTargetPredicate{{
			Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go",
		}}}},
	}
	if !RuntimeProspectiveTargetsSatisfyConstraint([]string{"runtime_test.go"}, constraint) {
		t.Fatal("ordinary length-one target-set semantics were rejected")
	}
	if RuntimeProspectiveTargetsSatisfyConstraint([]string{"runtime_test.go", "README.md"}, constraint) {
		t.Fatal("unrelated prospective target was admitted alongside a satisfying target")
	}
	if RuntimeProspectiveTargetsSatisfyConstraint([]string{"runtime_test.go", "runtime_test.go"}, constraint) {
		t.Fatal("duplicate prospective target was normalized into a valid target set")
	}
	overBound := make([]string, 0, RuntimeProspectiveTargetRefsMaxCount+1)
	for index := 0; index <= RuntimeProspectiveTargetRefsMaxCount; index++ {
		overBound = append(overBound, fmt.Sprintf("runtime_%d_test.go", index))
	}
	if RuntimeProspectiveTargetsSatisfyConstraint(overBound, constraint) {
		t.Fatal("prospective target set exceeded the shared bound")
	}
}

func TestRuntimeProspectiveTargetAllOfDeclarationIsCanonicalBoundedAndExact(t *testing.T) {
	declaration := NormalizeRuntimeProspectiveTargetAllOfConstraintRefs([]string{
		" new-target-requires-owner-selector ",
		"workspace-relative-path",
		"workspace-relative-path",
	})
	want := []string{"new-target-requires-owner-selector", "workspace-relative-path"}
	if !SameRuntimeProspectiveTargetAllOfConstraintRefs(declaration, want) {
		t.Fatalf("normalized all-of declaration = %#v, want %#v", declaration, want)
	}
	if SameRuntimeProspectiveTargetAllOfConstraintRefs(declaration, []string{"workspace-relative-path"}) {
		t.Fatal("all-of declaration accepted a mandatory-constraint subset")
	}
	if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(declaration); err != nil {
		t.Fatalf("valid all-of declaration rejected: %v", err)
	}
	overBound := make([]string, 0, RuntimeProspectiveTargetAllOfConstraintRefsMaxCount+1)
	for index := 0; index <= RuntimeProspectiveTargetAllOfConstraintRefsMaxCount; index++ {
		overBound = append(overBound, fmt.Sprintf("constraint-%02d", index))
	}
	if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(overBound); err == nil || !strings.Contains(err.Error(), "all-of declaration exceeds") {
		t.Fatalf("unbounded all-of declaration error = %v", err)
	}
}
