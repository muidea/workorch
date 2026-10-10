package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 独立 reference 保留复用前的完整逐项路径，不调用新增的私有校验 helper。
func referenceCoverageBindingValidate(b CoverageEvidenceBinding) error {
	if err := ValidateRuntimeLineageAssertions(b.LineageAssertions, b.CoverageWorkItemID, b.DeclarationRef); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(b.Proofs))
	for _, proof := range b.Proofs {
		kind := strings.TrimSpace(proof.EvidenceKind)
		if kind == "" {
			return fmt.Errorf("coverage evidence binding %q has a proof without evidence kind", strings.TrimSpace(b.ID))
		}
		if _, duplicate := seen[kind]; duplicate {
			return fmt.Errorf("coverage evidence binding %q duplicates evidence proof %q", strings.TrimSpace(b.ID), kind)
		}
		seen[kind] = struct{}{}
		if err := proof.Validate(); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", strings.TrimSpace(b.ID), err)
		}
	}
	b = b.Normalized()
	if b.ID == "" || b.CoverageWorkItemID == "" || b.FocusID == "" || b.DefinitionRevision == "" || b.DeclarationRef == "" {
		return fmt.Errorf("coverage evidence binding requires identity, coverage item, focus, definition and declaration")
	}
	if len(b.Proofs) == 0 {
		return fmt.Errorf("coverage evidence binding %q requires at least one evidence proof", b.ID)
	}
	if b.VerificationPolicy != nil {
		if err := b.VerificationPolicy.Validate(); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", b.ID, err)
		}
	}
	if len(b.TargetRefs) == 0 && b.TargetSelectorRef == "" && len(b.ProspectiveTargetRefs) == 0 {
		return fmt.Errorf("coverage evidence binding %q requires an exact target, selector or prospective target", b.ID)
	}
	if b.TargetSelectorRef != "" && (b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "") {
		return fmt.Errorf("coverage evidence binding %q selector requires owner and subject", b.ID)
	}
	if len(b.TargetSelectorCandidateRefs) != 0 && b.TargetSelectorRef == "" {
		return fmt.Errorf("coverage evidence binding %q selector candidates require a selector", b.ID)
	}
	if len(b.ProspectiveTargetAllOfConstraintRefs) != 0 {
		if b.TargetSelectorRef == "" || b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "" {
			return fmt.Errorf("coverage evidence binding %q prospective target all-of declaration requires a complete selector", b.ID)
		}
		if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(b.ProspectiveTargetAllOfConstraintRefs); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", b.ID, err)
		}
	}
	if len(b.ProspectiveTargetRefs) != 0 {
		if len(b.TargetRefs) != 0 || b.TargetSelectorRef == "" || b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "" {
			return fmt.Errorf("coverage evidence binding %q prospective targets require only a complete selector target surface", b.ID)
		}
		if len(b.ProspectiveTargetRefs) > RuntimeProspectiveTargetRefsMaxCount {
			return fmt.Errorf("coverage evidence binding %q prospective target set exceeds %d", b.ID, RuntimeProspectiveTargetRefsMaxCount)
		}
		for _, target := range b.ProspectiveTargetRefs {
			if len(b.TargetSelectorCandidateRefs) != 0 && !containsString(b.TargetSelectorCandidateRefs, target) {
				return fmt.Errorf("coverage evidence binding %q prospective target %q is outside selector candidates", b.ID, target)
			}
		}
	}
	if b.Hash != "" && b.Hash != stableCoverageEvidenceBindingHashNormalized(b) {
		return fmt.Errorf("coverage evidence binding %q hash does not match contents", b.ID)
	}
	return nil
}

func referenceCoverageBindingProjectionValidate(values []CoverageEvidenceBinding) error {
	if !reflect.DeepEqual(values, NormalizeCoverageEvidenceBindings(values)) {
		return fmt.Errorf("coverage evidence binding projection is not canonical")
	}
	seen := make(map[string]struct{}, len(values))
	for _, binding := range values {
		if _, duplicate := seen[binding.ID]; duplicate {
			return fmt.Errorf("coverage evidence binding projection duplicates %q", binding.ID)
		}
		seen[binding.ID] = struct{}{}
		if err := referenceCoverageBindingValidate(binding); err != nil {
			return fmt.Errorf("coverage evidence binding projection %q: %w", binding.ID, err)
		}
	}
	return nil
}

func assertCoverageBindingValidationReference(t *testing.T, values []CoverageEvidenceBinding) {
	t.Helper()
	before, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, want := ValidateCoverageEvidenceBindingProjection(values), referenceCoverageBindingProjectionValidate(values)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("projection 拒绝原因改变: got=%v want=%v", got, want)
		}
		for _, value := range values {
			got, want := value.Validate(), referenceCoverageBindingValidate(value)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("公开 binding 校验改变: got=%v want=%v", got, want)
			}
		}
	}
	after, err := json.Marshal(values)
	if err != nil || string(before) != string(after) {
		t.Fatal("校验修改原始 binding 正文")
	}
}

func TestCoverageBindingProjectionValidationReuseMatchesReference(t *testing.T) {
	base := testCoverageEvidenceBinding()
	for _, test := range []struct {
		name   string
		mutate func(*CoverageEvidenceBinding)
	}{
		{"valid", func(*CoverageEvidenceBinding) {}},
		{"identity", func(b *CoverageEvidenceBinding) { b.CoverageWorkItemID = "" }},
		{"no proof", func(b *CoverageEvidenceBinding) { b.Proofs = nil }},
		{"blank proof", func(b *CoverageEvidenceBinding) { b.Proofs[0].EvidenceKind = "" }},
		{"duplicate proof", func(b *CoverageEvidenceBinding) { b.Proofs = append(b.Proofs, b.Proofs[0]) }},
		{"duplicate check", func(b *CoverageEvidenceBinding) {
			b.Proofs[1].RequiredCheckRefs = append(b.Proofs[1].RequiredCheckRefs, b.Proofs[1].RequiredCheckRefs[0])
		}},
		{"wrong effect", func(b *CoverageEvidenceBinding) {
			b.Proofs[0].RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectEphemeralSession}
		}},
		{"missing attestation", func(b *CoverageEvidenceBinding) { b.Proofs[1].ValidationCheckAttestations = nil }},
		{"no target", func(b *CoverageEvidenceBinding) { b.TargetRefs = nil }},
		{"selector", func(b *CoverageEvidenceBinding) { b.TargetSelectorRef = "selector" }},
		{"candidate without selector", func(b *CoverageEvidenceBinding) { b.TargetSelectorCandidateRefs = []string{"candidate"} }},
		{"constraint without selector", func(b *CoverageEvidenceBinding) { b.ProspectiveTargetAllOfConstraintRefs = []string{"constraint"} }},
		{"prospective with target", func(b *CoverageEvidenceBinding) { b.ProspectiveTargetRefs = []string{"prospective"} }},
		{"invalid policy", func(b *CoverageEvidenceBinding) {
			b.VerificationPolicy = &RuntimeVerificationPolicy{Version: "unknown"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := base.Clone()
			test.mutate(&value)
			value.Hash = StableCoverageEvidenceBindingHash(value)
			// 自洽 hash 也不能代替原始 proof/body 的完整验证。
			assertCoverageBindingValidationReference(t, []CoverageEvidenceBinding{value})
			if got := ValidateCoverageEvidenceBindingProjection([]CoverageEvidenceBinding{value}); (got == nil) != (test.name == "valid") {
				t.Fatalf("独立场景预期改变: %s: %v", test.name, got)
			}
			var reloaded CoverageEvidenceBinding
			data, _ := json.Marshal(value)
			if err := json.Unmarshal(data, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertCoverageBindingValidationReference(t, []CoverageEvidenceBinding{reloaded})
		})
	}
	for _, values := range [][]CoverageEvidenceBinding{nil, {}, {base, base}} {
		assertCoverageBindingValidationReference(t, values)
	}
	values := []CoverageEvidenceBinding{base.Clone()}
	if err := ValidateCoverageEvidenceBindingProjection(values); err != nil {
		t.Fatal(err)
	}
	values[0].Proofs[1].ValidationCheckAttestations[0].NativeCheckRef = "changed-after-success"
	assertCoverageBindingValidationReference(t, values)
	if err := ValidateCoverageEvidenceBindingProjection(values); err == nil {
		t.Fatal("先前成功掩盖后续原始正文漂移")
	}
	values[0].Hash = StableCoverageEvidenceBindingHash(values[0])
	assertCoverageBindingValidationReference(t, values)
}

func FuzzCoverageBindingProjectionValidationReuse(f *testing.F) {
	data, _ := json.Marshal([]CoverageEvidenceBinding{testCoverageEvidenceBinding()})
	f.Add(data)
	f.Add([]byte("null"))
	f.Add([]byte("[]"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var values []CoverageEvidenceBinding
		if err := json.Unmarshal(data, &values); err != nil {
			return
		}
		assertCoverageBindingValidationReference(t, values)
	})
}

func BenchmarkCoverageBindingProjectionValidationReuse(b *testing.B) {
	values := []CoverageEvidenceBinding{testCoverageEvidenceBinding()}
	for _, test := range []struct {
		name     string
		validate func([]CoverageEvidenceBinding) error
	}{{"reference", referenceCoverageBindingProjectionValidate}, {"reused", ValidateCoverageEvidenceBindingProjection}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := test.validate(values); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
