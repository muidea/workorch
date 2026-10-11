package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus CoverageFocus) bool {
	focus = focus.Normalized()
	return containsEvidenceKind(focus.RequiredEvidenceKinds, EvidenceKindValidation) || focus.RequireExecutableBehaviorEvidence || len(focus.RequiredOwnerFacts) != 0 || len(focus.MetricConstraints) != 0 || len(focus.ArtifactRefs) != 0
}

func referenceRuntimeFocusEvidenceDeclarationValidate(focus CoverageFocus) error {
	focus = focus.Normalized()
	if focus.VerificationPolicy != nil {
		if err := focus.VerificationPolicy.Validate(); err != nil {
			return fmt.Errorf("coverage focus %q: %w", focus.ID, err)
		}
	}
	if err := ValidateRuntimeOwnerFactRequirements(focus.RequiredOwnerFacts); err != nil {
		return fmt.Errorf("coverage focus %q owner facts: %w", focus.ID, err)
	}
	if err := ValidateRuntimeOwnerFactConvergences(focus.RequiredOwnerFacts, focus.OwnerFactConvergences); err != nil {
		return fmt.Errorf("coverage focus %q owner-fact convergence: %w", focus.ID, err)
	}
	if !referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		return nil
	}
	if len(focus.CoverageWorkItemIDs) != 1 {
		return fmt.Errorf("coverage binding unavailable for focus %q: semantic proof must be bound to exactly one CWI, got %d", focus.ID, len(focus.CoverageWorkItemIDs))
	}
	if strings.HasSuffix(focus.EvidenceDeclarationRef, "/*") {
		return fmt.Errorf("coverage binding unavailable for focus %q: semantic proof requires a CWI-specific declaration; wildcard declaration %q is a quality gate and cannot close coverage", focus.ID, focus.EvidenceDeclarationRef)
	}
	return nil
}

func assertIndependentBindingPredicateReference(t testing.TB, focus CoverageFocus) {
	t.Helper()
	before := cloneCoverageFocusRaw(focus)
	want := referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus)
	for range 2 {
		if got := RuntimeFocusRequiresIndependentEvidenceBinding(focus); got != want {
			t.Fatalf("independent-binding 谓词漂移：got=%v want=%v", got, want)
		}
		assertProofCollectionIdentityErrors(t, ValidateRuntimeFocusEvidenceDeclaration(focus), referenceRuntimeFocusEvidenceDeclarationValidate(focus))
	}
	if !reflect.DeepEqual(focus, before) {
		t.Fatal("只读谓词改写原 focus")
	}
	body, err := json.Marshal(focus)
	if err != nil {
		t.Fatal(err)
	}
	var loaded CoverageFocus
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if RuntimeFocusRequiresIndependentEvidenceBinding(loaded) != referenceRuntimeFocusRequiresIndependentEvidenceBinding(loaded) {
		t.Fatal("重载后谓词漂移")
	}
	assertProofCollectionIdentityErrors(t, ValidateRuntimeFocusEvidenceDeclaration(loaded), referenceRuntimeFocusEvidenceDeclarationValidate(loaded))
}

func independentBindingPredicateFixture() CoverageFocus {
	binding := CoverageEvidenceBinding{ID: "binding", CoverageWorkItemID: "cwi", FocusID: "focus", DefinitionRevision: "definition", DeclarationRef: "declaration", Proofs: []CoverageEvidenceProof{{EvidenceKind: EvidenceKindValidation, ValidationRecipeID: "recipe", RequiredCheckRefs: []string{" b ", "a", "a"}}}}
	return CoverageFocus{ID: " focus ", CoverageWorkItemIDs: []common.CoverageWorkItemID{" cwi ", "cwi"}, EvidenceDeclarationRef: " declaration ", CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}, TargetRefs: []string{" b ", "a", "a"}}
}

func TestIndependentBindingPredicateKeepsOriginalNormalizationAndCompleteGate(t *testing.T) {
	for _, scenario := range []string{"none", "validation", "behavior", "owner", "unknown-owner", "incomplete-owner", "metric", "artifact", "blank", "nil", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			focus := independentBindingPredicateFixture()
			switch scenario {
			case "validation":
				focus.RequiredEvidenceKinds = []string{"", " validation ", "validation"}
			case "behavior":
				focus.RequireExecutableBehaviorEvidence = true
			case "owner":
				requirement := RuntimeOwnerFactRequirement{OwnerModuleID: " " + RuntimeModuleShell + " ", FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{requirement, requirement}
				focus.OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: requirement, RequirePreexisting: true}}
			case "unknown-owner":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "foreign", FactKind: "unknown", OperationKind: "unknown"}}
			case "incomplete-owner":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "owner"}, {FactKind: "kind"}, {OperationKind: "operation"}}
			case "metric":
				focus.MetricConstraints = []string{" ", " metric ", "metric"}
			case "artifact":
				focus.ArtifactRefs = []string{"", " artifact ", "artifact"}
			case "blank":
				focus.MetricConstraints, focus.ArtifactRefs, focus.RequiredEvidenceKinds = []string{"", "\t\n"}, []string{" "}, []string{" validation-x "}
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: " ", FactKind: " ", OperationKind: " "}}
			case "nil":
				focus = CoverageFocus{}
			case "empty":
				focus = CoverageFocus{MetricConstraints: []string{}, ArtifactRefs: []string{}, RequiredOwnerFacts: []RuntimeOwnerFactRequirement{}}
			}
			want := scenario == "validation" || scenario == "behavior" || scenario == "owner" || scenario == "unknown-owner" || scenario == "metric" || scenario == "artifact"
			if referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus) != want {
				t.Fatal("原谓词 fixture 未命中")
			}
			assertIndependentBindingPredicateReference(t, focus)
			focus.CoverageWorkItemIDs = append(focus.CoverageWorkItemIDs, "other")
			assertIndependentBindingPredicateReference(t, focus)
			focus.CoverageWorkItemIDs, focus.EvidenceDeclarationRef = []common.CoverageWorkItemID{"cwi"}, " declaration/* "
			assertIndependentBindingPredicateReference(t, focus)
		})
	}
	focus := independentBindingPredicateFixture()
	if RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		t.Fatal("无声明前像应为 false")
	}
	focus.ArtifactRefs = []string{"artifact"}
	if !RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		t.Fatal("后续声明变化借用先前结果")
	}
	focus.ArtifactRefs[0] = " "
	if RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		t.Fatal("空白条目未按旧语义丢弃")
	}
}

func TestIndependentBindingPredicateKeepsOwnerRequirementFiltering(t *testing.T) {
	for _, requirement := range []RuntimeOwnerFactRequirement{
		{OwnerModuleID: " ", FactKind: "kind", OperationKind: "operation"},
		{OwnerModuleID: "owner", FactKind: " ", OperationKind: "operation"},
		{OwnerModuleID: "owner", FactKind: "kind", OperationKind: "\t"},
	} {
		focus := independentBindingPredicateFixture()
		focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{requirement, requirement}
		if referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus) || RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
			t.Fatalf("缺失 identity 的 owner requirement 未丢弃：%+v", requirement)
		}
		assertIndependentBindingPredicateReference(t, focus)
		focus.RequiredOwnerFacts = append(focus.RequiredOwnerFacts, RuntimeOwnerFactRequirement{OwnerModuleID: "unknown", FactKind: "unknown", OperationKind: "unknown"})
		if !RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
			t.Fatal("后续非空非法声明必须保留存在性")
		}
		assertIndependentBindingPredicateReference(t, focus)
	}
}

func TestIndependentBindingPredicateKeepsDeclarationRejectionOrder(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	for _, scenario := range []struct {
		name string
		want string
	}{
		{"verification", "verification policy requires"},
		{"owner", "owner facts:"},
		{"convergence", "owner-fact convergence:"},
		{"cardinality", "exactly one CWI"},
		{"wildcard", "wildcard declaration"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			focus := independentBindingPredicateFixture()
			focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{requirement}
			focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi", "other"}
			focus.EvidenceDeclarationRef = " declaration/* "
			switch scenario.name {
			case "verification":
				focus.VerificationPolicy = &RuntimeVerificationPolicy{Version: "invalid"}
				focus.RequiredOwnerFacts[0].FactKind = "unknown"
			case "owner":
				focus.RequiredOwnerFacts[0].FactKind = "unknown"
			case "cardinality", "wildcard":
				focus.OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: requirement, RequirePreexisting: true}}
			}
			if scenario.name == "wildcard" {
				focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi"}
			}
			if !RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
				t.Fatal("非空声明不可在谓词中静默过滤")
			}
			if err := ValidateRuntimeFocusEvidenceDeclaration(focus); err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("完整声明拒绝优先级漂移：%v，want=%q", err, scenario.want)
			}
			assertIndependentBindingPredicateReference(t, focus)
		})
	}
}

func TestIndependentBindingPredicateKeepsNormalizationIsolation(t *testing.T) {
	focus := independentBindingPredicateFixture()
	focus.MetricConstraints = []string{" metric "}
	focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "owner", FactKind: "kind", OperationKind: "operation"}}
	before := cloneCoverageFocusRaw(focus)
	normalized := focus.Normalized()
	normalized.MetricConstraints[0] = "changed"
	normalized.RequiredOwnerFacts[0].OwnerModuleID = "changed"
	normalized.CoverageEvidenceBindings[0].Proofs[0].RequiredCheckRefs[0] = "changed"
	if !reflect.DeepEqual(focus, before) {
		t.Fatal("公开规范化副本污染原 focus")
	}
	assertIndependentBindingPredicateReference(t, focus)
	assertIndependentBindingPredicateReference(t, normalized)
}

func TestIndependentBindingPredicateRemovesOnlyDiscardedFocusCopies(t *testing.T) {
	for _, trigger := range []string{"none", "evidence", "behavior", "owner", "metric", "artifact", "blank"} {
		t.Run(trigger, func(t *testing.T) {
			focus := independentBindingPredicateFixture()
			switch trigger {
			case "evidence":
				focus.RequiredEvidenceKinds = []string{" validation "}
			case "behavior":
				focus.RequireExecutableBehaviorEvidence = true
			case "owner":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: "owner", FactKind: "kind", OperationKind: "operation"}}
			case "metric":
				focus.MetricConstraints = []string{" metric "}
			case "artifact":
				focus.ArtifactRefs = []string{" artifact "}
			case "blank":
				focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: " ", FactKind: "kind", OperationKind: "operation"}}
				focus.MetricConstraints, focus.ArtifactRefs = []string{" "}, []string{"\t\n"}
			}
			original := testing.AllocsPerRun(20, func() { _ = referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus) })
			current := testing.AllocsPerRun(20, func() { _ = RuntimeFocusRequiresIndependentEvidenceBinding(focus) })
			if current != 0 || current >= original {
				t.Fatalf("谓词仍复制无关 focus：current=%v original=%v", current, original)
			}
		})
	}
}

func FuzzIndependentBindingPredicateReference(f *testing.F) {
	f.Add(" validation ", "", "", "owner", "kind", "operation", false)
	f.Add(string([]byte{0xff, 0x80}), " ", "\x00", "", "", "", true)
	f.Fuzz(func(t *testing.T, evidence, metric, artifact, owner, kind, operation string, behavior bool) {
		focus := independentBindingPredicateFixture()
		focus.RequiredEvidenceKinds, focus.MetricConstraints, focus.ArtifactRefs = []string{evidence, evidence}, []string{metric, metric}, []string{artifact, artifact}
		focus.RequireExecutableBehaviorEvidence = behavior
		focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{OwnerModuleID: owner, FactKind: RuntimeOwnerFactKind(kind), OperationKind: operation}}
		assertIndependentBindingPredicateReference(t, focus)
	})
}

func BenchmarkIndependentBindingPredicate(b *testing.B) {
	focus := independentBindingPredicateFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceRuntimeFocusRequiresIndependentEvidenceBinding(focus)
				} else {
					_ = RuntimeFocusRequiresIndependentEvidenceBinding(focus)
				}
			}
		})
	}
}
