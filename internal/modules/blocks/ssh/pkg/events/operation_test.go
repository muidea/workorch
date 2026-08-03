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
		if contract.Owner != "ssh" || !slices.Equal(contract.ReceiverOwners, []string{"ssh"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	host, username, password, command := "example.internal", "operator", "secret", "uname -a"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{ID: "ssh-1", Kind: runtimecontract.StepKindSSHCommand, Parameters: []runtimecontract.OperationParameter{{Name: "host", String: &host}, {Name: "username", String: &username}, {Name: "password", String: &password}, {Name: "command", String: &command}}})
	if err != nil {
		t.Fatal(err)
	}
	if data.Host != host || data.Username != username || data.Command != command {
		t.Fatalf("data=%+v", data)
	}
}
