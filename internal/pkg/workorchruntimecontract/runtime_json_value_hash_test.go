package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func referenceJSONValueHash(prefix string, value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return prefix + hex.EncodeToString(digest[:])
}

func workRoundFixture(t *testing.T) RuntimeImplementationWorkRound {
	t.Helper()
	ledger, _ := implementationWorkLedgerFixture(t)
	return ledger.Rounds[0]
}

func compareJSONValueHash(t *testing.T, value any) {
	t.Helper()
	before, beforeErr := json.Marshal(value)
	for _, prefix := range []string{"", "original:", "原文\x00\xff:"} {
		if got, want := implementationWorkRawHash(prefix, value), referenceJSONValueHash(prefix, value); got != want {
			t.Fatal("original JSON/digest differs")
		}
	}
	after, afterErr := json.Marshal(value)
	if cwiFocusBindingsError(beforeErr) != cwiFocusBindingsError(afterErr) || !bytes.Equal(before, after) {
		t.Fatal("hash changed original input")
	}
}

func TestJSONValueHashRetainsOriginalFullMarshal(t *testing.T) {
	for _, value := range []any{nil, true, 42, "<>&\u2028\u2029\n\x00\xff", []string(nil), []string{}, []string{"", "原文"}, map[string]string{"b": "second", "a": "first"}, json.RawMessage(" \n{\"原文\":\"<>&\"}\t"), math.NaN(), make(chan int), json.RawMessage("{broken")} {
		compareJSONValueHash(t, value)
	}
	for _, text := range []string{"", strings.Repeat("s", 120), strings.Repeat("s", 121), strings.Repeat("long 原文<>&\x00", 8192)} {
		round := workRoundFixture(t)
		round.StartupInputHash = text
		compareJSONValueHash(t, round)
		body, err := json.Marshal(round)
		var reload RuntimeImplementationWorkRound
		if err != nil || json.Unmarshal(body, &reload) != nil {
			t.Fatal("fixture reload failed")
		}
		compareJSONValueHash(t, reload)
	}
}

func TestJSONValueHashWriterRetainsAllChunkedBytes(t *testing.T) {
	for _, body := range []string{"null\n", "\"内部\\n原文\"\n", strings.Repeat("x", 4096) + "\n"} {
		for _, size := range []int{1, 2, 7, 120, 121, 4096} {
			w := runtimeJSONHashWriter{digest: sha256.New()}
			for at := 0; at < len(body); at += size {
				end := min(at+size, len(body))
				n, err := w.Write([]byte(body[at:end]))
				if err != nil || n != end-at {
					t.Fatal("chunk write failed")
				}
				if n, err := w.Write(nil); n != 0 || err != nil {
					t.Fatal("empty chunk changed writer")
				}
			}
			want := sha256.Sum256([]byte(body[:len(body)-1]))
			if !w.hasTail || w.tail[0] != '\n' || !bytes.Equal(w.digest.Sum(nil), want[:]) {
				t.Fatal("chunk boundaries dropped or changed internal bytes")
			}
		}
	}
}

func FuzzJSONValueHashOriginalWorkRound(f *testing.F) {
	for _, text := range []string{"", "原文<>&\x00\xff", strings.Repeat("s", 121)} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 8192 {
			t.Skip()
		}
		round := workRoundFixture(t)
		round.StartupInputHash = text
		compareJSONValueHash(t, round)
	})
}

func TestJSONValueHashExplicitArchiveRetainsWorkIdentity(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if json.Unmarshal(body, &runs) != nil {
		t.Fatal("invalid archive")
	}
	for _, run := range runs {
		for _, round := range run.RuntimeState.ImplementationWork.Rounds {
			compareJSONValueHash(t, round)
			copy := round
			copy.Hash = ""
			if StableRuntimeImplementationWorkRoundHash(round) != referenceJSONValueHash("implementation-work-round:", copy) {
				t.Fatal("sealed round hash changed")
			}
			if round.WorkingBinding != nil {
				compareJSONValueHash(t, *round.WorkingBinding)
				binding := *round.WorkingBinding
				binding.Hash = ""
				if StableRuntimeImplementationWorkingBindingHash(*round.WorkingBinding) != referenceJSONValueHash("implementation-working-binding:", binding) {
					t.Fatal("sealed working binding hash changed")
				}
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only hash test changed archive")
	}
}

func BenchmarkJSONValueHashLargeRound(b *testing.B) {
	round := RuntimeImplementationWorkRound{StartupInputHash: strings.Repeat("原文 <>& ", 32768)}
	for _, original := range []bool{true, false} {
		name := "encoder"
		if original {
			name = "marshal"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if original {
					_ = referenceJSONValueHash("original:", round)
				} else {
					_ = implementationWorkRawHash("original:", round)
				}
			}
		})
	}
}

func BenchmarkJSONValueHashPublicArchiveRound(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		b.Fatal(err)
	}
	for _, run := range runs {
		if len(run.RuntimeState.ImplementationWork.Rounds) == 0 {
			b.Fatal("archive has no work round")
		}
		round := run.RuntimeState.ImplementationWork.Rounds[0]
		for _, original := range []bool{true, false} {
			name := "encoder"
			if original {
				name = "marshal"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if original {
						_ = referenceJSONValueHash("original:", round)
					} else {
						_ = implementationWorkRawHash("original:", round)
					}
				}
			})
		}
	}
}
