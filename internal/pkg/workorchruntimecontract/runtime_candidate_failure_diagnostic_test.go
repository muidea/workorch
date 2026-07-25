package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRuntimeCandidateFailureDiagnosticConversionsDropCauses(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		value *RuntimeCandidateFailureDiagnostic
	}{
		{
			name: "payload",
			value: RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(&RuntimeCandidatePayloadDiagnostic{
				Path: "$.operations[0].parameters[0].name", Actual: "summary", Allowed: []string{"command", "args"},
				Forbidden: []string{"summary"}, Cause: errors.New("provider-private-secret"),
			}),
		},
		{
			name: "semantic",
			value: RuntimeCandidateFailureDiagnosticFromContractDiagnostic(&RuntimeCandidateContractDiagnostic{
				OperationID: "write", Path: "$.operations[0].targetRefs", Actual: "other.go", Required: []string{"main.go"},
				Cause: errors.New("owner-private-secret"),
			}),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.value == nil || testCase.value.Path == "" {
				t.Fatalf("diagnostic = %+v", testCase.value)
			}
			payload, err := json.Marshal(testCase.value)
			if err != nil || strings.Contains(string(payload), "private-secret") || strings.Contains(string(payload), "Cause") {
				t.Fatalf("provider-safe diagnostic JSON=%s err=%v", payload, err)
			}
		})
	}
}
