package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestRuntimeStateIntegrityHashKeepsExactJSONValue(t *testing.T) {
	for _, r := range []RuntimeStateRecord{{}, {Facts: RuntimeFacts{RunID: "literal\n\u2028<原文>&\x00"}}, {ProcessedEventIDs: []string{}}, {ProcessedEventIDs: []string{"", "\n", "原文"}}} {
		body, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		if got := StableRuntimeStateRecordHash(r); got != "runtime-state-"+hex.EncodeToString(digest[:]) {
			t.Fatal("raw JSON identity changed", got)
		}
		for size := 1; size <= len(body)+1; size *= 2 {
			writer := runtimeJSONValueHashWriter{digest: sha256.New()}
			framed := append(append([]byte{}, body...), '\n')
			for offset := 0; offset < len(framed); offset += size {
				end := min(offset+size, len(framed))
				if _, err := writer.Write(framed[offset:end]); err != nil {
					t.Fatal(err)
				}
			}
			if !writer.hasPending || writer.pending != '\n' || hex.EncodeToString(writer.digest.Sum(nil)) != hex.EncodeToString(digest[:]) {
				t.Fatal("chunking changed original value bytes")
			}
		}
	}
}

func TestRuntimeStateIntegrityHashFixedArchivePreservesOriginalFingerprint(t *testing.T) {
	_, _, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		body, err := json.Marshal(run.RuntimeState)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		if StableRuntimeStateRecordHash(run.RuntimeState) != "runtime-state-"+hex.EncodeToString(digest[:]) {
			t.Fatal("complete raw archive fingerprint changed")
		}
	}
}
