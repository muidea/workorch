package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeRuntimeCandidatePayloadClassifiesIncompleteAndEmptyDocuments(t *testing.T) {
	for name, payload := range map[string][]byte{
		"mid token":  []byte(`{"operations":[{"id":"write"`),
		"open array": []byte(`{"operations":[`),
		"empty":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayload(RuntimeActionProduceStateChange, payload)
			if err == nil {
				t.Fatal("decode unexpectedly succeeded")
			}
			got := RuntimeCandidateDocumentDispositionForError(err)
			want := RuntimeCandidateDocumentIncomplete
			if name == "empty" {
				want = RuntimeCandidateDocumentEmpty
			}
			if got != want {
				t.Fatalf("document disposition = %q, want %q (err=%v)", got, want, err)
			}
			if name != "empty" {
				var incomplete *RuntimeCandidatePayloadIncompleteError
				if !errors.As(err, &incomplete) {
					t.Fatalf("error %T does not preserve incomplete marker", err)
				}
			}
		})
	}
}

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
