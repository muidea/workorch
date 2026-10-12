package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assertAssessmentPreimageCopyEquivalent(t testing.TB, ledger RuntimeSemanticPlanningLedger) {
	t.Helper()
	before, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	original := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	want, wantErr := referenceAssessmentPurposePreimagesFromSemanticGeneration(ledger)
	for range 2 {
		got, gotErr := runtimeAssessmentPurposePreimagesFromSemanticGeneration(ledger)
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Fatalf("减少复制改变完整前像/首个错误: got=%v want=%v", gotErr, wantErr)
		}
		if len(got) != 0 {
			got[0].bindings[0].TargetRefs = []string{"caller-change"}
			got[0].bindings[0].Proofs[0].AllowedOperationKinds[0] = "caller-change"
			got[0].acceptedDocument.Proposals[0].OperationKind = "caller-change"
			got[0].acceptedDocument.Proposals[0].Target.OwnerSelectorRefs = []string{"caller-change"}
			got[0].ownerCatalogs[0].ObservedRevision = "caller-change"
		}
	}
	after, err := json.Marshal(ledger)
	if err != nil || string(before) != string(after) || !reflect.DeepEqual(ledger, original) {
		t.Fatal("前像中间复制复用改写原始 ledger")
	}
}

func TestAssessmentPreimageIntermediateCopiesKeepOriginalOutput(t *testing.T) {
	original := semanticBindingIndexedMultiBatchFixture(t, 4)
	for _, scenario := range []string{"valid", "document_space", "document_negative_sequence", "duplicate_document", "document_hash", "owner", "batch"} {
		t.Run(scenario, func(t *testing.T) {
			ledger := cloneRuntimeSemanticPlanningLedgerRaw(original)
			switch scenario {
			case "document_space":
				ledger.BatchDocuments[0].SchemaVersion = " " + ledger.BatchDocuments[0].SchemaVersion + " "
			case "document_negative_sequence":
				ledger.BatchDocuments[0].BatchSequence = -1
			case "duplicate_document":
				ledger.BatchDocuments = append(ledger.BatchDocuments, ledger.BatchDocuments[0])
			case "document_hash":
				ledger.BatchDocuments[0].ManifestHash = "changed"
			case "owner":
				ledger.BatchFeasibilityResults[0].Feasibilities[0].OwnerModuleID = "other"
			case "batch":
				ledger.Batches[0].Hash = "changed"
			}
			assertAssessmentPreimageCopyEquivalent(t, ledger)
			body, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertAssessmentPreimageCopyEquivalent(t, reloaded)
		})
	}
	current := testing.AllocsPerRun(10, func() {
		if _, err := runtimeAssessmentPurposePreimagesFromSemanticGeneration(original); err != nil {
			t.Fatal(err)
		}
	})
	reference := testing.AllocsPerRun(10, func() {
		if _, err := referenceAssessmentPurposePreimagesFromSemanticGeneration(original); err != nil {
			t.Fatal(err)
		}
	})
	if current >= reference {
		t.Fatalf("相同 generation 完整校验未减少中间复制分配: current=%v reference=%v", current, reference)
	}
}

func FuzzAssessmentPreimageIntermediateCopiesMatchOriginal(f *testing.F) {
	base := semanticBindingIndexedMultiBatchFixture(f, 2)
	for selector := range uint8(5) {
		f.Add(selector, " 原文\r\n ")
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string) {
		ledger := cloneRuntimeSemanticPlanningLedgerRaw(base)
		switch selector % 5 {
		case 0:
			ledger.BatchDocuments[0].SchemaVersion = value
		case 1:
			ledger.BatchDocuments[0].Proposals[0].DeclarationRef = value
		case 2:
			ledger.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = append(ledger.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs, value)
		case 3:
			ledger.BatchFeasibilityResults[0].Feasibilities[0].OwnerModuleID = value
		case 4:
			ledger.Batches[0].Hash = value
		}
		assertAssessmentPreimageCopyEquivalent(t, ledger)
	})
}
