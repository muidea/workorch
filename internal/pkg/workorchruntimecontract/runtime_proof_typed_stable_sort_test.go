package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func referenceCoverageEvidenceProofsStableSort(values []CoverageEvidenceProof) []CoverageEvidenceProof {
	ret := make([]CoverageEvidenceProof, 0, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.EvidenceKind == "" {
			continue
		}
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].EvidenceKind < ret[j].EvidenceKind })
	return nilIfEmpty(ret)
}

func assertProofStableSortReference(t testing.TB, values []CoverageEvidenceProof) {
	t.Helper()
	before := cloneRuntimeStateRawSlice(values, cloneCoverageEvidenceProofRaw)
	want := referenceCoverageEvidenceProofsStableSort(values)
	got := NormalizeCoverageEvidenceProofs(values)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("typed stable sort 改变完整 proof 顺序或表示")
	}
	for _, proof := range got {
		if len(proof.ArtifactRefs) != 0 {
			proof.ArtifactRefs[0] = "changed-return"
		}
		if len(proof.AllowedOperationKinds) != 0 {
			proof.AllowedOperationKinds[0] = "changed-return"
		}
		if len(proof.ValidationCheckAttestations) != 0 {
			proof.ValidationCheckAttestations[0].NativeCheckRef = "changed-return"
		}
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("typed sort 返回值污染原 proof")
	}
	if !reflect.DeepEqual(NormalizeCoverageEvidenceProofs(values), want) {
		t.Fatal("返回副本污染后续规范化")
	}
	body, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []CoverageEvidenceProof
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(NormalizeCoverageEvidenceProofs(loaded), referenceCoverageEvidenceProofsStableSort(loaded)) {
		t.Fatal("重载后 stable sort 漂移")
	}
}

func proofStableSortFixture(kinds []string) []CoverageEvidenceProof {
	var ret []CoverageEvidenceProof
	for index, kind := range kinds {
		ret = append(ret, CoverageEvidenceProof{EvidenceKind: kind, AllowedOperationKinds: []string{" kind ", "kind"}, ArtifactRefs: []string{fmt.Sprintf("ordinal-%03d", index)}, RequiredCheckRefs: []string{" b ", "a", "a"}, ValidationCheckAttestations: runtimeValidationCheckAttestationsForTest([]string{"a"})})
	}
	return ret
}

func TestProofTypedStableSortKeepsOriginalOrderAndIsolation(t *testing.T) {
	for _, kinds := range [][]string{nil, {}, {""}, {"validation"}, {"validation", "state_change"}, {"validation", " validation "}, {"state_change", "validation"}, {"validation", "state_change", " validation ", "unknown", "", "state_change"}, {"\x00", string([]byte{0xff, 0x80}), "中文", " "}} {
		assertProofStableSortReference(t, proofStableSortFixture(kinds))
	}
	assertProofStableSortReference(t, []CoverageEvidenceProof{})
	kinds := make([]string, 128)
	for index := range kinds {
		kinds[index] = fmt.Sprintf("kind-%d", (127-index)%7)
	}
	values := proofStableSortFixture(kinds)
	assertProofStableSortReference(t, values)
	values[0].EvidenceKind = "later"
	assertProofStableSortReference(t, values)
}

func TestProofTypedStableSortKeepsDuplicateRawAuthority(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	binding.Proofs = append(binding.Proofs, binding.Proofs[1].Clone())
	if err := binding.Validate(); err == nil {
		t.Fatal("duplicate raw proof 不得因稳定排序通过")
	}
	assertProofStableSortReference(t, binding.Proofs)
	if len(NormalizeCoverageEvidenceProofs(binding.Proofs)) != len(binding.Proofs) {
		t.Fatal("排序去除了重复 proof")
	}
}

func TestProofTypedStableSortRemovesOnlyReflectionAllocations(t *testing.T) {
	values := proofStableSortFixture([]string{"validation", "state_change", "validation"})
	original := testing.AllocsPerRun(20, func() { _ = referenceCoverageEvidenceProofsStableSort(values) })
	current := testing.AllocsPerRun(20, func() { _ = NormalizeCoverageEvidenceProofs(values) })
	if current >= original {
		t.Fatalf("反射排序分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzProofTypedStableSortReference(f *testing.F) {
	f.Add(" validation ", "state_change", "", uint8(4))
	f.Add(string([]byte{0xff, 0x80}), "\x00", "中文", uint8(64))
	f.Fuzz(func(t *testing.T, first, second, third string, count uint8) {
		kinds := make([]string, int(count))
		for index := range kinds {
			kinds[index] = []string{first, second, third}[index%3]
		}
		assertProofStableSortReference(t, proofStableSortFixture(kinds))
	})
}

func BenchmarkProofTypedStableSort(b *testing.B) {
	for _, count := range []int{2, 32} {
		kinds := make([]string, count)
		for index := range kinds {
			kinds[index] = fmt.Sprintf("kind-%d", (count-index)%3)
		}
		values := proofStableSortFixture(kinds)
		for _, mode := range []string{"reference", "current"} {
			b.Run(fmt.Sprintf("%s/%d", mode, count), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "reference" {
						_ = referenceCoverageEvidenceProofsStableSort(values)
					} else {
						_ = NormalizeCoverageEvidenceProofs(values)
					}
				}
			})
		}
	}
}
