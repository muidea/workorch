package workspace

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestSealedMappingDecoderShapeMatchesProviderSchemaAtEveryField(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(DefinitionAuthoringMappingSchema), &schema); err != nil {
		t.Fatal(err)
	}
	var check func(mappingShape, map[string]any, string)
	check = func(shape mappingShape, schema map[string]any, path string) {
		t.Helper()
		switch shape.kind {
		case 'o':
			properties, ok := schema["properties"].(map[string]any)
			required, requiredOK := schema["required"].([]any)
			count := 0
			for _, child := range shape.fields {
				if !child.optional {
					count++
				}
			}
			if !ok || !requiredOK || schema["type"] != "object" || schema["additionalProperties"] != false || len(properties) != len(shape.fields) || len(required) != count {
				t.Fatalf("%s：decoder/schema 对象合同漂移", path)
			}
			seen := map[string]bool{}
			for _, value := range required {
				name, ok := value.(string)
				if !ok || seen[name] {
					t.Fatalf("%s：重复或非法 required", path)
				}
				seen[name] = true
			}
			for name, child := range shape.fields {
				nested, ok := properties[name].(map[string]any)
				if !ok || seen[name] == child.optional {
					t.Fatalf("%s.%s：缺失必填字段", path, name)
				}
				check(child, nested, path+"."+name)
			}
		case 'a':
			item, ok := schema["items"].(map[string]any)
			if schema["type"] != "array" || !ok {
				t.Fatalf("%s：数组合同漂移", path)
			}
			check(*shape.item, item, path+"[]")
		case 's':
			if schema["type"] != "string" {
				t.Fatalf("%s：字符串合同漂移", path)
			}
		default:
			t.Fatalf("%s：未覆盖 shape", path)
		}
	}
	check(authoringMappingShape, schema, "$")
}

func TestSealedMappingDecoderRejectsEachMissingNullCaseAndWrongTypeField(t *testing.T) {
	_, mapping, _ := fullSizeMappingFixture(t)
	mapping.ExecutionProofMappings = []GoalExecutionProofMapping{{ContractRef: "runtime/cross-source-execution-proof/v1", ConsumerItemRef: "consumer", RuleRef: "rule", SourceItemRef: "source"}}
	mapping.EvidencePurposeMappings = []GoalEvidencePurposeMapping{{ContractRef: "runtime/cross-cwi-evidence-purpose/v1", HistoricalItemRef: "history", AcceptanceItemRef: "current"}}
	mapping.GoalRequirement.ResourceIDs = []string{"resource"}
	mapping.Dimensions[0].ResourceIDs = []string{"resource"}
	for index := range mapping.Requirements {
		mapping.Requirements[index].ResourceIDs = []string{"resource"}
	}
	audit := mappingAuditFixture(t, mapping)
	var root map[string]any
	if err := json.Unmarshal(audit.Document.Content, &root); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeDefinitionAuthoringMapping(audit.Document.Content); err != nil {
		t.Fatal(err)
	}
	reject := func(name string) {
		t.Helper()
		raw, err := json.Marshal(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeDefinitionAuthoringMapping(raw); err == nil {
			t.Fatalf("非法字段被接受：%s", name)
		}
	}
	var walk func(mappingShape, any, string)
	walk = func(shape mappingShape, value any, path string) {
		t.Helper()
		switch shape.kind {
		case 'o':
			object := value.(map[string]any)
			object["UnknownField"] = "invented"
			reject(path + ".unknown")
			delete(object, "UnknownField")
			keys := make([]string, 0, len(shape.fields))
			for key := range shape.fields {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := shape.fields[key]
				original, existed := object[key]
				delete(object, key)
				if !child.optional {
					reject(path + "." + key + ".missing")
				}
				object[key] = nil
				reject(path + "." + key + ".null")
				object[key] = false
				reject(path + "." + key + ".wrong_type")
				delete(object, key)
				wrongCase := strings.ToLower(key)
				if wrongCase == key {
					wrongCase = strings.ToUpper(key)
				}
				object[wrongCase] = original
				reject(path + "." + key + ".case")
				delete(object, wrongCase)
				object[key] = original
				walk(child, original, path+"."+key)
				if !existed {
					delete(object, key)
				}
			}
		case 'a':
			values := value.([]any)
			if len(values) == 0 {
				t.Fatalf("%s：fixture 无元素，不能验证嵌套合同", path)
			}
			original := slices.Clone(values)
			values[0] = nil
			reject(path + "[0].null")
			copy(values, original)
			walk(*shape.item, values[0], fmt.Sprintf("%s[0]", path))
		}
	}
	walk(authoringMappingShape, root, "$")
	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeDefinitionAuthoringMapping(raw); err != nil {
		t.Fatalf("反例恢复后合法前像被改变：%v", err)
	}
}
