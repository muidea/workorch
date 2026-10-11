package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func assertFocusDeclarationProjectionReference(t testing.TB, focus CoverageFocus) {
	t.Helper()
	before := cloneCoverageFocusRaw(focus)
	assertProofCollectionIdentityErrors(t, ValidateRuntimeFocusEvidenceDeclaration(focus), referenceRuntimeFocusEvidenceDeclarationValidate(focus))
	if !reflect.DeepEqual(focus, before) {
		t.Fatal("声明门禁改写原 focus")
	}
	body, err := json.Marshal(focus)
	if err != nil {
		t.Fatal(err)
	}
	var loaded CoverageFocus
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	assertProofCollectionIdentityErrors(t, ValidateRuntimeFocusEvidenceDeclaration(loaded), referenceRuntimeFocusEvidenceDeclarationValidate(loaded))
}

func TestFocusDeclarationProjectionKeepsOriginalGateAndRawValues(t *testing.T) {
	for _, scenario := range []string{"valid", "trim", "empty", "blank-cwi", "duplicate-cwi", "multiple-cwi", "wildcard", "verification", "owner", "convergence", "incomplete-owner", "unknown-evidence", "blank-artifact"} {
		t.Run(scenario, func(t *testing.T) {
			focus := independentBindingPredicateFixture()
			focus.RequiredEvidenceKinds = []string{" validation "}
			switch scenario {
			case "trim":
				focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: " " + RuntimeVerificationPolicyVersion + " "}
			case "empty":
				focus = CoverageFocus{}
			case "blank-cwi":
				focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"", " "}
			case "duplicate-cwi":
				focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{" cwi ", "cwi", ""}
			case "multiple-cwi":
				focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi", "other"}
			case "wildcard":
				focus.EvidenceDeclarationRef = " declaration/* "
			case "verification":
				focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: "invalid", MinimumEvidenceLevel: -1}
			case "owner":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "unknown", FactKind: "unknown", OperationKind: "unknown"}}
			case "convergence":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}}
			case "incomplete-owner":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "owner", FactKind: "kind"}}
			case "unknown-evidence":
				focus.RequiredEvidenceKinds = []string{"unknown"}
			case "blank-artifact":
				focus.RequiredEvidenceKinds, focus.ArtifactRefs = nil, []string{" ", "\t"}
			}
			assertFocusDeclarationProjectionReference(t, focus)
			focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"later", "other"}
			assertFocusDeclarationProjectionReference(t, focus)
		})
	}
}

func TestFocusDeclarationProjectionLeavesBindingValidationAuthorityIntact(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	focus := CoverageFocus{ID: binding.FocusID, EvidenceDeclarationRef: binding.DeclarationRef, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}, TargetRefs: binding.TargetRefs, RequiredEvidenceKinds: referenceCoverageBindingEvidenceKinds(binding)}
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, binding.DefinitionRevision); err != nil {
		t.Fatalf("完整前像非法：%v", err)
	}
	focus.CoverageEvidenceBindings[0].Proofs = append(focus.CoverageEvidenceBindings[0].Proofs, binding.Proofs[1].Clone())
	focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: "invalid"}
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, binding.DefinitionRevision); err == nil || !strings.Contains(err.Error(), "duplicates evidence proof") {
		t.Fatalf("完整 raw proof 拒绝必须优先：%v", err)
	}
	assertFocusDeclarationProjectionReference(t, focus)
	focus.CoverageEvidenceBindings[0] = binding
	focus.VerificationPolicy = nil
	focus.CoverageEvidenceBindings[0].Hash = "drift"
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, binding.DefinitionRevision); err == nil || !strings.Contains(err.Error(), "hash does not match") {
		t.Fatalf("后续原 binding hash 漂移未拒绝：%v", err)
	}
}

func TestFocusDeclarationProjectionRemovesOnlyUnusedNormalization(t *testing.T) {
	focus := independentBindingPredicateFixture()
	original := testing.AllocsPerRun(20, func() { _ = referenceRuntimeFocusEvidenceDeclarationValidate(focus) })
	current := testing.AllocsPerRun(20, func() { _ = ValidateRuntimeFocusEvidenceDeclaration(focus) })
	if current >= original {
		t.Fatalf("声明门禁仍复制无关 focus：current=%v original=%v", current, original)
	}
}

func FuzzFocusDeclarationProjectionReference(f *testing.F) {
	f.Add(" focus ", " cwi ", "", " declaration/* ", " validation ", " ", "", false)
	f.Add("\x00", string([]byte{0xff, 0x80}), "other", "declaration", "unknown", "artifact", RuntimeVerificationPolicyVersion, true)
	f.Fuzz(func(t *testing.T, id, first, second, declaration, evidence, artifact, policy string, behavior bool) {
		focus := independentBindingPredicateFixture()
		focus.ID, focus.EvidenceDeclarationRef = id, declaration
		focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{common.CoverageWorkItemID(first), common.CoverageWorkItemID(second), common.CoverageWorkItemID(first)}
		focus.RequiredEvidenceKinds, focus.ArtifactRefs = []string{evidence}, []string{artifact}
		focus.RequireExecutableBehaviorEvidence = behavior
		if policy != "" {
			focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: policy}
		}
		assertFocusDeclarationProjectionReference(t, focus)
	})
}

func BenchmarkFocusDeclarationProjection(b *testing.B) {
	focus := independentBindingPredicateFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceRuntimeFocusEvidenceDeclarationValidate(focus)
				} else {
					_ = ValidateRuntimeFocusEvidenceDeclaration(focus)
				}
			}
		})
	}
}
