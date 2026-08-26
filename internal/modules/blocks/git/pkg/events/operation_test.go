package events

import (
	"slices"
	"strings"
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

func TestSealedRuntimeOperationBindsPayloadAndExecutionScope(t *testing.T) {
	operationName := "add"
	operation := runtimecontract.RuntimeOperation{
		ID: "git-add", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "operation", String: &operationName},
			{Name: "pathspecs", StringList: []string{"main.go"}},
		},
	}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch",
		Execution: testExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws/run/dispatch/git-add", Fence: 1,
			ExpectedEffectScope: runtimecontract.ResolveExpectedEffectScope(operation, nil),
		}, nil),
		OperationID: operation.ID, TargetRefs: []string{"main.go"},
	}
	validated, err := sealed.ValidateForExecution(context)
	if err != nil || validated.ID != operation.ID || validated.TargetRefs[0] != "main.go" {
		t.Fatalf("validated operation=%+v err=%v", validated, err)
	}
	idempotencyDrift := context
	idempotencyDrift.Execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencyAtMostOnce
	if _, err := sealed.ValidateForExecution(idempotencyDrift); err == nil || !strings.Contains(err.Error(), "idempotency class mismatch") {
		t.Fatalf("sealed Git operation accepted idempotency drift: %v", err)
	}
	authorityDrift := context
	authorityDrift.Execution.AuthorityHash = "forged"
	if _, err := sealed.ValidateForExecution(authorityDrift); err == nil || !strings.Contains(err.Error(), "authority hash mismatch") {
		t.Fatalf("sealed Git operation accepted authority hash drift: %v", err)
	}

	mutated := sealed
	mutated.Operation.Parameters[1].StringList = []string{"other.go"}
	if _, err := mutated.ValidateForExecution(context); err == nil {
		t.Fatal("mutated Git operation payload reused the sealed hash")
	}
	mismatchedContext := context
	mismatchedContext.TargetRefs = []string{"other.go"}
	if _, err := sealed.ValidateForExecution(mismatchedContext); err == nil {
		t.Fatal("sealed Git operation accepted a different execution target")
	}
}
