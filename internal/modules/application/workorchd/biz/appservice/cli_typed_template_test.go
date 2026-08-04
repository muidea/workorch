package appservice

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
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

	got, err := cliWorkspaceTemplate(workorchdeventmapping.TemplateDTO(readmodelports.PortTemplateInfo(template)))
	if err != nil {
		t.Fatal(err)
	}
	profiles, ok := got.ContextHints["profileContracts"].(map[string]any)
	if !ok || profiles["software-dev"] == nil {
		t.Fatalf("ContextHints = %#v, want profileContracts preserved", got.ContextHints)
	}
}
