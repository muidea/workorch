package appservice

import (
	"testing"

	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestCLITypedTemplateRoundTripPreservesExecutionContractHints(t *testing.T) {
	template := workorchmanagementcontract.WorkspaceTemplate{
		ID:   "semantic-proof-template",
		Name: "Semantic proof template",
		ContextHints: map[string]any{
			"profileContracts": map[string]any{
				"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "dod-exact"}}},
			},
		},
	}

	got, err := cliWorkspaceTemplate(workorchdeventmapping.TemplateDTO(port.TemplateInfoFromManagement(template)))
	if err != nil {
		t.Fatal(err)
	}
	profiles, ok := got.ContextHints["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", got.ContextHints)
	}
}
