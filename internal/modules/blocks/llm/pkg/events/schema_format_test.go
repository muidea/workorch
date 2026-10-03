package events

import (
	"encoding/json"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestProviderSchemaFormatMappingAndCompatibility(t *testing.T) {
	for _, schema := range []string{`{}`, `{"type":"object"}`} {
		in := models.ProviderRequest{Schema: schema, SchemaFormat: models.ProviderSchemaFormatJSONSchema}
		out, err := ProviderRequestFromModel(in).ToModel()
		if err != nil || out.SchemaFormat != in.SchemaFormat || out.Schema != schema {
			t.Fatalf("mapping: %+v %v", out, err)
		}
	}
	for _, schema := range []string{"", "legacy-id", "null", "[]", `"object"`, "{"} {
		if _, err := (ProviderRequest{Schema: schema, SchemaFormat: models.ProviderSchemaFormatJSONSchema}).ToModel(); err == nil {
			t.Fatal("accepted explicit invalid schema", schema)
		}
	}
	old := models.ProviderRequest{Schema: "legacy-id"}
	if _, err := ProviderRequestFromModel(old).ToModel(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"schemaFormat":`) {
		t.Fatal("legacy serialized identity changed")
	}
}

func TestProviderSchemaFormatRejectsUnknownFormatAndRawTextConflict(t *testing.T) {
	for _, request := range []ProviderRequest{{Schema: "{}", SchemaFormat: "unknown"}, {Schema: "{}", SchemaFormat: models.ProviderSchemaFormatJSONSchema, RawTextOutput: true}} {
		if _, err := request.ToModel(); err == nil {
			t.Fatal("invalid explicit output contract accepted")
		}
	}
	original := models.ProviderRequest{Schema: "{}", SchemaFormat: models.ProviderSchemaFormatJSONSchema}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var clone models.ProviderRequest
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	if clone.SchemaFormat != original.SchemaFormat || clone.Schema != original.Schema {
		t.Fatal("serialized clone lost explicit schema")
	}
}
