package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestRuntimeGovernanceRequestContractBelongsToDecisionOwner(t *testing.T) {
	var eventValue *eventcontract.Contract
	for _, contract := range Contracts() {
		if contract.Topic == TopicOpenRuntimeGovernanceRequest {
			value := contract
			eventValue = &value
			break
		}
	}
	if eventValue == nil || eventValue.Owner != "decision" || !slices.Equal(eventValue.ReceiverOwners, []string{"decision"}) || eventValue.CommandClass != eventcontract.CommandClassWrite || eventValue.LanePolicy != eventcontract.LanePolicyState {
		t.Fatalf("runtime governance event contract = %+v", eventValue)
	}
	for _, contract := range MessageContracts() {
		if contract.Topic != TopicOpenRuntimeGovernanceRequest {
			continue
		}
		if contract.Owner != eventValue.Owner || !slices.Equal(contract.ReceiverOwners, eventValue.ReceiverOwners) || contract.CommandClass != eventValue.CommandClass || contract.LanePolicy != eventValue.LanePolicy || contract.DefinitionPackage != definitionPackage {
			t.Fatalf("runtime governance message contract = %+v, event contract = %+v", contract, eventValue)
		}
		return
	}
	t.Fatal("runtime governance message contract is not registered")
}
