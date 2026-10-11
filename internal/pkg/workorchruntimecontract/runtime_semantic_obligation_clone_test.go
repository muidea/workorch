package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func semanticObligationCloneFullReference(o RuntimeSemanticProofObligation) RuntimeSemanticProofObligation {
	o = o.Normalized()
	o.RequiredEvidenceKinds = append([]string(nil), o.RequiredEvidenceKinds...)
	o.DeclarationAcceptedStepKinds = append([]string(nil), o.DeclarationAcceptedStepKinds...)
	if o.ValidationProofRequirement != nil {
		value := o.ValidationProofRequirement.Clone()
		o.ValidationProofRequirement = &value
	}
	o.DeclarationTargetDiscoverySources = append([]string(nil), o.DeclarationTargetDiscoverySources...)
	o.DeclarationTargetSelectorCandidateRefs = append([]string(nil), o.DeclarationTargetSelectorCandidateRefs...)
	o.DeclarationProspectiveTargetAllOfConstraintRefs = append([]string(nil), o.DeclarationProspectiveTargetAllOfConstraintRefs...)
	o.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), o.RequiredOwnerFacts...)
	o.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(o.OwnerFactConvergences)
	o.MetricConstraints = append([]string(nil), o.MetricConstraints...)
	o.ArtifactRefs = append([]string(nil), o.ArtifactRefs...)
	o.VerificationPolicy = cloneRuntimeVerificationPolicy(o.VerificationPolicy)
	if o.OwnerFactAcquisition != nil {
		value := o.OwnerFactAcquisition.Clone()
		o.OwnerFactAcquisition = &value
	}
	if o.ValidationFailureRepair != nil {
		value := o.ValidationFailureRepair.Clone()
		o.ValidationFailureRepair = &value
	}
	return o
}

func semanticObligationCloneFixture() RuntimeSemanticProofObligation {
	requirement := RuntimeOwnerFactRequirement{OwnerModuleID: " shell ", FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	return RuntimeSemanticProofObligation{
		CoverageWorkItemID: " cwi ", DeclarationRef: " declaration ",
		RequiredEvidenceKinds: []string{" validation ", EvidenceKindValidation}, DeclarationAcceptedStepKinds: []string{" file/write ", StepKindFileWrite},
		DeclarationTargetDiscoverySources: []string{" source ", "source"}, DeclarationTargetSelectorCandidateRefs: []string{" b ", "a"},
		DeclarationProspectiveTargetAllOfConstraintRefs: []string{" constraint ", "constraint"},
		RequiredOwnerFacts:    []RuntimeOwnerFactRequirement{requirement},
		OwnerFactConvergences: []RuntimeOwnerFactConvergence{{Requirement: requirement, DependsOnRequirementRefs: []string{" dependency ", "dependency"}}},
		MetricConstraints:     []string{" metric ", "metric"}, ArtifactRefs: []string{" artifact ", "artifact"},
		LineageAssertions:          []RuntimeLineageAssertionRule{{Ref: "原 rule"}},
		ValidationProofRequirement: &RuntimeValidationProofRequirement{CoverageWorkItemID: " cwi ", RequiredCheckRefs: []string{" b ", "a"}, SourceObligationChecks: []RuntimeSourceObligationCheck{{ObligationID: "obligation", Criterion: " 原判据\r\n\x00 "}}},
		VerificationPolicy:         &RuntimeVerificationPolicy{Version: " " + RuntimeVerificationPolicyVersion + " ", MinimumEvidenceLevel: 2},
		OwnerFactAcquisition: &RuntimeSemanticOwnerFactAcquisition{Requirement: requirement, AcquisitionConstraintRefs: []string{" constraint ", "constraint"}, ProspectiveTargetSatisfactionConstraint: &RuntimeProspectiveTargetSatisfactionConstraint{
			AllOf: []RuntimeProspectiveTargetClause{{ID: " clause ", AllOf: []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathPrefix, Value: " ./input\\ "}}}},
		}},
		ValidationFailureRepair: &RuntimeSemanticValidationFailureRepair{RepairOperationKinds: []string{" file/write "}, RepairDeclarationTargetSelectorCandidateRefs: []string{" target "}, RepairConstraintRefs: []string{" constraint "}},
		ImplementationWork:      &RuntimeSemanticImplementationWork{ExcludedTargetRefs: []string{" 原target "}, MutationInputCatalogs: []RuntimeOperationAdmissibility{{TargetRefs: []string{" 原input "}}}},
	}
}

func assertSemanticObligationCloneReference(t testing.TB, value RuntimeSemanticProofObligation) {
	t.Helper()
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := semanticObligationCloneFullReference(value)
	for range 2 {
		if !reflect.DeepEqual(value.Clone(), want) {
			t.Fatal("obligation Clone 改变原规范化或 nil/empty 表示")
		}
	}
	gotErr, wantErr := value.Clone().Validate(), want.Validate()
	if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
		t.Fatalf("obligation Clone 改变完整拒绝原因：got=%v want=%v", gotErr, wantErr)
	}
	after, err := json.Marshal(value)
	if err != nil || string(before) != string(after) {
		t.Fatal("obligation Clone 改写原输入")
	}
	var reloaded RuntimeSemanticProofObligation
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Clone(), semanticObligationCloneFullReference(reloaded)) {
		t.Fatal("JSON 重载改变原 obligation Clone")
	}
}

func TestSemanticObligationCloneMatchesCompleteReference(t *testing.T) {
	for name, value := range map[string]RuntimeSemanticProofObligation{
		"nil": {}, "complete": semanticObligationCloneFixture(),
		"empty": {RequiredEvidenceKinds: []string{}, DeclarationAcceptedStepKinds: []string{}, RequiredOwnerFacts: []RuntimeOwnerFactRequirement{}, OwnerFactConvergences: []RuntimeOwnerFactConvergence{}, LineageAssertions: []RuntimeLineageAssertionRule{},
			ValidationProofRequirement: &RuntimeValidationProofRequirement{SourceObligationChecks: []RuntimeSourceObligationCheck{}}, ImplementationWork: &RuntimeSemanticImplementationWork{ExcludedTargetRefs: []string{}}},
		"blank": {RequiredEvidenceKinds: []string{" "}, DeclarationAcceptedStepKinds: []string{" "}, OwnerFactConvergences: []RuntimeOwnerFactConvergence{{DependsOnRequirementRefs: []string{" "}}},
			OwnerFactAcquisition: &RuntimeSemanticOwnerFactAcquisition{AcquisitionConstraintRefs: []string{" "}}, ValidationFailureRepair: &RuntimeSemanticValidationFailureRepair{RepairOperationKinds: []string{" "}}},
	} {
		t.Run(name, func(t *testing.T) { assertSemanticObligationCloneReference(t, value) })
	}
}

func TestSemanticObligationCloneDetachesAllNestedContracts(t *testing.T) {
	value := semanticObligationCloneFixture()
	before, _ := json.Marshal(value)
	got := value.Clone()
	for _, refs := range [][]string{got.RequiredEvidenceKinds, got.DeclarationAcceptedStepKinds, got.DeclarationTargetDiscoverySources,
		got.DeclarationTargetSelectorCandidateRefs, got.DeclarationProspectiveTargetAllOfConstraintRefs, got.MetricConstraints, got.ArtifactRefs,
		got.OwnerFactConvergences[0].DependsOnRequirementRefs, got.ValidationProofRequirement.RequiredCheckRefs, got.OwnerFactAcquisition.AcquisitionConstraintRefs,
		got.ValidationFailureRepair.RepairOperationKinds, got.ValidationFailureRepair.RepairDeclarationTargetSelectorCandidateRefs, got.ValidationFailureRepair.RepairConstraintRefs,
		got.ImplementationWork.ExcludedTargetRefs, got.ImplementationWork.MutationInputCatalogs[0].TargetRefs} {
		refs[0] = "changed"
	}
	got.RequiredOwnerFacts[0].OwnerModuleID = "changed"
	got.OwnerFactConvergences[0].Requirement.OwnerModuleID = "changed"
	got.LineageAssertions[0].Ref = "changed"
	got.ValidationProofRequirement.DeclarationRef = "changed"
	got.ValidationProofRequirement.SourceObligationChecks[0].Criterion = "changed"
	got.VerificationPolicy.MinimumEvidenceLevel = 5
	got.OwnerFactAcquisition.Requirement.OwnerModuleID = "changed"
	got.OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint.AllOf[0].ID = "changed"
	got.OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed"
	got.ValidationFailureRepair.FailureSignature = "changed"
	got.ImplementationWork.RoundHash = "changed"
	after, _ := json.Marshal(value)
	if string(before) != string(after) {
		t.Fatal("obligation Clone 嵌套值与原输入共享存储")
	}
	assertSemanticObligationCloneReference(t, value)
	value.DeclarationRef = "新声明"
	value.OwnerFactConvergences[0].DependsOnRequirementRefs[0] = "新依赖"
	value.ValidationProofRequirement.SourceObligationChecks[0].Criterion = "新原判据"
	assertSemanticObligationCloneReference(t, value)
}

func TestSemanticObligationCloneValidManifestAndLedgerRetainIdentity(t *testing.T) {
	manifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	for index, value := range manifest.ProofObligations {
		assertSemanticObligationCloneReference(t, value)
		manifest.ProofObligations[index] = value.Clone()
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("有效 manifest obligation Clone 后拒绝：%v", err)
	}
	if manifest.Hash != semanticManifestHashFullEncodingReference(manifest) {
		t.Fatal("obligation Clone 改变原完整 manifest hash")
	}
	ledger := semanticSealedLedgerFixture(t)
	body, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("sealed ledger 重载身份漂移：%v", err)
	}
	for _, value := range reloaded.Manifest.ProofObligations {
		assertSemanticObligationCloneReference(t, value)
	}
	value := semanticObligationCloneFixture()
	original := testing.AllocsPerRun(20, func() { _ = semanticObligationCloneFullReference(value) })
	current := testing.AllocsPerRun(20, func() { _ = value.Clone() })
	if current >= original {
		t.Fatalf("obligation Clone 分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzSemanticObligationCloneReference(f *testing.F) {
	f.Add("0原声明\"\"&0", "0000000&\x80000", "原判据", ".")
	f.Add(" 原声明\r\n\x00 ", " file/write ", "原判据", " ./input\\ ")
	f.Add("", string([]byte{0xff, 0x80}), "", "")
	f.Fuzz(func(t *testing.T, declaration, kind, criterion, target string) {
		value := semanticObligationCloneFixture()
		value.DeclarationRef = declaration
		value.DeclarationAcceptedStepKinds = []string{kind, kind, " "}
		value.ValidationFailureRepair.RepairOperationKinds = []string{kind}
		value.ValidationProofRequirement.SourceObligationChecks[0].Criterion = criterion
		value.OwnerFactConvergences[0].DependsOnRequirementRefs = []string{declaration, " ", declaration}
		value.OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = target
		assertSemanticObligationCloneReference(t, value)
	})
}

func BenchmarkSemanticObligationClone(b *testing.B) {
	value := semanticObligationCloneFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = semanticObligationCloneFullReference(value)
				} else {
					_ = value.Clone()
				}
			}
		})
	}
}
