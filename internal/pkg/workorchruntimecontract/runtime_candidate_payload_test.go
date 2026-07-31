package workorchruntimecontract

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeRuntimeCandidatePayloadDocumentOnlyUnwrapsOneExactJSONFence(t *testing.T) {
	bare := []byte(`{"operations":[]}`)
	fenced := []byte("```json\n" + string(bare) + "\n```")

	normalized, wrapped := NormalizeRuntimeCandidatePayloadDocument(fenced)
	if !wrapped || !bytes.Equal(normalized, bare) {
		t.Fatalf("fenced candidate normalization = %q wrapped=%v, want bare JSON", normalized, wrapped)
	}

	for name, payload := range map[string][]byte{
		"bare JSON":             bare,
		"prose before fence":    append([]byte("candidate:\n"), fenced...),
		"prose after fence":     append(append([]byte(nil), fenced...), []byte("\nthanks")...),
		"wrong language":        []byte("```yaml\n" + string(bare) + "\n```"),
		"missing closing fence": []byte("```json\n" + string(bare)),
		"empty fenced body":     []byte("```json\n\n```"),
		"multiple outer fences": []byte("```json\n" + string(bare) + "\n```\n```json\n{}\n```"),
	} {
		t.Run(name, func(t *testing.T) {
			got, accepted := NormalizeRuntimeCandidatePayloadDocument(payload)
			if accepted || !bytes.Equal(got, payload) {
				t.Fatalf("normalization must preserve non-canonical payload: got=%q accepted=%v", got, accepted)
			}
		})
	}
}

func TestDecodeRuntimeCandidatePayloadClassifiesIncompleteAndEmptyDocuments(t *testing.T) {
	for name, payload := range map[string][]byte{
		"mid token":  []byte(`{"operations":[{"id":"write"`),
		"open array": []byte(`{"operations":[`),
		"empty":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayload(ConvergenceStateNeedStateChange, payload)
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
	candidate, err := DecodeRuntimeCandidatePayload(ConvergenceStateNeedStateChange, []byte(`{
		"operations":[
			{"id":"prepare","kind":"git/write","bindingRefs":["binding/test/cwi-1"]},
			{"id":"commit","kind":"git/write","dependsOn":["prepare"],"bindingRefs":["binding/test/cwi-1"]}
		]
	}`))
	if err != nil {
		t.Fatalf("operation dependency candidate: %v", err)
	}
	if candidate.StateChange == nil || len(candidate.StateChange.Operations) != 2 || len(candidate.StateChange.Operations[1].Operation.DependsOn) != 1 || candidate.StateChange.Operations[1].Operation.DependsOn[0] != "prepare" {
		t.Fatalf("dependency was not preserved: %+v", candidate)
	}

	_, err = DecodeRuntimeCandidatePayload(ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{"id":"write","kind":"file/write","bindingRefs":["binding/test/cwi-1"]}],
		"dependsOn":["prepare"]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "dependsOn"`) {
		t.Fatalf("top-level dependency error = %v, want strict unknown-field rejection", err)
	}
}
