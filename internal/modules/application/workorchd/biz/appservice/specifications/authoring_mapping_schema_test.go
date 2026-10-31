package specifications

import (
	"encoding/json"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestAuthoringMappingProviderSchemaIsClosedAndDTOAligned(t *testing.T) {
	raw, err := workspace.AuthoringWireSchema(workspace.DefinitionAuthoringOwnerBound, "proposal")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		t.Fatal(err)
	}
	props := schema["properties"].(map[string]any)
	mapping := props["mapping"].(map[string]any)
	typ := reflect.TypeOf(workspace.DefinitionAuthoringMapping{})
	fields := []reflect.StructField{}
	for n := 0; n < typ.NumField(); n++ {
		f := typ.Field(n)
		switch f.Name {
		case "Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash":
			continue
		}
		fields = append(fields, f)
	}
	checkSchemaDTO(t, mapping, reflect.StructOf(fields))
	for _, name := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"} {
		if _, ok := mapping["properties"].(map[string]any)[name]; ok {
			t.Fatal("provider receives owner field", name)
		}
	}
	if _, err := workspace.AuthoringWireSchema("workorch.authoring.owner-bound/v1", "proposal"); err == nil {
		t.Fatal("obsolete schema alias accepted")
	}
}
