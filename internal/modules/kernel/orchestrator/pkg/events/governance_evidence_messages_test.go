package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestGovernanceEvidenceCommandsAreOrchestratorOwned(t *testing.T) {
	want := []string{
		TopicOrchestratorCommandRecordDecisionEvidence,
		TopicOrchestratorCommandRecordApprovalEvidence,
	}
	contracts := make(map[string]eventcontract.Contract, len(Contracts()))
	for _, contract := range Contracts() {
		contracts[contract.Topic] = contract
	}
	messages := make(map[string]eventcontract.MessageContract, len(MessageContracts()))
	for _, message := range MessageContracts() {
		messages[message.Topic] = message
	}
	for _, topic := range want {
		contract, ok := contracts[topic]
		if !ok || contract.Owner != "orchestrator" || !slices.Equal(contract.ReceiverOwners, []string{"orchestrator"}) || contract.CommandClass != eventcontract.CommandClassWrite || contract.LanePolicy != eventcontract.LanePolicyState {
			t.Fatalf("governance evidence contract %q = %+v", topic, contract)
		}
		message, ok := messages[topic]
		if !ok || message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.DefinitionPackage != definitionPackage {
			t.Fatalf("governance evidence message %q = %+v", topic, message)
		}
	}
}

func TestGovernanceEvidencePrincipalRejectsUnknownKind(t *testing.T) {
	if err := (GovernanceEvidencePrincipal{Kind: "human_operator", SubjectID: "operator"}).Validate(); err != nil {
		t.Fatalf("valid principal rejected: %v", err)
	}
	if err := (GovernanceEvidencePrincipal{Kind: "unknown", SubjectID: "operator"}).Validate(); err == nil {
		t.Fatal("unknown governance evidence principal kind accepted")
	}
}
