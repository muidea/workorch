package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"strings"
	"sync"
	"testing"
)

func assertRuntimeHashWriterEncoding(t testing.TB, values []string) {
	t.Helper()
	actual, reference := newRuntimeHashWriter(), sha256.New()
	for range 2 {
		writeHashStrings(actual, values)
		var count [8]byte
		binary.BigEndian.PutUint64(count[:], uint64(len(values)))
		reference.Write(count[:])
		for _, value := range values {
			runtimeWriteHashStringReference(reference, value)
		}
		if !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
			t.Fatal("局部缓冲改变原集合 cardinality 或完整原始 bytes hash")
		}
		// 混合普通 Write 与短／长字段；Sum 不得消费或改变后续编码状态。
		actual.Write([]byte("direct\x00"))
		reference.Write([]byte("direct\x00"))
		writeHashString(actual, "原文\r\n")
		runtimeWriteHashStringReference(reference, "原文\r\n")
		if !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
			t.Fatal("Write/Sum 混合编码改变 identity")
		}
	}
	actual.Reset()
	reference.Reset()
	writeHashString(actual, "reset")
	runtimeWriteHashStringReference(reference, "reset")
	if !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
		t.Fatal("Reset 保留前次缓冲正文")
	}
}

func TestRuntimeHashWriterPreservesFullReferenceEncoding(t *testing.T) {
	for _, values := range [][]string{nil, {}, {""}, {"A", "B"}, {"AB", ""}, {" 原文\r\n ", "A\x00B", string([]byte{0xff, 0x80})}, {
		strings.Repeat("a", 120), strings.Repeat("b", 121), "", strings.Repeat("原", 32768), "short",
	}} {
		assertRuntimeHashWriterEncoding(t, values)
	}
	var workers sync.WaitGroup
	for index := range 16 {
		workers.Go(func() {
			for range 16 {
				assertRuntimeHashWriterEncoding(t, []string{strings.Repeat("x", index+112), "短字段", ""})
			}
		})
	}
	workers.Wait()
}

func TestRuntimeHashWriterCoverageBindingMatchesFullReference(t *testing.T) {
	_, lineage := lineageSourceFixture(t, RuntimeLineageAssertionRule{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery})
	if got, want := StableCoverageEvidenceBindingHash(lineage), referenceRuntimeBufferedCoverageBindingHash(lineage.Normalized()); got != want {
		t.Fatal("lineage 扩展的完整 binding hash 编码发生漂移")
	}
	for _, preparing := range []bool{false, true} {
		catalog := runtimeOwnerCatalogSurfaceReuseFixture(preparing)
		for _, surface := range catalog.ValidationSurfaces {
			binding := CoverageEvidenceBinding{
				ID: "binding", CoverageWorkItemID: "cwi", FocusID: "focus", DefinitionRevision: "definition",
				DeclarationRef: " 原判据\r\n ", TargetRefs: []string{"source.txt"}, TargetSelectorCandidateRefs: []string{strings.Repeat("原", 1024)},
				Proofs: []CoverageEvidenceProof{{
					EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
					AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: surface.ID,
					RequiredCheckRefs: []string{"check-A", "check-B"}, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check-A", ObservationProtocolRef: "protocol", NativeCheckRef: "native-A"}},
				}},
			}.Normalized()
			if got, want := stableCoverageEvidenceBindingHashNormalized(binding), referenceRuntimeBufferedCoverageBindingHash(binding); got != want {
				t.Fatal("完整 binding hash 编码发生漂移")
			}
		}
	}
}

func FuzzRuntimeHashWriterFullEncoding(f *testing.F) {
	for _, value := range []string{"", " 原文\r\n ", "A\x00B", string([]byte{0xff, 0x80}), strings.Repeat("x", 120), strings.Repeat("y", 121)} {
		f.Add(value, "short")
	}
	f.Fuzz(func(t *testing.T, first, second string) {
		assertRuntimeHashWriterEncoding(t, []string{first, "", second})
	})
}

func BenchmarkRuntimeHashWriterShortFields(b *testing.B) {
	values := []string{"runtime-coverage-evidence-binding", "cwi-source", "focus-source", "definition-v2", "state_change", "workspace_durable", "file/edit", "", "source.txt", "declaration/source"}
	for _, mode := range []string{"reference", "single_write", "call_local"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var h hash.Hash
				if mode == "call_local" {
					h = newRuntimeHashWriter()
				} else {
					h = sha256.New()
				}
				for _, value := range values {
					if mode == "reference" {
						runtimeWriteHashStringReference(h, value)
					} else {
						writeHashString(h, value)
					}
				}
				if len(h.Sum(nil)) != sha256.Size {
					b.Fatal("invalid hash")
				}
			}
		})
	}
}

func TestRuntimeHashWriterBlockBoundariesAndHashInterface(t *testing.T) {
	for _, size := range []int{0, 1, 7, 8, 119, 120, 121, 247, 248, 249, 255, 256, 257, 511, 512, 4095, 4096, 8191, 8192, 8193} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			actual, reference := newRuntimeHashWriter(), sha256.New()
			if actual.Size() != reference.Size() || actual.BlockSize() != reference.BlockSize() {
				t.Fatal("hash interface size drift")
			}
			for round := range 4 {
				value := strings.Repeat("x", size) + string([]byte{0xff, 0x00})
				if round%2 == 0 {
					writeHashString(actual, value)
					runtimeWriteHashStringReference(reference, value)
				} else {
					n, err := actual.Write([]byte(value))
					reference.Write([]byte(value))
					if n != len(value) || err != nil {
						t.Fatal("Write contract drift")
					}
				}
				prefix := make([]byte, 3, 512)
				copy(prefix, "pre")
				want := reference.Sum(append([]byte(nil), prefix...))
				if got := actual.Sum(prefix); !bytes.Equal(got, want) || !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
					t.Fatal("Sum changed complete bytes or destination prefix")
				}
				if round == 2 {
					actual.Reset()
					reference.Reset()
				}
			}
			// Reset discards still-buffered bytes without requiring Sum first.
			writeHashString(actual, "discard")
			actual.Reset()
			reference.Reset()
			actual.Write(nil)
			reference.Write(nil)
			if !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
				t.Fatal("Reset retained pending bytes")
			}
		})
	}
}

func BenchmarkRuntimeHashWriterFieldBatch(b *testing.B) {
	for _, count := range []int{10, 256, 2048} {
		values := make([]string, count)
		for i := range values {
			values[i] = fmt.Sprintf("source-contract-%04d/原文", i)
		}
		for _, mode := range []string{"prior", "buffered"} {
			b.Run(fmt.Sprintf("fields_%d/%s", count, mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "prior" {
						w := newReferenceFieldRuntimeHashWriter()
						for _, v := range values {
							w.writeString(v)
						}
						w.Sum(nil)
					} else {
						w := newRuntimeHashWriter()
						for _, v := range values {
							w.writeString(v)
						}
						w.Sum(nil)
					}
				}
			})
		}
	}
}
