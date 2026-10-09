package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assessmentArchivedResultFixture(t testing.TB) (RuntimeSemanticPlanningLedger, RuntimeSemanticPlanBatchFeasibilityResult) {
	t.Helper()
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	result := RuntimeSemanticPlanBatchFeasibilityResult{BatchID: ledger.Batches[0].BatchID, BatchHash: ledger.Batches[0].Hash, DocumentHash: ledger.DocumentHash, Feasibilities: ledger.Feasibilities}
	result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
	return ledger, result
}

func TestAssessmentArchivedResultReusePreservesCompleteGates(t *testing.T) {
	ledger, result := assessmentArchivedResultFixture(t)
	for _, scenario := range []string{"valid", "batch_hash", "document", "result_hash", "owner", "missing_owner", "duplicate_owner"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := ledger.Manifest.Clone()
			batch := ledger.Batches[0].Clone()
			document := ledger.Document.Clone()
			result := result.Normalized()
			switch scenario {
			case "batch_hash":
				batch.Hash = "drift"
			case "document":
				document.BatchSequence++
			case "result_hash":
				result.Hash = "drift"
			case "owner":
				result.Feasibilities[0].OwnerModuleID = RuntimeModuleShell
				result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
			case "missing_owner":
				result.Feasibilities = nil
				result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
			case "duplicate_owner":
				result.Feasibilities = append(result.Feasibilities, result.Feasibilities[0])
				result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
			}
			validated, err := validateRuntimeSemanticPlanManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			wantOK := result.Validate(batch, document, manifest) == nil
			before, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			got, gotOK := runtimeAssessmentPurposeBatchFeasibilities(ledger, batch, document, map[string]RuntimeSemanticPlanBatchFeasibilityResult{batch.BatchID: result}, validated)
			after, marshalErr := json.Marshal(ledger)
			if gotOK != wantOK || (gotOK && !reflect.DeepEqual(got, result.Feasibilities)) || marshalErr != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("owner result 完整校验不等价: got=%v want=%v", gotOK, wantOK)
			}
			if scenario != "valid" && gotOK {
				t.Fatal("篡改 owner 前像获得接受")
			}
			if gotOK {
				got[0].OwnerModuleID = "changed"
				after, err := json.Marshal(ledger)
				if err != nil || !reflect.DeepEqual(before, after) || result.Feasibilities[0].OwnerModuleID == "changed" {
					t.Fatal("复用返回值共享 owner 来源")
				}
			}
		})
	}
}

func TestAssessmentArchivedResultRevalidatesRawManifest(t *testing.T) {
	ledger, result := assessmentArchivedResultFixture(t)
	ledger.BatchFeasibilityResults = []RuntimeSemanticPlanBatchFeasibilityResult{result}
	if _, err := runtimeAssessmentPurposePreimagesFromSemanticGeneration(ledger); err != nil {
		t.Fatal(err)
	}
	changed := ledger.Clone()
	changed.Manifest.OwnerCatalogs = append(changed.Manifest.OwnerCatalogs, changed.Manifest.OwnerCatalogs[0])
	if _, err := runtimeAssessmentPurposePreimagesFromSemanticGeneration(changed); err == nil {
		t.Fatal("同 hash 重复 raw owner catalog 被规范化掩盖")
	}
	changed = ledger.Clone()
	changed.Manifest.OwnerCatalogs[0].ObservedRevision = "forged"
	if _, err := runtimeAssessmentPurposePreimagesFromSemanticGeneration(changed); err == nil {
		t.Fatal("前次通过掩盖后续原始 owner catalog 漂移")
	}
}

func BenchmarkAssessmentArchivedResultValidation(b *testing.B) {
	ledger, result := assessmentArchivedResultFixture(b)
	for _, mode := range []string{"independent", "call_local"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
				if err != nil {
					b.Fatal(err)
				}
				if mode == "independent" {
					err = result.Validate(ledger.Batches[0], *ledger.Document, *ledger.Manifest)
				} else {
					_, ok := runtimeAssessmentPurposeBatchFeasibilities(ledger, ledger.Batches[0], *ledger.Document, map[string]RuntimeSemanticPlanBatchFeasibilityResult{result.BatchID: result}, validated)
					if !ok {
						b.Fatal("call-local validation failed")
					}
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
