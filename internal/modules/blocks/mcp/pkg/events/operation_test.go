package events

import (
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
	"testing"
)

func TestOwnerContractAndOperationMapper(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 6 || len(MessageContracts()) != 6 {
		t.Fatalf("contracts=%+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "mcp" || !slices.Equal(contract.ReceiverOwners, []string{"mcp"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	server, tool, path := "context", "lookup", "main.go"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{ID: "mcp-1", Kind: runtimecontract.StepKindMCPToolCall, Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &server}, {Name: "tool", String: &tool}, {Name: "input.path", String: &path}}})
	if err != nil {
		t.Fatal(err)
	}
	if data.ServerID != server || data.Tool != tool || len(data.Input) != 1 || data.Input[0].Name != "path" || data.Input[0].String == nil || *data.Input[0].String != path {
		t.Fatalf("data=%+v", data)
	}
}
