package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func referenceCoverageBindingEvidenceKinds(binding CoverageEvidenceBinding) []string {
	binding = binding.Normalized()
	ret := make([]string, 0, len(binding.Proofs))
	for _, proof := range binding.Proofs {
		ret = append(ret, proof.EvidenceKind)
	}
	return normalizeStrings(ret)
}

func assertCoverageBindingEvidenceKindsReference(t testing.TB, binding CoverageEvidenceBinding) {
	t.Helper()
	before := cloneCoverageEvidenceBindingRaw(binding)
	want, got := referenceCoverageBindingEvidenceKinds(binding), binding.EvidenceKinds()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("proof kind 集合漂移：got=%q want=%q", got, want)
	}
	if len(got) != 0 {
		got[0] = "changed-return"
		if !reflect.DeepEqual(binding.EvidenceKinds(), want) {
			t.Fatal("返回集合污染后续读取")
		}
	}
	if !reflect.DeepEqual(binding, before) {
		t.Fatal("kind 投影改写原 binding 或嵌套 proof")
	}
	body, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var loaded CoverageEvidenceBinding
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.EvidenceKinds(), referenceCoverageBindingEvidenceKinds(loaded)) {
		t.Fatal("JSON 重载后 kind 投影漂移")
	}
}

func TestCoverageBindingEvidenceKindsKeepsOriginalSetAndIsolation(t *testing.T) {
	for _, kinds := range [][]string{
		nil, {}, {"", " ", "\t\n"},
		{EvidenceKindValidation, EvidenceKindStateChange},
		{" validation ", "state_change", "validation", "", "unknown"},
		{string([]byte{0xff, 0x80}), "\x00", "中文\r\n", strings.Repeat("长", 4096)},
	} {
		binding := testCoverageEvidenceBinding()
		binding.Proofs = nil
		for _, kind := range kinds {
			binding.Proofs = append(binding.Proofs, CoverageEvidenceProof{EvidenceKind: kind, RequiredCheckRefs: []string{" check ", "check"}, ArtifactRefs: []string{" artifact "}})
		}
		assertCoverageBindingEvidenceKindsReference(t, binding)
		binding.Proofs = append(binding.Proofs, CoverageEvidenceProof{EvidenceKind: "later"})
		assertCoverageBindingEvidenceKindsReference(t, binding)
		binding.Proofs[len(binding.Proofs)-1].EvidenceKind = " "
		assertCoverageBindingEvidenceKindsReference(t, binding)
	}
	assertCoverageBindingEvidenceKindsReference(t, CoverageEvidenceBinding{Proofs: []CoverageEvidenceProof{}})
}

func TestCoverageBindingEvidenceKindsDoesNotReplaceCompleteProofGate(t *testing.T) {
	for _, scenario := range []string{"valid", "duplicate", "duplicate-check", "unknown", "hash"} {
		t.Run(scenario, func(t *testing.T) {
			binding := testCoverageEvidenceBinding()
			switch scenario {
			case "duplicate":
				binding.Proofs = append(binding.Proofs, binding.Proofs[1].Clone())
			case "duplicate-check":
				binding.Proofs[1].RequiredCheckRefs = append(binding.Proofs[1].RequiredCheckRefs, binding.Proofs[1].RequiredCheckRefs[0])
			case "unknown":
				binding.Proofs[1].EvidenceKind = "unknown"
			}
			binding.Hash = StableCoverageEvidenceBindingHash(binding)
			if scenario == "hash" {
				binding.Hash = "drift"
			}
			before := cloneCoverageEvidenceBindingRaw(binding)
			originalError := binding.Validate()
			if (originalError == nil) != (scenario == "valid") {
				t.Fatalf("完整 binding fixture 未命中：%v", originalError)
			}
			assertCoverageBindingEvidenceKindsReference(t, binding)
			assertProofCollectionIdentityErrors(t, binding.Validate(), originalError)
			if !reflect.DeepEqual(binding, before) {
				t.Fatal("查询改变完整门禁前像")
			}
		})
	}
}

func TestCoverageBindingEvidenceKindsRemovesOnlyDiscardedBindingCopies(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	original := testing.AllocsPerRun(20, func() { _ = referenceCoverageBindingEvidenceKinds(binding) })
	current := testing.AllocsPerRun(20, func() { _ = binding.EvidenceKinds() })
	if current >= original {
		t.Fatalf("kind 查询分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzCoverageBindingEvidenceKindsReference(f *testing.F) {
	f.Add(" validation ", "state_change", "", "check")
	f.Add(string([]byte{0xff, 0x80}), "\x00", " ", "中文\r\n")
	f.Fuzz(func(t *testing.T, first, second, third, check string) {
		binding := testCoverageEvidenceBinding()
		binding.Proofs = []CoverageEvidenceProof{
			{EvidenceKind: first, RequiredCheckRefs: []string{check, check}},
			{EvidenceKind: second}, {EvidenceKind: third}, {EvidenceKind: first},
		}
		assertCoverageBindingEvidenceKindsReference(t, binding)
	})
}

func BenchmarkCoverageBindingEvidenceKinds(b *testing.B) {
	binding := testCoverageEvidenceBinding()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceCoverageBindingEvidenceKinds(binding)
				} else {
					_ = binding.EvidenceKinds()
				}
			}
		})
	}
}
