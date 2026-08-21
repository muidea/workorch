package events

import (
	"encoding/json"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
	"strings"
	"testing"
)

func TestOwnerContractAndOperationMapper(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 5 || len(MessageContracts()) != 5 {
		t.Fatalf("contracts=%+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "web" || !slices.Equal(contract.ReceiverOwners, []string{"web"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	query := "typed event contracts"
	maxResults := int64(5)
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{ID: "web-1", Kind: runtimecontract.StepKindWebSearch, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}}})
	if err != nil {
		t.Fatal(err)
	}
	if data.Query != query || data.MaxResults != maxResults || !data.SafeSearch {
		t.Fatalf("data=%+v", data)
	}
	operation := runtimecontract.RuntimeOperation{ID: "web-1", Kind: runtimecontract.StepKindWebSearch, TargetRefs: []string{"web-target"}, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}}}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{OperationID: operation.ID, TargetRefs: operation.TargetRefs}
	if _, err := sealed.ValidateForExecution(context); err != nil {
		t.Fatal(err)
	}
	drifted := sealed
	for index := range drifted.Operation.Parameters {
		if drifted.Operation.Parameters[index].Name == "query" {
			drifted.Operation.Parameters[index].String = ptrWebString("changed")
		}
	}
	if _, err := drifted.ValidateForExecution(context); err == nil || !strings.Contains(err.Error(), "payload hash") {
		t.Fatalf("drifted sealed operation err=%v", err)
	}
	wire, err := json.Marshal(ExecuteOperationCommand{RuntimeOperationExecutionContext: context, SealedOperation: sealed})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "approvalId") {
		t.Fatalf("Web operation wire exposes legacy approvalId: %s", wire)
	}
}

func ptrWebString(value string) *string { return &value }
