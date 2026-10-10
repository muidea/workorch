package specifications

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestOutputSchemaClosedAndDTOAligned(t *testing.T) {
	for _, purpose := range []string{"proposal", "review/traceability"} {
		var root map[string]any
		if err := json.Unmarshal([]byte(specificationOutputSchema(purpose)), &root); err != nil {
			t.Fatal(err)
		}
		typ := reflect.TypeOf(workspace.GoalSpecification{})
		key := "specification"
		if purpose != "proposal" {
			typ = reflect.TypeOf(workspace.GoalSpecificationCompletenessReview{})
			key = "review"
		}
		checkSchemaDTO(t, root["properties"].(map[string]any)[key].(map[string]any), typ)
		if root["additionalProperties"] != false || len(root["properties"].(map[string]any)) != 1 {
			t.Fatal("wrapper is not closed")
		}
		if key == "specification" && root["properties"].(map[string]any)[key].(map[string]any)["properties"].(map[string]any)["Reviews"].(map[string]any)["maxItems"] != float64(0) {
			t.Fatal("proposal allows reviews")
		}
	}
}

func checkSchemaDTO(t *testing.T, schema map[string]any, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Struct:
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("%s is not closed", typ)
		}
		props := schema["properties"].(map[string]any)
		required := schema["required"].([]any)
		if len(props) != typ.NumField() || len(required) != typ.NumField() {
			t.Fatalf("%s field drift", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			child, ok := props[f.Name].(map[string]any)
			if !ok {
				t.Fatalf("missing %s.%s", typ, f.Name)
			}
			found := false
			for _, r := range required {
				found = found || r == f.Name
			}
			if !found {
				t.Fatal("field not required", f.Name)
			}
			checkSchemaDTO(t, child, f.Type)
		}
	case reflect.Slice:
		if schema["type"] != "array" {
			t.Fatal("slice is not array")
		}
		checkSchemaDTO(t, schema["items"].(map[string]any), typ.Elem())
	case reflect.String:
		if schema["type"] != "string" {
			t.Fatal("string drift")
		}
	case reflect.Bool:
		if schema["type"] != "boolean" {
			t.Fatal("boolean drift")
		}
	case reflect.Uint32:
		if schema["type"] != "integer" {
			t.Fatal("integer drift")
		}
	default:
		t.Fatal("uncovered DTO type", typ)
	}
}

// 测试专用遍历器只实现上述显式合同使用的小范围词汇，不替代生产 owner 校验。
func validateOutputShape(schema map[string]any, value any) error {
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("not object")
		}
		props := schema["properties"].(map[string]any)
		for key, v := range object {
			child, ok := props[key].(map[string]any)
			if !ok {
				return fmt.Errorf("unknown field %s", key)
			}
			if err := validateOutputShape(child, v); err != nil {
				return err
			}
		}
		for _, key := range schema["required"].([]any) {
			if _, ok := object[key.(string)]; !ok {
				return fmt.Errorf("missing field %s", key)
			}
		}
	case "array":
		values, ok := value.([]any)
		if !ok {
			return fmt.Errorf("not array")
		}
		if limit, ok := schema["maxItems"].(float64); ok && len(values) > int(limit) {
			return fmt.Errorf("too many items")
		}
		for _, v := range values {
			if err := validateOutputShape(schema["items"].(map[string]any), v); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("not string")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("not boolean")
		}
	case "integer":
		if n, ok := value.(float64); !ok || n != float64(int64(n)) {
			return fmt.Errorf("not integer")
		}
	}
	if enums, ok := schema["enum"].([]any); ok {
		found := false
		for _, v := range enums {
			found = found || reflect.DeepEqual(v, value)
		}
		if !found {
			return fmt.Errorf("invalid enum")
		}
	}
	return nil
}

func TestOutputSchemaRejectsInventedFieldsAndReviewConclusions(t *testing.T) {
	// 合成 DTO fixture，不使用 provider 或 live 数据。
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: "goal", GoalTextHash: "hash", Resources: []workspace.GoalSpecificationResource{{}}, Dimensions: []workspace.GoalSpecificationDimension{{ResourceIDs: []string{}}}, Requirements: []workspace.GoalSpecificationRequirement{{SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{}, ResourceIDs: []string{}}}, Obligations: []workspace.GoalSpecificationObligation{{DoDItemRefs: []string{}}}, Verifications: []workspace.GoalSpecificationVerification{{Status: workspace.GoalVerificationUnverified}}, Reviews: []workspace.GoalSpecificationCompletenessReview{}}
	raw, _ := json.Marshal(map[string]any{"specification": spec})
	var valid map[string]any
	_ = json.Unmarshal(raw, &valid)
	var schema map[string]any
	_ = json.Unmarshal([]byte(specificationOutputSchema("proposal")), &schema)
	if err := validateOutputShape(schema, valid); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ConditionMode", "Authority", "ProfileAssessment"} {
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		value["specification"].(map[string]any)["Requirements"].([]any)[0].(map[string]any)[field] = "invented"
		if validateOutputShape(schema, value) == nil {
			t.Fatal("accepted", field)
		}
	}
	for _, key := range []string{"Resources", "Dimensions", "Requirements", "Obligations", "Verifications"} {
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		value["specification"].(map[string]any)[key].([]any)[0].(map[string]any)["unknown"] = true
		if validateOutputShape(schema, value) == nil {
			t.Fatal("nested object open", key)
		}
	}
	for _, bad := range []any{nil, []any{map[string]any{"Status": "PASS"}}} {
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		value["specification"].(map[string]any)["Reviews"] = bad
		if validateOutputShape(schema, value) == nil {
			t.Fatal("accepted proposal review", bad)
		}
	}
	review := workspace.GoalSpecificationCompletenessReview{Kind: workspace.GoalReviewTraceability, Status: workspace.GoalReviewUnknown, EvidenceRefs: []string{}, RequirementIDs: []string{}}
	raw, _ = json.Marshal(map[string]any{"review": review})
	valid = nil
	schema = nil
	_ = json.Unmarshal(raw, &valid)
	_ = json.Unmarshal([]byte(specificationOutputSchema("review/traceability")), &schema)
	if err := validateOutputShape(schema, valid); err != nil {
		t.Fatal(err)
	}
	valid["review"].(map[string]any)["Status"] = "ACCEPTED"
	if validateOutputShape(schema, valid) == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestProposalStrictDecoderRejectsInventedNestedFields(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充验收"}}
	spec := authoringCommandFixture(t, input, template, derived)
	spec.Resources = []workspace.GoalSpecificationResource{{ID: "resource", Ref: "fixture", SnapshotHash: "hash", Available: true}}
	spec.Reviews = []workspace.GoalSpecificationCompletenessReview{}
	raw, _ := json.Marshal(map[string]any{"specification": spec})
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	goal := workspace.DefinitionCandidateItem{CandidateItemRef: spec.GoalItemRef, Text: input.GoalText}
	for _, layer := range []string{"specification", "Resources", "Dimensions", "Requirements", "Obligations", "Verifications"} {
		for _, field := range []string{"ConditionMode", "Authority", "ProfileAssessment", "Method"} {
			var value map[string]any
			_ = json.Unmarshal(raw, &value)
			object := value["specification"].(map[string]any)
			if layer != "specification" {
				object = object[layer].([]any)[0].(map[string]any)
			}
			object[field] = "invented"
			payload, _ := json.Marshal(value)
			if _, err := ParseProposal(payload, goal, items); err == nil {
				t.Fatalf("strict decoder accepted %s.%s", layer, field)
			}
		}
	}
}

func TestOutputSchemaValidProposalAlsoPassesOwnerContract(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充验收"}}
	spec := authoringCommandFixture(t, input, template, derived)
	spec.Resources = []workspace.GoalSpecificationResource{}
	spec.Reviews = []workspace.GoalSpecificationCompletenessReview{}
	for n := range spec.Dimensions {
		spec.Dimensions[n].ResourceIDs = []string{}
	}
	for n := range spec.Requirements {
		spec.Requirements[n].ResourceIDs = []string{}
	}
	raw, err := json.Marshal(map[string]any{"specification": spec})
	if err != nil {
		t.Fatal(err)
	}
	var value, schema map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(specificationOutputSchema("proposal")), &schema); err != nil {
		t.Fatal(err)
	}
	if err := validateOutputShape(schema, value); err != nil {
		t.Fatal(err)
	}
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProposal(raw, workspace.DefinitionCandidateItem{CandidateItemRef: spec.GoalItemRef, Text: input.GoalText}, items)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.ValidateAuthoringSpecification(input, &template, derived, nil, parsed); err != nil {
		t.Fatal(err)
	}
}

func TestOutputSchemaEnumsMatchDomainContract(t *testing.T) {
	var proposal, review map[string]any
	_ = json.Unmarshal([]byte(specificationOutputSchema("proposal")), &proposal)
	_ = json.Unmarshal([]byte(specificationOutputSchema("review/traceability")), &review)
	props := proposal["properties"].(map[string]any)["specification"].(map[string]any)["properties"].(map[string]any)
	getEnum := func(array, field string) []any {
		return props[array].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)[field].(map[string]any)["enum"].([]any)
	}
	if !reflect.DeepEqual(getEnum("Requirements", "SourceKind"), []any{workspace.GoalRequirementUserExplicit, workspace.GoalRequirementTemplateRequired, workspace.GoalRequirementTemplateConditional, workspace.GoalRequirementDerived, workspace.GoalRequirementDiscovered}) {
		t.Fatal("source enum drift")
	}
	if !reflect.DeepEqual(getEnum("Verifications", "Status"), []any{workspace.GoalVerificationDeclared, workspace.GoalVerificationUnverified}) {
		t.Fatal("verification enum drift")
	}
	reviewProps := review["properties"].(map[string]any)["review"].(map[string]any)["properties"].(map[string]any)
	if !reflect.DeepEqual(reviewProps["Status"].(map[string]any)["enum"], []any{workspace.GoalReviewPass, workspace.GoalReviewFail, workspace.GoalReviewUnknown}) {
		t.Fatal("review status drift")
	}
	kinds := reviewProps["Kind"].(map[string]any)["enum"].([]any)
	expected := ReviewKinds()
	if len(kinds) != len(expected) {
		t.Fatal("review kind count drift")
	}
	for i, v := range expected {
		if kinds[i] != v {
			t.Fatal("review kind drift")
		}
	}
}
