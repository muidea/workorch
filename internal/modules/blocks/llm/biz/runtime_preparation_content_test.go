package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerPreparationContentReachesSemanticAndOperationC3Unchanged(t *testing.T) {
	requirement := runtimeValidationProofRequirementForLLMTest(t, "definition", "cwi-001", "dod-001", "original criterion", runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-001"), "recipe")
	constraint := runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: "owner/input-manifest/v1",
		ValidationSurfaceID: "recipe", ValidationRecipeID: "recipe", InputSchemaHash: "schema", CatalogObservedRevision: "policy",
		AllOf: []runtimecontract.RuntimeProspectiveTargetClause{{ID: "input", AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathExact, Value: "checks/input.spec"}}}},
	}
	p := runtimecontract.RuntimeValidationProofPreparation{
		RequirementHash: requirement.Hash, DefinitionRevision: requirement.DefinitionRevision, CoverageWorkItemID: requirement.CoverageWorkItemID, DeclarationRef: requirement.DeclarationRef,
		ValidationRecipeID: "recipe", TargetRef: "checks/input.spec", CheckAttestations: runtimeValidationCheckAttestationsForLLMTest(requirement.RequiredCheckRefs), ProspectiveTargetSatisfactionConstraint: constraint,
		ContentContractRef: "owner/exact-input/v1", ContentRequirements: "Keep the exact original criterion and use only the declared complete input scope.",
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	catalogs := []runtimecontract.RuntimeOperationAdmissibility{{PrerequisiteID: "catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell, ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ID: "recipe", ProofRequirementPreparations: []runtimecontract.RuntimeValidationProofPreparation{p}}}}}
	semantic, err := runtimeSemanticProspectiveTargetConstraintForProvider(&constraint, &p)
	if err != nil {
		t.Fatal(err)
	}
	projections := [][]runtimeSemanticNativeCheckAttestationProjection{
		semantic.ValidationProofPreparations,
		runtimeSemanticNativeCheckAttestationsForRequirement(catalogs, requirement),
		runtimeCandidateValidationProofPreparations(runtimecontract.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{requirement.CoverageWorkItemID}, ProspectiveTargetRefs: []string{p.TargetRef}}, catalogs),
	}
	for _, values := range projections {
		if len(values) != 1 || values[0].ContentContractRef != p.ContentContractRef || values[0].ContentRequirements != p.ContentRequirements || values[0].TargetRef != p.TargetRef {
			t.Fatalf("owner contract lost in C3: %+v", values)
		}
		encoded, _ := json.Marshal(values)
		if !strings.Contains(string(encoded), `"contentContractRef"`) || !strings.Contains(string(encoded), `"contentRequirements"`) {
			t.Fatal("provider-facing content contract omitted")
		}
	}
	legacy := p.Clone()
	legacy.ContentContractRef, legacy.ContentRequirements = "", ""
	old, err := runtimeSemanticProspectiveTargetConstraintForProvider(&constraint, &legacy)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(old)
	if strings.Contains(string(encoded), "contentRequirements") || strings.Contains(string(encoded), "contentContractRef") {
		t.Fatal("legacy request gained content authority")
	}
}
