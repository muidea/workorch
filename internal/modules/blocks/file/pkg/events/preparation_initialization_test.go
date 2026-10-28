package events

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func TestPreparationInitializationPlanPreservesExactJSONAcrossPersistence(t *testing.T) {
	raw := []byte("{\r\n\"n\":18446744073709551615,\"dec\":1.000,\"custom\":[null,{}]}\n")
	c := InitializePreparationResourcesCommand{WorkspaceID: "ws", RequestID: "original", BindingHash: strings.Repeat("a", 64), InternalDirectory: "/internal/ws", DeliverableDirectory: "/project", Declarations: []PreparationInitializationDeclaration{{Scope: workorchmanagedconfig.ScopeSystem, Path: "mcp/config.json", Policy: workorchmanagedconfig.PolicySystemExact, DefaultsJSON: raw}}}
	if c.Validate() != nil {
		t.Fatal("invalid exact JSON plan")
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var reopened InitializePreparationResourcesCommand
	if json.Unmarshal(encoded, &reopened) != nil || !reflect.DeepEqual(c, reopened) || !bytes.Equal(reopened.Declarations[0].DefaultsJSON, raw) {
		t.Fatal("lost exact body across journal persistence")
	}
	copy := c.Clone()
	copy.Declarations[0].DefaultsJSON[0] = 'x'
	if !bytes.Equal(c.Declarations[0].DefaultsJSON, raw) {
		t.Fatal("clone aliases raw bytes")
	}
	for _, mutate := range []func(*InitializePreparationResourcesCommand){func(v *InitializePreparationResourcesCommand) { v.Declarations[0].Path = "../escape" }, func(v *InitializePreparationResourcesCommand) { v.RequestID = "new\nrequest" }, func(v *InitializePreparationResourcesCommand) { v.BindingHash = "not-a-hash" }, func(v *InitializePreparationResourcesCommand) {
		v.Declarations = append(v.Declarations, v.Declarations[0])
	}, func(v *InitializePreparationResourcesCommand) {
		v.Declarations[0].DefaultsJSON = []byte(`{"duplicate":1,"duplicate":2}`)
	}} {
		bad := c.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("accepted invalid plan")
		}
	}
}
