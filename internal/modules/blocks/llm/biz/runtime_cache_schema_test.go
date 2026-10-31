package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func expandCacheParameterSchema(t testing.TB, schema map[string]any) any {
	t.Helper()
	defs, _ := schema["$defs"].(map[string]any)
	var walk func(any) any
	walk = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"]; ok {
				if len(v) != 1 {
					t.Fatal("reference siblings not compiled")
				}
				s, ok := ref.(string)
				if !ok || len(s) < 8 || s[:8] != "#/$defs/" {
					t.Fatal("external schema ref")
				}
				body, ok := defs[s[8:]]
				if !ok {
					t.Fatal("unresolved schema ref")
				}
				return walk(body)
			}
			ret := map[string]any{}
			for k, child := range v {
				if k != "$defs" {
					ret[k] = walk(child)
				}
			}
			return ret
		case []any:
			ret := make([]any, len(v))
			for i, child := range v {
				ret[i] = walk(child)
			}
			return ret
		default:
			return value
		}
	}
	return walk(schema)
}

func TestCacheParameterSchemaKeepsExactGrammarAndCardinality(t *testing.T) {
	for _, kind := range []string{rc.StepKindFileWrite, rc.StepKindFileEdit} {
		t.Run(kind, func(t *testing.T) {
			manifest := runtimeCandidateManifestForSchema(t, rc.RuntimeDecision{State: rc.ConvergenceStateNeedStateChange, RequiredOutput: rc.RequiredOutputContract{AcceptedKinds: []string{kind}, TargetRefs: []string{"a", "b"}, RequiredOperationSequence: []rc.RuntimeRequiredOperationVariant{{RequirementID: "first", Kind: kind, TargetRef: "a"}, {RequirementID: "second", Kind: kind, TargetRef: "b", DependsOnRequirementIDs: []string{"first"}}}}}, nil)
			wire := runtimeCandidateWireForTest(t, manifest)
			original := []byte(runtimeCandidateCardinalitySchema(manifest, wire))
			compact, err := compactRuntimeParameterSchema(original)
			if err != nil {
				t.Fatal(err)
			}
			if len(compact) >= len(original) || !bytes.Contains(compact, []byte(`"$defs"`)) {
				t.Fatal("repeated schema was not reduced")
			}
			if !reflect.DeepEqual(cacheJSONValue(t, original), expandCacheParameterSchema(t, cacheJSONValue(t, compact).(map[string]any))) {
				t.Fatal("reference expansion changed grammar")
			}
			compile := func(raw []byte) *jsonschema.Schema {
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					t.Fatal(err)
				}
				c := jsonschema.NewCompiler()
				if err := c.AddResource("https://workorch.invalid/cache.json", v); err != nil {
					t.Fatal(err)
				}
				s, err := c.Compile("https://workorch.invalid/cache.json")
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			before, after := compile(original), compile(compact)
			jsonValue := func(value any) any {
				body, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var decoded any
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatal(err)
				}
				return decoded
			}

			parameters := []map[string]any{{"name": "content", "string": "complete body"}}
			if kind == rc.StepKindFileEdit {
				parameters = []map[string]any{{"name": "oldText", "string": "old"}, {"name": "newText", "string": "new"}}
			}
			payload := func(params []map[string]any) map[string]any {
				return map[string]any{"operations": []any{map[string]any{"requiredOperationRef": "first", "parameters": params}, map[string]any{"requiredOperationRef": "second", "parameters": parameters}}}
			}
			good := payload(parameters)
			if before.Validate(jsonValue(good)) != nil || after.Validate(jsonValue(good)) != nil {
				t.Fatalf("valid shared parameter names across slots rejected: original=%v compact=%v", before.Validate(jsonValue(good)), after.Validate(jsonValue(good)))
			}
			if raw, err := json.Marshal(good); err != nil {
				t.Fatal(err)
			} else if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, raw); err != nil {
				t.Fatal("local decoder disagreed on valid wire", err)
			}
			wrongType := append([]map[string]any(nil), parameters...)
			wrongType[0] = map[string]any{"name": parameters[0]["name"], "integer": 1}
			duplicate := append(append([]map[string]any(nil), parameters...), parameters[0])
			unknown := append(append([]map[string]any(nil), parameters...), map[string]any{"name": "unsealed", "string": "x"})
			wrongSlot := payload(parameters)
			wrongSlot["operations"].([]any)[0].(map[string]any)["requiredOperationRef"] = "unsealed"
			tests := []map[string]any{payload(nil), payload(parameters[1:]), payload(duplicate), payload(wrongType), payload(unknown), wrongSlot}
			if kind == rc.StepKindFileEdit {
				optional := append(append([]map[string]any(nil), parameters...), map[string]any{"name": "occurrence", "integer": 1}, map[string]any{"name": "occurrence", "integer": 2})
				tests = append(tests, payload(optional))
			}
			for _, v := range tests {
				if before.Validate(jsonValue(v)) == nil || after.Validate(jsonValue(v)) == nil {
					t.Fatal("grammar admitted a missing, duplicate, unknown or wrong-type field")
				}
				raw, _ := json.Marshal(v)
				if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, raw); err == nil {
					t.Fatal("local manifest decoder admitted invalid candidate")
				}
			}
			t.Logf("complete parameters grammar bytes %d -> %d", len(original), len(compact))
		})
	}
}

func TestCacheParameterSchemaDoesNotCaptureReferencesOrInflate(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object"}`,
		`{"$defs":{"keep":{"type":"string"}},"properties":{"operations":{"items":{"oneOf":[]}}}}`,
		`{"properties":{"operations":{"items":{"oneOf":[{"properties":{"parameters":{"$ref":"#/original"}}},{"properties":{"parameters":{"$ref":"#/original"}}}]}}}}`,
		`{"properties":{"operations":{"items":{"oneOf":[{"properties":{"parameters":{"$id":"https://example.invalid/a","type":"array"}}},{"properties":{"parameters":{"$id":"https://example.invalid/a","type":"array"}}}]}}}}`,
	} {
		got, err := compactRuntimeParameterSchema([]byte(raw))
		if err != nil || !bytes.Equal(got, []byte(raw)) {
			t.Fatal("unrecognized namespace or non-repeated schema changed", err)
		}
	}
	for _, key := range []string{"$ref", "$id", "id", "$anchor", "$dynamicRef", "$dynamicAnchor", "$recursiveRef", "$recursiveAnchor", "$schema"} {
		body, _ := json.Marshal(map[string]any{"nested": map[string]any{key: "identity"}})
		if selfContainedParameterSchema(body) {
			t.Fatal("schema namespace was factored", key)
		}
	}
}
