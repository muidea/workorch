package events

import (
	"strings"
	"testing"
)

func TestEventHubRegisteredTopicsHaveMessageContracts(t *testing.T) {
	for topic := range eventHubContracts {
		message, ok := ResolveEventHubMessageContract(topic)
		if !ok {
			t.Fatalf("topic %s has event contract but no message contract", topic)
		}
		if strings.TrimSpace(message.Owner) == "" {
			t.Fatalf("topic %s message contract has empty owner", topic)
		}
		if strings.TrimSpace(message.DefinitionPackage) == "" {
			t.Fatalf("topic %s message contract has empty definition package", topic)
		}
	}
}

func TestEventHubMessageContractsMatchDeliveryContracts(t *testing.T) {
	for topic, message := range eventHubMessageContracts {
		contract, ok := ResolveEventHubContract(topic)
		if !ok {
			t.Fatalf("topic %s has message contract but no event contract", topic)
		}
		if message.Owner != contract.Owner {
			t.Fatalf("topic %s owner = %s, want %s", topic, message.Owner, contract.Owner)
		}
		if message.CommandClass != contract.CommandClass {
			t.Fatalf("topic %s command class = %s, want %s", topic, message.CommandClass, contract.CommandClass)
		}
		if message.LanePolicy != contract.LanePolicy {
			t.Fatalf("topic %s lane policy = %s, want %s", topic, message.LanePolicy, contract.LanePolicy)
		}
	}
}
