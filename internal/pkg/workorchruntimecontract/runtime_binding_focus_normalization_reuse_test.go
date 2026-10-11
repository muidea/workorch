package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func referenceCoverageBindingsForFocusValidate(focus CoverageFocus, definitionRevision string) error {
	seenRawIDs := make(map[string]struct{}, len(focus.CoverageEvidenceBindings))
	for _, binding := range focus.CoverageEvidenceBindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		id := strings.TrimSpace(binding.ID)
		if _, duplicate := seenRawIDs[id]; duplicate {
			return fmt.Errorf("coverage binding projection duplicates %q", id)
		}
		seenRawIDs[id] = struct{}{}
	}
	focus = focus.Normalized()
	definitionRevision = strings.TrimSpace(definitionRevision)
	if err := referenceRuntimeFocusEvidenceDeclarationValidate(focus); err != nil {
		return err
	}
	bindings := NormalizeCoverageEvidenceBindings(focus.CoverageEvidenceBindings)
	if len(bindings) != len(focus.CoverageWorkItemIDs) {
		return fmt.Errorf("coverage binding unavailable for focus %q: requires exactly one binding per coverage item", focus.ID)
	}
	expected := make(map[common.CoverageWorkItemID]struct{}, len(focus.CoverageWorkItemIDs))
	for _, id := range focus.CoverageWorkItemIDs {
		expected[id] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.FocusID != focus.ID || binding.DefinitionRevision != definitionRevision {
			return fmt.Errorf("coverage binding %q does not match focus/definition identity", binding.ID)
		}
		if !SameRuntimeVerificationPolicy(binding.VerificationPolicy, focus.VerificationPolicy) {
			return fmt.Errorf("coverage binding %q verification policy does not match focus authority", binding.ID)
		}
		if !sameStrings(referenceCoverageBindingEvidenceKinds(binding), focus.RequiredEvidenceKinds) {
			return fmt.Errorf("coverage binding %q proof kinds do not match focus requirements", binding.ID)
		}
		if !sameStrings(binding.TargetRefs, focus.TargetRefs) || binding.TargetSelectorRef != focus.TargetSelectorRef || binding.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef || binding.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef || !sameStrings(binding.TargetSelectorCandidateRefs, focus.TargetSelectorCandidateRefs) || !SameRuntimeProspectiveTargetAllOfConstraintRefs(binding.ProspectiveTargetAllOfConstraintRefs, focus.ProspectiveTargetAllOfConstraintRefs) || !sameStrings(binding.ProspectiveTargetRefs, focus.ProspectiveTargetRefs) {
			return fmt.Errorf("coverage binding %q target surface does not match focus authority", binding.ID)
		}
		if _, ok := expected[binding.CoverageWorkItemID]; !ok {
			return fmt.Errorf("coverage binding %q references coverage item outside focus", binding.ID)
		}
		if _, duplicate := seen[binding.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("coverage binding %q duplicates coverage item %s", binding.ID, binding.CoverageWorkItemID)
		}
		seen[binding.CoverageWorkItemID] = struct{}{}
	}
	return nil
}

func bindingFocusReuseFixture() CoverageFocus {
	binding := testCoverageEvidenceBinding()
	return CoverageFocus{ID: binding.FocusID, EvidenceDeclarationRef: binding.DeclarationRef, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}, TargetRefs: append([]string(nil), binding.TargetRefs...), RequiredEvidenceKinds: referenceCoverageBindingEvidenceKinds(binding)}
}

func assertBindingFocusReuseReference(t testing.TB, focus CoverageFocus, revision string) {
	t.Helper()
	before := cloneCoverageFocusRaw(focus)
	for range 2 {
		assertProofCollectionIdentityErrors(t, ValidateCoverageEvidenceBindingsForFocus(focus, revision), referenceCoverageBindingsForFocusValidate(focus, revision))
	}
	if !reflect.DeepEqual(focus, before) {
		t.Fatal("同次复用改写原 focus/binding")
	}
	body, err := json.Marshal(focus)
	if err != nil {
		t.Fatal(err)
	}
	var loaded CoverageFocus
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	assertProofCollectionIdentityErrors(t, ValidateCoverageEvidenceBindingsForFocus(loaded, revision), referenceCoverageBindingsForFocusValidate(loaded, revision))
}

func TestBindingFocusNormalizationReuseKeepsCompleteGateAndRejectionOrder(t *testing.T) {
	for _, scenario := range []string{"valid", "trim", "duplicate-binding", "duplicate-proof", "duplicate-check", "hash", "definition", "focus", "policy", "kinds", "target", "coverage", "cardinality", "wildcard", "nil", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			focus := bindingFocusReuseFixture()
			revision := "definition-main"
			switch scenario {
			case "trim":
				focus.ID = " " + focus.ID + " "
				revision = " " + revision + " "
			case "duplicate-binding":
				focus.CoverageEvidenceBindings = append(focus.CoverageEvidenceBindings, focus.CoverageEvidenceBindings[0].Clone())
			case "duplicate-proof":
				focus.CoverageEvidenceBindings[0].Proofs = append(focus.CoverageEvidenceBindings[0].Proofs, focus.CoverageEvidenceBindings[0].Proofs[1].Clone())
			case "duplicate-check":
				focus.CoverageEvidenceBindings[0].Proofs[1].RequiredCheckRefs = append(focus.CoverageEvidenceBindings[0].Proofs[1].RequiredCheckRefs, focus.CoverageEvidenceBindings[0].Proofs[1].RequiredCheckRefs[0])
			case "hash":
				focus.CoverageEvidenceBindings[0].Hash = "drift"
				focus.ID = "also-drift"
			case "definition":
				revision = "drift"
			case "focus":
				focus.ID = "drift"
			case "policy":
				focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 1}
			case "kinds":
				focus.RequiredEvidenceKinds = []string{EvidenceKindValidation}
			case "target":
				focus.TargetRefs = []string{"other.go"}
			case "coverage":
				focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"other"}
			case "cardinality":
				focus.CoverageWorkItemIDs = append(focus.CoverageWorkItemIDs, "other")
			case "wildcard":
				focus.EvidenceDeclarationRef = "declaration/*"
			case "nil":
				focus = CoverageFocus{}
			case "empty":
				focus = CoverageFocus{CoverageEvidenceBindings: []CoverageEvidenceBinding{}, CoverageWorkItemIDs: []common.CoverageWorkItemID{}}
			}
			assertBindingFocusReuseReference(t, focus, revision)
		})
	}
	focus := bindingFocusReuseFixture()
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main"); err != nil {
		t.Fatal(err)
	}
	focus.CoverageEvidenceBindings[0].Proofs[1].ArtifactRefs = []string{"later"}
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main"); err == nil {
		t.Fatal("后续 raw 变化借用先前成功")
	}
	assertBindingFocusReuseReference(t, focus, "definition-main")
}

func TestBindingFocusNormalizationReuseKeepsMultiBindingOrder(t *testing.T) {
	focus := bindingFocusReuseFixture()
	first := focus.CoverageEvidenceBindings[0]
	first.Proofs = first.Proofs[:1]
	first.Hash = StableCoverageEvidenceBindingHash(first)
	second := first.Clone()
	second.ID, second.CoverageWorkItemID = "binding/other", "other"
	second.Hash = StableCoverageEvidenceBindingHash(second)
	focus.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{first.CoverageWorkItemID, second.CoverageWorkItemID}
	focus.CoverageEvidenceBindings = []CoverageEvidenceBinding{second, first}
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main"); err != nil {
		t.Fatalf("多 binding 前像非法：%v", err)
	}
	assertBindingFocusReuseReference(t, focus, "definition-main")
	focus.CoverageEvidenceBindings[0].CoverageWorkItemID = first.CoverageWorkItemID
	focus.CoverageEvidenceBindings[0].Hash = StableCoverageEvidenceBindingHash(focus.CoverageEvidenceBindings[0])
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main"); err == nil || !strings.Contains(err.Error(), "duplicates coverage item") {
		t.Fatalf("重复 CWI 未由后续完整门禁拒绝：%v", err)
	}
	assertBindingFocusReuseReference(t, focus, "definition-main")
}

func TestBindingFocusNormalizationReuseRemovesOnlySecondCopy(t *testing.T) {
	focus := bindingFocusReuseFixture()
	original := testing.AllocsPerRun(20, func() { _ = referenceCoverageBindingsForFocusValidate(focus, "definition-main") })
	current := testing.AllocsPerRun(20, func() { _ = ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main") })
	if current >= original {
		t.Fatalf("同次复用分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzBindingFocusNormalizationReuseReference(f *testing.F) {
	f.Add("focus-main", "definition-main", "main.go", "validation", "check", false)
	f.Add(" ", "\x00", string([]byte{0xff, 0x80}), "unknown", "", true)
	f.Fuzz(func(t *testing.T, id, revision, target, kind, check string, duplicate bool) {
		focus := bindingFocusReuseFixture()
		focus.ID, focus.TargetRefs = id, []string{target, target}
		focus.CoverageEvidenceBindings[0].Proofs[1].EvidenceKind = kind
		focus.CoverageEvidenceBindings[0].Proofs[1].RequiredCheckRefs = []string{check}
		focus.CoverageEvidenceBindings[0].Hash = StableCoverageEvidenceBindingHash(focus.CoverageEvidenceBindings[0])
		if duplicate {
			focus.CoverageEvidenceBindings = append(focus.CoverageEvidenceBindings, focus.CoverageEvidenceBindings[0].Clone())
		}
		assertBindingFocusReuseReference(t, focus, revision)
	})
}

func BenchmarkBindingFocusNormalizationReuse(b *testing.B) {
	focus := bindingFocusReuseFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceCoverageBindingsForFocusValidate(focus, "definition-main")
				} else {
					_ = ValidateCoverageEvidenceBindingsForFocus(focus, "definition-main")
				}
			}
		})
	}
}
