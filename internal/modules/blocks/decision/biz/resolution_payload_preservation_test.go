package decision

import (
	"testing"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestDecisionResolutionPreservesExactPayloadAndDistinctIdentity(t *testing.T) {
	command := decisioncontracts.ResolveTicketCommand{ResolutionID: "resolution", TicketID: "ticket", Type: "provide_missing_info",
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("answer", "  api_key=example [REDACTED]\r\nBearer literal-example\n"),
			workorcheventpayload.StringField("tokenBudget", "business value"),
			workorcheventpayload.ObjectField("context", workorcheventpayload.New(workorcheventpayload.StringField("token", "example-one"))),
		)}
	original := command.Payload.Clone()
	proposal, err := runtimeGovernanceProposalFromCommand(command)
	if err != nil || !proposal.Payload.Equal(original) || !resolutionFromResolveCommand(command).Payload.Equal(original) {
		t.Fatalf("resolution payload was rewritten: %v", err)
	}
	changed := command
	changed.Payload = command.Payload.Clone()
	changed.Payload.Fields[2].Value.Object[0].Value.String = "example-two"
	other, err := runtimeGovernanceProposalFromCommand(changed)
	if err != nil || other.PayloadHash == proposal.PayloadHash {
		t.Fatal("different original inputs collapsed to one masked identity")
	}
	proposal.Payload.Fields[2].Value.Object[0].Value.String = "caller mutation"
	if !command.Payload.Equal(original) {
		t.Fatal("proposal shares mutable input payload")
	}
}
