package workspace

import (
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceTemplateOwnerMappingsDeepCloneCommandAndRepositoryValues(t *testing.T) {
	command := workspaceevents.CreateWorkspaceTemplateCommand{
		TemplateID: "template-1", DoDItems: []string{"dod-original"},
		SoulBindings:     []soulbinding.Binding{{Stage: "planning", SoulID: "soul-original"}},
		ContextHints:     workorcheventpayload.New(workorcheventpayload.ObjectField("nested", workorcheventpayload.New(workorcheventpayload.StringField("value", "hint-original")))),
		EvidencePolicies: []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringArrayField("checks", []string{"check-original"}))},
	}
	model, err := workspaceTemplateFromCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	command.DoDItems[0] = "command-mutated"
	command.SoulBindings[0].SoulID = "command-soul-mutated"
	command.ContextHints.Fields[0].Value.Object[0].Value.String = "command-hint-mutated"
	command.EvidencePolicies[0].Fields[0].Value.Array[0].String = "command-policy-mutated"
	if model.DoDItems[0] != "dod-original" || model.SoulBindings["planning"] != "soul-original" ||
		model.ContextHints.Fields[0].Value.Object[0].Value.String != "hint-original" || model.EvidencePolicies[0].Fields[0].Value.Array[0].String != "check-original" {
		t.Fatalf("command mutation leaked to owner model: %+v", model)
	}

	dto := workspaceTemplateFromRepository(model)
	dto.DoDItems[0] = "dto-mutated"
	dto.SoulBindings[0].SoulID = "dto-soul-mutated"
	dto.ContextHints.Fields[0].Value.Object[0].Value.String = "dto-hint-mutated"
	dto.EvidencePolicies[0].Fields[0].Value.Array[0].String = "dto-policy-mutated"
	if model.DoDItems[0] != "dod-original" || model.SoulBindings["planning"] != "soul-original" ||
		model.ContextHints.Fields[0].Value.Object[0].Value.String != "hint-original" || model.EvidencePolicies[0].Fields[0].Value.Array[0].String != "check-original" {
		t.Fatalf("repository DTO mutation leaked to owner model: %+v", model)
	}
}
