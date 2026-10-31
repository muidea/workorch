package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCardinalitySchemaRejectsDuplicateNamesWithDistinctValues(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, rc.RuntimeDecision{State: rc.ConvergenceStateNeedStateChange, RequiredOutput: rc.RequiredOutputContract{
		AcceptedKinds: []string{rc.StepKindFileEdit}, TargetRefs: []string{"one.txt", "two.txt"}, RequiredOperationSequence: []rc.RuntimeRequiredOperationVariant{
			{RequirementID: "one", Kind: rc.StepKindFileEdit, TargetRef: "one.txt"}, {RequirementID: "two", Kind: rc.StepKindFileEdit, TargetRef: "two.txt"},
		},
	}}, nil)
	wire := runtimeCandidateWireForTest(t, manifest)
	schemaText := runtimeCandidateCardinalitySchema(manifest, wire)
	var schemaDoc any
	if err := json.Unmarshal([]byte(schemaText), &schemaDoc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("urn:cardinality-test", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:cardinality-test")
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"operations":[{"requiredOperationRef":"one","parameters":[{"name":"oldText","string":"a"},{"name":"newText","string":""}]},{"requiredOperationRef":"two","parameters":[{"name":"oldText","string":"b"},{"name":"newText","string":"c"},{"name":"occurrence","integer":1}]}]}`
	for name, body := range map[string]string{
		"distinct values":    strings.Replace(valid, `"string":"a"}`, `"string":"a"},{"name":"oldText","string":"different"}`, 1),
		"equal values":       strings.Replace(valid, `"string":"a"}`, `"string":"a"},{"name":"oldText","string":"a"}`, 1),
		"optional duplicate": strings.Replace(valid, `"integer":1}`, `"integer":1},{"name":"occurrence","integer":2}`, 1),
		"missing required":   strings.Replace(valid, `{"name":"oldText","string":"a"},`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			var value any
			json.Unmarshal([]byte(body), &value)
			if schema.Validate(value) == nil {
				t.Fatal("schema accepted invalid name cardinality")
			}
			if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(body)); err == nil {
				t.Fatal("decoder accepted invalid name cardinality")
			}
		})
	}
	var good any
	json.Unmarshal([]byte(valid), &good)
	if err := schema.Validate(good); err != nil {
		t.Fatal(err)
	}
	if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(valid)); err != nil {
		t.Fatal(err)
	}
	// The legacy schema remains available for exact recovery identity.
	var legacy any
	json.Unmarshal([]byte(runtimeCandidateSchemaForManifest(manifest)), &legacy)
	if err := compiler.AddResource("urn:cardinality-legacy", legacy); err != nil {
		t.Fatal(err)
	}
	old, err := compiler.Compile("urn:cardinality-legacy")
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(valid, `"string":"a"}`, `"string":"a"},{"name":"oldText","string":"different"}`, 1)
	var value any
	json.Unmarshal([]byte(duplicate), &value)
	if err := old.Validate(value); err != nil {
		t.Fatalf("legacy rendering unexpectedly changed: %v", err)
	}
}

func TestCardinalitySchemaKeepsDynamicParameterCapacity(t *testing.T) {
	manifest := runtimeCandidateManifestForSchema(t, rc.RuntimeDecision{State: rc.ConvergenceStateNeedValidation, RequiredOutput: rc.RequiredOutputContract{AcceptedKinds: []string{rc.StepKindMCPPromptGet}}}, nil)
	wire := runtimeCandidateWireForTest(t, manifest)
	var schemaDoc any
	if err := json.Unmarshal([]byte(runtimeCandidateCardinalitySchema(manifest, wire)), &schemaDoc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("urn:dynamic", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:dynamic")
	if err != nil {
		t.Fatal(err)
	}
	parameters := []map[string]any{{"name": "serverId", "string": "server"}, {"name": "prompt", "string": "inspect"}}
	for i := 0; i < 20; i++ {
		parameters = append(parameters, map[string]any{"name": fmt.Sprintf("arguments.value%d", i), "string": "value"})
	}
	document := map[string]any{"operations": []any{map[string]any{"kind": rc.StepKindMCPPromptGet, "parameters": parameters}}}
	body, _ := json.Marshal(document)
	var value any
	json.Unmarshal(body, &value)
	if err := schema.Validate(value); err != nil {
		t.Fatal("v3 reduced authorized dynamic capacity", err)
	}
	if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, body); err != nil {
		t.Fatal(err)
	}
	parameters = append(parameters, parameters[2])
	document["operations"].([]any)[0].(map[string]any)["parameters"] = parameters
	body, _ = json.Marshal(document)
	if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, body); err == nil {
		t.Fatal("dynamic name duplication passed strict decoder")
	}
	if !strings.Contains(runtimeCandidateContractCatalogCurrent(), "same name in different operations") || !strings.Contains(runtimeCandidateContractCatalogCurrent(), "contiguous oldText") {
		t.Fatal("wire and File encoding instructions missing")
	}
}
