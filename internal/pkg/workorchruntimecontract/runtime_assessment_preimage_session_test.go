package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assessmentPreimageSessionFixture(t testing.TB) RuntimeSemanticPlanningLedger {
	t.Helper()
	active, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "current-request", false)
	retainResult := func(ledger *RuntimeSemanticPlanningLedger) {
		result := RuntimeSemanticPlanBatchFeasibilityResult{
			BatchID: ledger.Batches[0].BatchID, BatchHash: ledger.Batches[0].Hash,
			DocumentHash: ledger.DocumentHash, Feasibilities: ledger.Feasibilities,
		}
		result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
		ledger.BatchFeasibilityResults = []RuntimeSemanticPlanBatchFeasibilityResult{result}
	}
	retainResult(&active)
	for index, request := range []string{"history-request-a", "history-request-b"} {
		historical, _ := assessmentPurposeGenerationForTest(t, "cwi-source", request, false)
		retainResult(&historical)
		active.History = append(active.History, RuntimeSemanticPlanningGeneration{
			RetiredAtStateRevision: uint64(index + 2), Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
			InvalidationSignatures: []string{request}, PrerequisiteIDs: []string{historical.Manifest.OwnerCatalogs[0].PrerequisiteID}, Snapshot: historical,
		})
	}
	if err := active.Validate(); err != nil {
		t.Fatal(err)
	}
	return active
}

func assertAssessmentPreimageSessionEquivalent(t testing.TB, ledger RuntimeSemanticPlanningLedger, alreadyValidated bool) {
	t.Helper()
	before, _ := json.Marshal(ledger)
	want, wantErr := referenceAssessmentPurposePreimagesFromSemanticLedger(ledger, alreadyValidated)
	for range 2 {
		got, gotErr := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, alreadyValidated)
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Fatalf("前像产物或拒绝顺序不等价: got=%v want=%v", gotErr, wantErr)
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("同次 session 改写原 ledger bytes")
	}
}

func TestAssessmentPreimageSessionPreservesExactRawHistory(t *testing.T) {
	original := assessmentPreimageSessionFixture(t)
	for _, scenario := range []string{"valid", "same_hash_catalog", "duplicate_catalog", "manifest", "duplicate_document", "document", "batch", "owner", "history_identity"} {
		t.Run(scenario, func(t *testing.T) {
			ledger := original.Clone()
			history := &ledger.History[0]
			switch scenario {
			case "same_hash_catalog":
				history.Snapshot.Manifest.OwnerCatalogs[0].ObservedRevision = "forged"
			case "duplicate_catalog":
				history.Snapshot.Manifest.OwnerCatalogs = append(history.Snapshot.Manifest.OwnerCatalogs, history.Snapshot.Manifest.OwnerCatalogs[0])
			case "manifest":
				history.Snapshot.Manifest.ProofObligations[0].DeclarationRef = "changed"
			case "duplicate_document":
				history.Snapshot.BatchDocuments = append(history.Snapshot.BatchDocuments, history.Snapshot.BatchDocuments[0])
			case "document":
				history.Snapshot.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = []string{"other.txt"}
			case "batch":
				history.Snapshot.Batches[0].Hash = "changed"
			case "owner":
				history.Snapshot.Feasibilities[0].OwnerModuleID = RuntimeModuleShell
			case "history_identity":
				history.RetiredAtStateRevision = 0
			}
			for _, alreadyValidated := range []bool{false, true} {
				assertAssessmentPreimageSessionEquivalent(t, ledger, alreadyValidated)
				if !alreadyValidated && scenario != "valid" {
					if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, false); err == nil {
						t.Fatal("未验证历史的非法身份、batch、document 或 owner 被接受")
					}
				}
				if scenario == "same_hash_catalog" || scenario == "duplicate_catalog" || scenario == "manifest" {
					if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, alreadyValidated); err == nil {
						t.Fatal("原始 manifest 变更被前代成功或同 hash 掩盖")
					}
				}
			}
			body, _ := json.Marshal(ledger)
			var reloaded RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertAssessmentPreimageSessionEquivalent(t, reloaded, false)
		})
	}
	assertAssessmentPreimageSessionEquivalent(t, semanticBindingIndexedMultiBatchFixture(t, 4), false)
	before, _ := json.Marshal(original)
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(original, false)
	if err != nil || len(entries) != 3 {
		t.Fatalf("完整前像数量=%d err=%v", len(entries), err)
	}
	entries[0].bindings[0].ProspectiveTargetRefs[0] = "changed"
	entries[0].ownerCatalogs[0].TargetRefs = []string{"changed"}
	entries[1].acceptedDocument.Proposals[0].Target.ProspectiveTargetRefs[0] = "changed"
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("同次 session 返回前像共享原历史")
	}
	original.History[0].Snapshot.Manifest.OwnerCatalogs[0].ObservedRevision = "later-drift"
	if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(original, false); err == nil {
		t.Fatal("后续调用复用前次成功")
	}
}

func TestAssessmentPreimageSessionRejectsMutatedSameHashWithinCall(t *testing.T) {
	ledger := assessmentPreimageSessionFixture(t)
	session := newRuntimeSemanticValidationSession()
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(ledger, session)
	if err != nil || len(entries) != 1 {
		t.Fatalf("合法原始前像数量=%d err=%v", len(entries), err)
	}
	changed := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	changed.History = nil
	changed.Manifest.OwnerCatalogs[0].ObservedRevision = "same-hash-drift"
	if _, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(changed, session); err == nil {
		t.Fatal("同次前像 session 仅凭 hash 复用了已变 raw manifest")
	}
	entries[0].ownerCatalogs[0].ObservedRevision = "caller-change"
	entries[0].bindings[0].ProspectiveTargetRefs[0] = "caller-change"
	replayed, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(ledger, session)
	if err != nil || len(replayed) != 1 || replayed[0].ownerCatalogs[0].ObservedRevision == "caller-change" || replayed[0].bindings[0].ProspectiveTargetRefs[0] == "caller-change" {
		t.Fatal("同次返回前像污染 session 内部校验产物")
	}
}

func FuzzAssessmentPreimageSessionNested(f *testing.F) {
	base := assessmentPreimageSessionFixture(f)
	for selector := range uint8(6) {
		f.Add(selector, "", false)
		f.Add(selector, " changed\r\n原文 ", true)
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string, alreadyValidated bool) {
		ledger := base.Clone()
		history := &ledger.History[0].Snapshot
		switch selector % 6 {
		case 0:
			history.Manifest.OwnerCatalogs[0].ObservedRevision = value
		case 1:
			history.Manifest.OwnerCatalogs[0].TargetRefs = append(history.Manifest.OwnerCatalogs[0].TargetRefs, value)
		case 2:
			history.Manifest.Hash = value
		case 3:
			history.BatchDocuments[0].Proposals[0].DeclarationRef = value
		case 4:
			history.Batches[0].Hash = value
		case 5:
			history.Feasibilities[0].OwnerModuleID = value
		}
		assertAssessmentPreimageSessionEquivalent(t, ledger, alreadyValidated)
	})
}

func BenchmarkAssessmentPreimageSession(b *testing.B) {
	ledger := assessmentPreimageSessionFixture(b)
	for _, alreadyValidated := range []bool{false, true} {
		mode := "independent"
		if alreadyValidated {
			mode = "enclosing_ledger_validated"
		}
		b.Run(mode, func(b *testing.B) {
			for _, reference := range []bool{false, true} {
				name := "call_local"
				if reference {
					name = "reference"
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var err error
						if reference {
							_, err = referenceAssessmentPurposePreimagesFromSemanticLedger(ledger, alreadyValidated)
						} else {
							_, err = runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, alreadyValidated)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
