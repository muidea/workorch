package backend

import (
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestRemoteTemplateContractRoundTripPreservesContextHints(t *testing.T) {
	hints := map[string]any{
		"profileContracts": map[string]any{
			"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "dod-exact"}}},
		},
	}

	wire, err := json.Marshal(&workorchmanagementcontract.WorkspaceTemplate{ID: "template", Name: "Template", ContextHints: hints})
	if err != nil {
		t.Fatalf("marshal remote template: %v", err)
	}
	var decoded workorchmanagementcontract.WorkspaceTemplate
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal remote template: %v", err)
	}
	profiles, ok := decoded.ContextHints["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", decoded.ContextHints)
	}
}
