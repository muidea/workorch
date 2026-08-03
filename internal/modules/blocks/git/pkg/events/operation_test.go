package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestContractsDeclareGitAsCapabilityStateAndExecutionReceiver(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 7 {
		t.Fatalf("contracts = %+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "git" || !slices.Equal(contract.ReceiverOwners, []string{"git"}) {
			t.Fatalf("contract = %+v", contract)
		}
	}
	for _, contract := range contracts[:6] {
		if contract.CommandClass != eventcontract.CommandClassRead || contract.LanePolicy != eventcontract.LanePolicyQuery {
			t.Fatalf("capability query contract = %+v", contract)
		}
	}
	if len(MessageContracts()) != 7 {
		t.Fatalf("message contract count = %d, want 7", len(MessageContracts()))
	}
}

func TestOperationDataFromRuntimeOperation(t *testing.T) {
	operationName := "commit"
	message := "seal runtime work"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "git-1", Kind: runtimecontract.StepKindGitWrite,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "operation", String: &operationName}, {Name: "message", String: &message},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Operation != operationName || data.Message != message {
		t.Fatalf("git operation data = %+v", data)
	}
}
