package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestDecodeRuntimeCandidatePayloadAcceptsOperationDependencyOnly(t *testing.T) {
	candidate, err := DecodeRuntimeCandidatePayload(RuntimeActionProduceStateChange, []byte(`{
		"operations":[
			{"id":"prepare","kind":"git/write","coverageWorkItemIds":["cwi-1"]},
			{"id":"commit","kind":"git/write","dependsOn":["prepare"],"coverageWorkItemIds":["cwi-1"]}
		]
	}`))
	if err != nil {
		t.Fatalf("operation dependency candidate: %v", err)
	}
	if candidate.StateChange == nil || len(candidate.StateChange.Operations) != 2 || len(candidate.StateChange.Operations[1].Operation.DependsOn) != 1 || candidate.StateChange.Operations[1].Operation.DependsOn[0] != "prepare" {
		t.Fatalf("dependency was not preserved: %+v", candidate)
	}

	_, err = DecodeRuntimeCandidatePayload(RuntimeActionProduceStateChange, []byte(`{
		"operations":[{"id":"write","kind":"file/write","coverageWorkItemIds":["cwi-1"]}],
		"dependsOn":["prepare"]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "dependsOn"`) {
		t.Fatalf("top-level dependency error = %v, want strict unknown-field rejection", err)
	}
}
