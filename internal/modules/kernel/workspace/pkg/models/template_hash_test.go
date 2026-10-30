package workspace

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestStableWorkspaceTemplateContentHashIncludesPolicyAndBindings(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{"policy": map[string]any{"maxWorkItems": 1}})
	if err != nil {
		t.Fatal(err)
	}
	base := WorkspaceTemplate{ID: "template", Name: "name", DoDItems: []string{"acceptance"}, SoulBindings: map[string]string{"planning": "p", "execution": "e"}, ContextHints: hints}
	hash := StableWorkspaceTemplateContentHash(&base)
	if hash == "" || StableWorkspaceTemplateContentHash(nil) != "" {
		t.Fatal("invalid template fingerprint")
	}
	reordered := base
	reordered.SoulBindings = map[string]string{"execution": "e", "planning": "p"}
	if StableWorkspaceTemplateContentHash(&reordered) != hash {
		t.Fatal("map ordering changed content hash")
	}
	for _, mutate := range []func(*WorkspaceTemplate){
		func(value *WorkspaceTemplate) { value.ID = "another" },
		func(value *WorkspaceTemplate) { value.DoDItems = []string{"different acceptance"} },
		func(value *WorkspaceTemplate) { value.SoulBindings = map[string]string{"execution": "custom"} },
		func(value *WorkspaceTemplate) {
			value.ContextHints = workorcheventpayload.New(workorcheventpayload.StringField("policy", "custom"))
		},
	} {
		changed := base
		mutate(&changed)
		if StableWorkspaceTemplateContentHash(&changed) == hash {
			t.Fatal("template identity/content change not fingerprinted")
		}
	}
}

func TestPreviousSoftwareTemplateCapacityIsRecognizedForExplicitUpgrade(t *testing.T) {
	// Exact public seed from Git 5c3b40e2, not today's template with one field
	// edited. Profile/adapter changes are part of the same content identity.
	raw, err := os.ReadFile("testdata/software-dev-5c3b40e2.json")
	if err != nil {
		t.Fatal(err)
	}
	var previous WorkspaceTemplate
	if err := json.Unmarshal(raw, &previous); err != nil {
		t.Fatal(err)
	}
	hash := StableWorkspaceTemplateContentHash(&previous)
	if previous.ID != "software-dev" || hash != "workspace-template-content-sha256:2642198f04d3247b7ba5cfa4040b8cf07088b48197fd8926d9709df1681d9942" || workorchdefaults.KnownBuiltinContentRevision("workspace_template", previous.ID, hash) != "5c3b40e2" {
		t.Fatalf("previous exact template is not recognized: %s", hash)
	}
	previous.Name += " changed"
	if workorchdefaults.KnownBuiltinContentRevision("workspace_template", previous.ID, StableWorkspaceTemplateContentHash(&previous)) != "" {
		t.Fatal("custom content recognized as builtin")
	}
}
