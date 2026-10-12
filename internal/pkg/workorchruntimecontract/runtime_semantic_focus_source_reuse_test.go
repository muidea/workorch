package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func FuzzSemanticFocusSourceReuseMatchesOriginalCompiler(f *testing.F) {
	ledger := semanticBindingIndexedMultiBatchFixture(f, 2)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		f.Fatal(err)
	}
	proofSets, err := OrderedRuntimeSemanticCWIProofSets(ledger.BatchDocuments[0])
	if err != nil {
		f.Fatal(err)
	}
	proofSet := proofSets[0]
	item := validated.coverageByID[proofSet.CoverageWorkItemID]
	obligation := validated.obligationByCoverage[proofSet.CoverageWorkItemID]
	original := runtimeSemanticProofObligationSourceFocus(obligation)
	evidenceObligation := resolveValidatedRuntimeSemanticEvidenceObligation(obligation)
	generationID := StableRuntimeSemanticGenerationID(validated.manifest.Hash, ledger.Batches[0].RequestLogicalID)
	facts := make(map[string]RuntimeSemanticProofFeasibility)
	for _, fact := range ledger.BatchFeasibilityResults[0].Feasibilities {
		facts[fact.ProposalHash] = fact
	}
	for selector := range uint8(5) {
		f.Add(selector, " 原文\r\n ")
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string) {
		source := original.Normalized()
		switch selector % 5 {
		case 0:
			source.MetricConstraints = append(source.MetricConstraints, value)
		case 1:
			source.ArtifactRefs = append(source.ArtifactRefs, value)
		case 2:
			source.TargetRole = value
		case 3:
			source.TargetSelectorCandidateRefs = append(source.TargetSelectorCandidateRefs, value)
		case 4:
			source.AssessmentText = value
		}
		before, _ := json.Marshal(source)
		originalSource := cloneCoverageFocusRaw(source)
		got, gotErr := compileRuntimeSemanticFocus(validated, generationID, item, proofSet, facts, &source, evidenceObligation)
		want, wantErr := semanticBindingIndexedReferenceFocus(validated.manifest, generationID, item, proofSet, facts, &source, evidenceObligation)
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Fatalf("source 单次规范化改变原 focus 产物/错误: got=%v want=%v", gotErr, wantErr)
		}
		if gotErr == nil {
			got.CoverageWorkItemIDs[0] = "caller-change"
			got.CoverageEvidenceBindings[0].Proofs[0].AllowedOperationKinds[0] = "caller-change"
		}
		after, _ := json.Marshal(source)
		if string(before) != string(after) || !reflect.DeepEqual(source, originalSource) {
			t.Fatal("source 复用改写原始来源 bytes")
		}
	})
}

func TestSemanticFocusSourceReuseKeepsOriginalCompilerOutput(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 4)
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	fixtures := []RuntimeSemanticPlanningLedger{ledger, semanticBindingIndexedDocumentFixture(manifest, document, facts)}
	definition, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	repair, _ := assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	fixtures = append(fixtures, definition, repair)
	for _, fixture := range fixtures {
		validated, err := validateRuntimeSemanticPlanManifest(*fixture.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		batch, document := fixture.Batches[0], fixture.BatchDocuments[0]
		facts := fixture.Feasibilities
		if len(fixture.BatchFeasibilityResults) != 0 {
			facts = fixture.BatchFeasibilityResults[0].Feasibilities
		}
		before, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		want, wantContract, wantErr := semanticBindingIndexedReferenceCompiler(validated, batch.RequestLogicalID, document, facts)
		if wantErr != nil {
			t.Fatal(wantErr)
		}
		for range 2 {
			got, contract, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, batch.RequestLogicalID, document, facts)
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(contract, wantContract) {
				t.Fatalf("source 复用改变完整 focus/binding/reconciliation: %v", err)
			}
			got[0].CoverageWorkItemIDs[0] = "caller-change"
			got[0].CoverageEvidenceBindings[0].Proofs[0].AllowedOperationKinds[0] = "caller-change"
			got[0].RequiredEvidenceKinds[0] = "caller-change"
		}
		after, err := json.Marshal(fixture)
		if err != nil || string(before) != string(after) {
			t.Fatal("source 复用共享原 ledger 嵌套正文")
		}
		current := testing.AllocsPerRun(10, func() {
			_, _, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, batch.RequestLogicalID, document, facts)
			if err != nil {
				t.Fatal(err)
			}
		})
		reference := testing.AllocsPerRun(10, func() {
			_, _, err := semanticBindingIndexedReferenceCompiler(validated, batch.RequestLogicalID, document, facts)
			if err != nil {
				t.Fatal(err)
			}
		})
		if current >= reference {
			t.Fatalf("source 复用路径未减少原 compiler 分配: current=%v reference=%v", current, reference)
		}
	}
}
