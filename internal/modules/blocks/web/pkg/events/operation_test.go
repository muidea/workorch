package events

import (
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
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
}
