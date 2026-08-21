package events

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	sessionRef, credentialRef, command := "session-a", "credential-a", "uname -a"
	operation := runtimecontract.RuntimeOperation{ID: "ssh-1", Kind: runtimecontract.StepKindSSHCommand, TargetRefs: []string{"target-a"}, Parameters: []runtimecontract.OperationParameter{{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &command}}}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	if data.TargetRef != "target-a" || data.SessionRef != sessionRef || data.CredentialRef != credentialRef || data.Command != command {
		t.Fatalf("data=%+v", data)
	}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{OperationID: operation.ID, TargetRefs: []string{"target-a"}}
	if _, err := sealed.ValidateForExecution(context); err != nil {
		t.Fatalf("validate sealed operation: %v", err)
	}
	drifted := sealed
	drifted.Operation.Parameters[0].String = ptrString("session-b")
	if _, err := drifted.ValidateForExecution(context); err == nil || !strings.Contains(err.Error(), "payload hash") {
		t.Fatalf("drifted sealed operation err = %v, want payload hash conflict", err)
	}
	wire, err := json.Marshal(ExecuteOperationCommand{RuntimeOperationExecutionContext: context, SealedOperation: sealed})
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"example.internal", "password", "privateKey", "username"} {
		if strings.Contains(string(wire), prohibited) {
			t.Fatalf("SSH EventHub wire contains prohibited endpoint/credential field %q: %s", prohibited, wire)
		}
	}
}

func ptrString(value string) *string { return &value }
