package specifications

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestCurrentDocumentRequiresExactClosedPresenceAndTypes(t *testing.T) {
	valid := []byte(`{"specification":{"Version":1,"GoalItemRef":"goal","GoalTextHash":"hash","Resources":[],"Dimensions":[{"ID":"d","Required":true,"Activated":false,"ResourceIDs":[]}],"Requirements":[],"Obligations":[],"Verifications":[],"Reviews":[]}}`)
	if err := validateCurrentDocument(valid, proposalShape); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string][2]string{
		"root-case":        {`"specification"`, `"Specification"`},
		"nested-case":      {`"Activated"`, `"activated"`},
		"missing-bool":     {`,"Activated":false`, ``},
		"null-bool":        {`"Activated":false`, `"Activated":null`},
		"null-array":       {`"ResourceIDs":[]`, `"ResourceIDs":null`},
		"null-string":      {`"ID":"d"`, `"ID":null`},
		"null-object":      {`{"ID":"d","Required":true,"Activated":false,"ResourceIDs":[]}`, `null`},
		"integer-fraction": {`"Version":1`, `"Version":1.0`},
		"integer-exponent": {`"Version":1`, `"Version":1e0`},
		"integer-string":   {`"Version":1`, `"Version":"1"`},
		"integer-overflow": {`"Version":1`, `"Version":4294967296`},
		"bool-string":      {`"Required":true`, `"Required":"true"`},
		"unknown":          {`"Activated":false`, `"Activated":false,"Authority":true`},
		"duplicate":        {`"Activated":false`, `"Activated":false,"Activated":true`},
		"case-duplicate":   {`"Activated":false`, `"Activated":false,"activated":true`},
	} {
		t.Run(name, func(t *testing.T) {
			payload := bytes.Replace(valid, []byte(change[0]), []byte(change[1]), 1)
			before := bytes.Clone(payload)
			if err := validateCurrentDocument(payload, proposalShape); err == nil {
				t.Fatal("非法当前版文档被接受")
			}
			if !bytes.Equal(before, payload) {
				t.Fatal("校验修改了原始诊断 bytes")
			}
		})
	}
}

func TestCurrentDocumentRejectsLossyUnicodeEscapes(t *testing.T) {
	for _, text := range []string{`\uD800`, `\udfff`, `\uD800x`, `\uD800A`, `\uD800\uD800`, `\uD800\\uDC00`} {
		if err := validateDocumentShape([]byte(`{"text":"` + text + `"}`)); err == nil {
			t.Fatalf("孤立 surrogate 被接受: %s", text)
		}
	}
	for _, text := range []string{`😀`, `\u` + `D83D\u` + `DE00`, `\u0000`, `\\uD800`, `\"`, `�`} {
		if err := validateDocumentShape([]byte(`{"text":"` + text + `"}`)); err != nil {
			t.Fatalf("合法无损字符串被拒绝: %s: %v", text, err)
		}
	}
}

func TestCurrentReviewPresenceDiagnosticIsDeterministic(t *testing.T) {
	shape := objectShape(map[string]documentShape{"review": reviewShape})
	for range 20 {
		err := validateCurrentDocument([]byte(`{"review":{"Kind":"regression"}}`), shape)
		if err == nil || err.Error() != "$.review.EvidenceRefs: required_field_missing" {
			t.Fatalf("缺字段定位不稳定: %v", err)
		}
	}
	if err := validateCurrentDocument([]byte(`{"review":{"Kind":"regression","Status":"PASS","SpecificationHash":"hash","Reason":"依据","EvidenceRefs":[],"RequirementIDs":[null]}}`), shape); err == nil || !strings.Contains(err.Error(), "$.review.RequirementIDs[0]: null_not_allowed") {
		t.Fatalf("数组元素未精确定位: %v", err)
	}
}

func TestCurrentShapeFieldsMatchProviderSchema(t *testing.T) {
	for _, test := range []struct {
		schema string
		shape  documentShape
	}{
		{derivationOutputSchema, derivationShape},
		{specificationOutputSchema("proposal"), proposalShape},
		{specificationOutputSchema("review/regression"), objectShape(map[string]documentShape{"review": reviewShape})},
	} {
		var schema map[string]any
		if err := json.Unmarshal([]byte(test.schema), &schema); err != nil {
			t.Fatal(err)
		}
		checkCurrentShapeSchema(t, test.shape, schema, "$")
	}
}

func checkCurrentShapeSchema(t *testing.T, shape documentShape, schema map[string]any, path string) {
	t.Helper()
	kind := map[byte]string{'o': "object", 'a': "array", 's': "string", 'b': "boolean", 'u': "integer"}[shape.kind]
	if schema["type"] != kind {
		t.Fatalf("%s shape/schema 类型漂移", path)
	}
	if shape.kind == 'a' {
		checkCurrentShapeSchema(t, *shape.item, schema["items"].(map[string]any), path+"[]")
	}
	if shape.kind == 'o' {
		properties := schema["properties"].(map[string]any)
		required := schema["required"].([]any)
		count := 0
		for _, child := range shape.fields {
			if !child.optional {
				count++
			}
		}
		if len(properties) != len(shape.fields) || len(required) != count || schema["additionalProperties"] != false {
			t.Fatalf("%s shape/schema 字段数量漂移", path)
		}
		for name, child := range shape.fields {
			property, ok := properties[name].(map[string]any)
			if !ok {
				t.Fatalf("%s.%s schema 缺字段", path, name)
			}
			found := false
			for _, value := range required {
				found = found || value == name
			}
			if found == child.optional {
				t.Fatalf("%s.%s schema 未要求字段", path, name)
			}
			checkCurrentShapeSchema(t, child, property, path+"."+name)
		}
	}
}

func TestDerivedAcceptanceCurrentShapeRejectsCaseAndSurrogate(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "template"}
	template := workspace.WorkspaceTemplate{ID: "template", DoDItems: []string{"验收"}}
	valid, err := json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(string(valid), "AuthoringInputHash", "authoringinputhash", 1),
		strings.Replace(string(valid), `"AdditionalItems":[]`, `"AdditionalItems":[null]`, 1),
		strings.Replace(string(valid), `"AdditionalItems":[]`, `"AdditionalItems":["\uD800"]`, 1),
	} {
		if _, err := ParseDerivedAcceptance([]byte(invalid), input, &template); err == nil {
			t.Fatal("非法派生文档被接受")
		}
	}
}
