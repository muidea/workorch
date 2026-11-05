package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSemanticInputReviewSchemaUsesExactRequestMembers(t *testing.T) {
	m, document, _, binding := semanticInputReviewFixture(t)
	m.CoverageItems = append(m.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "another original obligation", Order: 2})
	var otherHashes []string
	for i := range m.OwnerCatalogs {
		if m.OwnerCatalogs[i].OwnerModuleID != RuntimeModuleShell {
			continue
		}
		for _, id := range []common.CoverageWorkItemID{"cwi-002", "historical-cwi"} {
			other := binding.Clone()
			requirement := runtimeValidationProofRequirementForTest(t, m.DefinitionRevision, RuntimeCoverageItem{ID: id}, "", binding.ValidationRecipeID, binding.RequiredCheckRefs)
			other.RequirementHash = requirement.Hash
			other.CoverageWorkItemID = id
			other.DeclarationRef = requirement.DeclarationRef
			for j := range other.CheckAttestations {
				other.CheckAttestations[j].NativeCheckRef = string(id) + "/" + other.CheckAttestations[j].NativeCheckRef
			}
			other.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*other.InputReview, other)
			otherHashes = append(otherHashes, other.InputReview.Hash)
			m.OwnerCatalogs[i].ValidationSurfaces[0].ProofRequirementBindings = append(m.OwnerCatalogs[i].ValidationSurfaces[0].ProofRequirementBindings, other)
		}
	}
	m = sealSemanticTestManifest(t, m)
	document.ManifestHash = m.Hash
	before, _ := json.Marshal(m)
	raw, err := RuntimeSemanticPlanJSONSchemaForRequest(m, document.PlanStep, document.BatchID, document.BatchSequence, document.BatchCoverageWorkItemIDs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, binding.InputReview.Hash) {
		t.Fatal("current request lost its complete original review rule")
	}
	for _, hash := range otherHashes {
		if strings.Contains(raw, hash) {
			t.Fatal("request schema expanded another current or historical CWI review")
		}
	}
	full, err := RuntimeSemanticPlanJSONSchema(m)
	if err != nil || !strings.Contains(full, otherHashes[0]) || strings.Contains(full, otherHashes[1]) {
		t.Fatal("full schema does not match complete current manifest membership", err)
	}
	otherRequest, err := RuntimeSemanticPlanJSONSchemaForRequest(m, document.PlanStep, "other-batch", 1, []common.CoverageWorkItemID{"cwi-002"})
	if err != nil || !strings.Contains(otherRequest, otherHashes[0]) || strings.Contains(otherRequest, binding.InputReview.Hash) || strings.Contains(otherRequest, otherHashes[1]) {
		t.Fatal("next request did not select its own complete original review", err)
	}
	order, err := RuntimeSemanticPlanJSONSchemaForRequest(m, RuntimeSemanticPlanStepCoverageOrder, "", 0, nil)
	if err != nil || strings.Contains(order, binding.InputReview.Hash) || strings.Contains(order, otherHashes[0]) || strings.Contains(order, otherHashes[1]) {
		t.Fatal("order request expanded inapplicable review output rules", err)
	}
	if _, err := RuntimeSemanticPlanJSONSchemaForRequest(m, document.PlanStep, "foreign-batch", 1, []common.CoverageWorkItemID{"historical-cwi"}); err == nil {
		t.Fatal("historical catalog membership authorized a foreign batch")
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://workorch.invalid/input-review-scope.json", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("https://workorch.invalid/input-review-scope.json")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, document)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(wire)))
	if err != nil || schema.Validate(value) != nil {
		t.Fatal("current complete review changed acceptance", err, schema.Validate(value))
	}
	for _, mutation := range []string{"missing review", "wrong subject", "missing checks", "foreign coverage"} {
		t.Run(mutation, func(t *testing.T) {
			var bad map[string]any
			if err := json.Unmarshal(wire, &bad); err != nil {
				t.Fatal(err)
			}
			var p map[string]any
			for _, item := range bad["proposals"].([]any) {
				candidate := item.(map[string]any)
				if candidate["operationKind"] == RuntimeBlockKindManagedValidationSession {
					p = candidate
				}
			}
			if p == nil || p["validationInputReview"] == nil {
				t.Fatal("fixture has no validation proposal with its complete review")
			}
			switch mutation {
			case "missing review":
				delete(p, "validationInputReview")
			case "wrong subject":
				p["validationInputReview"].(map[string]any)["subjectHash"] = otherHashes[0]
			case "missing checks":
				p["validationInputReview"].(map[string]any)["checks"] = []any{}
			case "foreign coverage":
				p["coverageWorkItemId"] = "cwi-002"
			}
			if schema.Validate(bad) == nil {
				t.Fatal("selected request lost its current mandatory rule")
			}
		})
	}
	var restored RuntimeSemanticPlanManifest
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	reloaded, err := RuntimeSemanticPlanJSONSchemaForRequest(restored, document.PlanStep, document.BatchID, document.BatchSequence, document.BatchCoverageWorkItemIDs)
	after, _ := json.Marshal(m)
	if err != nil || raw != reloaded || !reflect.DeepEqual(before, after) {
		t.Fatal("schema projection mutated its source or changed across reload", err)
	}
}
