package specifications

import (
	"encoding/json"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestAuthoringMappingProviderSchemaIsClosedAndDTOAligned(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(authoringSpecificationOutputSchema("proposal")), &schema); err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	required, requiredOK := schema["required"].([]any)
	if !ok || !requiredOK || schema["type"] != "object" || schema["additionalProperties"] != false || len(properties) != 1 || len(required) != 1 || required[0] != "mapping" {
		t.Fatal("提案 wrapper 不是唯一封闭 mapping")
	}
	mapping, ok := properties["mapping"].(map[string]any)
	if !ok {
		t.Fatal("缺少 mapping schema")
	}
	// The immutable v1/v2 schema intentionally excludes the v3 extension.
	typ := reflect.TypeOf(workspace.DefinitionAuthoringMapping{})
	fields := []reflect.StructField{}
	for n := 0; n < typ.NumField(); n++ {
		if typ.Field(n).Name != "ProfileAcceptanceSelections" {
			fields = append(fields, typ.Field(n))
		}
	}
	checkSchemaDTO(t, mapping, reflect.StructOf(fields))
	protocol := mapping["properties"].(map[string]any)["Protocol"].(map[string]any)["enum"]
	if !reflect.DeepEqual(protocol, []any{workspace.DefinitionAuthoringMappingProtocol}) {
		t.Fatal("mapping schema 协议不是唯一当前版本")
	}
	if authoringSpecificationOutputSchema("review/traceability") != specificationOutputSchema("review/traceability") {
		t.Fatal("独立 review 合同被提案 mapping 替换")
	}
}
