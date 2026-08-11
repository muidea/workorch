package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestLLMOwnerCommandContractsRemainTypedAndLocal(t *testing.T) {
	want := map[string]struct {
		class eventcontract.CommandClass
		lane  eventcontract.LanePolicy
	}{
		TopicLLMCommandGetActiveConfiguration: {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicAcquireProviderCeiling:           {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicPreflightRuntimeCandidate:        {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicRequestRuntimeCandidate:          {class: eventcontract.CommandClassExecute, lane: eventcontract.LanePolicyExecute},
	}

	contracts := make(map[string]eventcontract.Contract, len(Contracts()))
	for _, contract := range Contracts() {
		contracts[contract.Topic] = contract
	}
	messages := make(map[string]eventcontract.MessageContract, len(MessageContracts()))
	for _, contract := range MessageContracts() {
		messages[contract.Topic] = contract
	}

	for topic, expected := range want {
		contract, ok := contracts[topic]
		if !ok {
			t.Fatalf("LLM owner contract %q is not registered", topic)
		}
		if contract.Owner != "llm" || !slices.Equal(contract.ReceiverOwners, []string{"llm"}) || contract.CommandClass != expected.class || contract.LanePolicy != expected.lane {
			t.Fatalf("LLM owner contract %q = %+v", topic, contract)
		}
		message, ok := messages[topic]
		if !ok {
			t.Fatalf("LLM owner message contract %q is not registered", topic)
		}
		if message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.CommandClass != contract.CommandClass || message.LanePolicy != contract.LanePolicy || message.DefinitionPackage != definitionPackage {
			t.Fatalf("LLM owner message contract %q = %+v, event contract = %+v", topic, message, contract)
		}
	}
}
