package workspace

import (
	"testing"

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
