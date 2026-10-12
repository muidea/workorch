package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// 保留原逐条先 Clone 再匹配的算法；不通过当前 helper 构造 oracle。
func referenceAssessmentPurposeBindingPreimageForFocus(manifest RuntimeSemanticPlanManifest, focus CoverageFocus) (runtimeAssessmentPurposeBindingPreimage, error) {
	if len(focus.CoverageWorkItemIDs) != 1 {
		return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic focus must contain one coverage item")
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	var obligation *RuntimeSemanticProofObligation
	for index := range manifest.ProofObligations {
		candidate := manifest.ProofObligations[index].Clone()
		if candidate.CoverageWorkItemID != coverageID {
			continue
		}
		if obligation != nil {
			return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic manifest has duplicate CWI obligations")
		}
		obligation = &candidate
	}
	if obligation == nil {
		return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic manifest has no CWI obligation")
	}
	return runtimeAssessmentPurposeBindingPreimage{
		bindings: CloneCoverageEvidenceBindings(focus.CoverageEvidenceBindings), semanticManifestHash: manifest.Hash, obligation: *obligation,
	}, nil
}

func TestAssessmentPurposePreimageClonesOnlyMatchedObligation(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 8)
	focuses, _, err := CompileRuntimeSemanticBindings(*ledger.Manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities)
	if err != nil || len(focuses) != 1 {
		t.Fatalf("原完整 compiler fixture: %v", err)
	}
	for _, scenario := range []string{"valid", "missing", "duplicate", "empty_focus", "multiple_focus", "unrelated_raw"} {
		t.Run(scenario, func(t *testing.T) {
			manifest, focus := ledger.Manifest.Clone(), focuses[0].Normalized()
			switch scenario {
			case "missing":
				manifest.ProofObligations = manifest.ProofObligations[:7]
			case "duplicate":
				manifest.ProofObligations = append(manifest.ProofObligations, manifest.ProofObligations[7])
			case "empty_focus":
				focus.CoverageWorkItemIDs = nil
			case "multiple_focus":
				focus.CoverageWorkItemIDs = append(focus.CoverageWorkItemIDs, manifest.CoverageItems[0].ID)
			case "unrelated_raw":
				manifest.ProofObligations[0].DeclarationRef = " 原始非匹配正文\r\n "
			}
			before, _ := json.Marshal(manifest)
			focusBefore, _ := json.Marshal(focus)
			want, wantErr := referenceAssessmentPurposeBindingPreimageForFocus(manifest, focus)
			for range 2 {
				got, gotErr := runtimeAssessmentPurposeBindingPreimageForFocus(manifest, focus)
				if !reflect.DeepEqual(got, want) || (gotErr == nil) != (wantErr == nil) || gotErr != nil && gotErr.Error() != wantErr.Error() {
					t.Fatalf("匹配前像或首个错误漂移: got=%v want=%v", gotErr, wantErr)
				}
				if gotErr == nil {
					got.obligation.RequiredEvidenceKinds[0] = "caller-change"
					got.bindings[0].Proofs[0].RequiredEffectScopes[0] = "caller-change"
				}
			}
			after, _ := json.Marshal(manifest)
			focusAfter, _ := json.Marshal(focus)
			if string(before) != string(after) || string(focusBefore) != string(focusAfter) {
				t.Fatal("匹配产物共享原 obligation/binding 或改写原 bytes")
			}
		})
	}
	current := testing.AllocsPerRun(20, func() {
		if _, err := runtimeAssessmentPurposeBindingPreimageForFocus(*ledger.Manifest, focuses[0]); err != nil {
			panic(err)
		}
	})
	reference := testing.AllocsPerRun(20, func() {
		if _, err := referenceAssessmentPurposeBindingPreimageForFocus(*ledger.Manifest, focuses[0]); err != nil {
			panic(err)
		}
	})
	if current >= reference {
		t.Fatalf("逐 focus 完整遍历未减少无关复制: current=%v reference=%v", current, reference)
	}
}
