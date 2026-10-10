package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSemanticBindingDocumentValidationRetainsCompleteObligationGate(t *testing.T) {
	manifest, original, facts, _ := semanticIndependentProofSetFixture(t)
	validated, err := validateRuntimeSemanticPlanManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "declaration", "recipe", "missing_validation", "target", "duplicate_check", "wrong_kind", "missing_owner"} {
		t.Run(scenario, func(t *testing.T) {
			document := original.Clone()
			ownerFacts := append([]RuntimeSemanticProofFeasibility(nil), facts...)
			stateIndex, validationIndex := -1, -1
			for idx, proposal := range document.Proposals {
				if proposal.RequiredEvidenceKinds[0] == EvidenceKindStateChange {
					stateIndex = idx
				} else {
					validationIndex = idx
				}
			}
			if stateIndex < 0 || validationIndex < 0 {
				t.Fatal("fixture 缺少独立 state-change／validation proof")
			}
			switch scenario {
			case "declaration":
				document.Proposals[0].DeclarationRef = "unsealed-declaration"
			case "recipe":
				document.Proposals[validationIndex].ValidationRecipeID = "unsealed-recipe"
			case "missing_validation":
				document.Proposals = []RuntimeSemanticProofProposal{document.Proposals[stateIndex]}
			case "target":
				document.Proposals[stateIndex].Target.ExactTargetRefs = []string{"outside-owner"}
			case "duplicate_check":
				document.Proposals[validationIndex].RequiredCheckRefs = []string{"duplicate", "duplicate"}
			case "wrong_kind":
				document.Proposals[stateIndex].RequiredEvidenceKinds = []string{EvidenceKindValidation}
			case "missing_owner":
				ownerFacts = nil
			}
			before, _ := json.Marshal(document)
			got, gotContract, gotErr := compileRuntimeSemanticBindingsWithValidatedManifest(validated, "request-proof-gate", document, ownerFacts)
			want, wantContract, wantErr := semanticBindingIndexedReferenceCompiler(validated, "request-proof-gate", document, ownerFacts)
			if (gotErr == nil) != (scenario == "valid") || (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) {
				t.Fatalf("完整 document/owner 门禁改变: got=%v want=%v", gotErr, wantErr)
			}
			if gotErr != nil && gotErr.Error() != wantErr.Error() {
				t.Fatalf("原拒绝原因改变: got=%v want=%v", gotErr, wantErr)
			}
			after, _ := json.Marshal(document)
			if string(before) != string(after) {
				t.Fatal("同次复用改写原始 document")
			}
		})
	}
}

func FuzzSemanticBindingDocumentValidationReuse(f *testing.F) {
	ledger := semanticBindingIndexedMultiBatchFixture(f, 2)
	original := ledger.BatchDocuments[0]
	data, _ := json.Marshal(original)
	f.Add(data)
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var document RuntimeSemanticPlanDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return
		}
		validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		facts := ledger.BatchFeasibilityResults[0].Feasibilities
		before, _ := json.Marshal(document)
		got, gotContract, gotErr := compileRuntimeSemanticBindingsWithValidatedManifest(validated, ledger.Batches[0].RequestLogicalID, document, facts)
		want, wantContract, wantErr := semanticBindingIndexedReferenceCompiler(validated, ledger.Batches[0].RequestLogicalID, document, facts)
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) {
			t.Fatalf("独立旧 compiler 接受／产物不等价: got=%v want=%v", gotErr, wantErr)
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("独立旧 compiler 拒绝原因改变: got=%v want=%v", gotErr, wantErr)
		}
		after, _ := json.Marshal(document)
		if string(before) != string(after) {
			t.Fatal("编译改写原始 document")
		}
	})
}
