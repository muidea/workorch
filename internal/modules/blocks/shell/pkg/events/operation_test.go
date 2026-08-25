package events

import (
	"slices"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestContractsDeclareShellAsExecutionReceiver(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 7 {
		t.Fatalf("contracts = %+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "shell" || !slices.Equal(contract.ReceiverOwners, []string{"shell"}) {
			t.Fatalf("contract = %+v", contract)
		}
	}
	if len(MessageContracts()) != 7 {
		t.Fatalf("message contract count = %d, want 7", len(MessageContracts()))
	}
}

func TestManagedValidationSessionOperationData(t *testing.T) {
	recipeID := "go-test"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.ValidationRecipeID != recipeID || data.Command != "" || len(data.Args) != 0 || data.TimeoutMillis != 0 {
		t.Fatalf("managed session shell data = %+v", data)
	}
	if !data.ManagedValidation.RuntimeManaged || data.ManagedValidation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession || !data.ManagedValidation.AutoCleanup {
		t.Fatalf("managed validation metadata = %+v", data.ManagedValidation)
	}
}
