package appservice

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
)

func TestCLITypedTemplateRoundTripPreservesExecutionContractHints(t *testing.T) {
	template := workspacecore.WorkspaceTemplate{
		ID:   "semantic-proof-template",
		Name: "Semantic proof template",
		ContextHints: map[string]any{
			"profileContracts": map[string]any{
				"software-dev": map[string]any{"evidenceRequirements": []any{map[string]any{"scope": "dodItem", "id": "dod-exact"}}},
			},
		},
	}

	got := cliWorkspaceTemplate(readmodelports.PortTemplateInfo(template))
	profiles, ok := got.ContextHints["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", got.ContextHints)
	}
}
