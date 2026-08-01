package backend

import (
	"encoding/json"
	"testing"

	client "github.com/muidea/workorch/internal/pkg/workorchclient"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestRemoteTemplateContractRoundTripPreservesContextHints(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{
		"profileContracts": map[string]any{
			"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "dod-exact"}}},
		},
	})
	if err != nil {
		t.Fatalf("context hints: %v", err)
	}

	wire, err := json.Marshal(clientTemplateFromInfo(&TemplateInfo{ID: "template", Name: "Template", ContextHints: hints}))
	if err != nil {
		t.Fatalf("marshal remote template: %v", err)
	}
	var decoded client.WorkspaceTemplate
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal remote template: %v", err)
	}
	got := templateInfoFromClient(&decoded)
	profiles, ok := got.ContextHints.Map()["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", got.ContextHints.Map())
	}
}
