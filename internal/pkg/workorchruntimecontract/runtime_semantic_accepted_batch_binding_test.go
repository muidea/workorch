package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompileRuntimeSemanticAcceptedBatchBindingsPreservesCompleteGates(t *testing.T) {
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	before, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	want, wantContract, err := CompileRuntimeSemanticBindings(*ledger.Manifest, ledger.Batches[0].RequestLogicalID, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		t.Fatal(err)
	}
	got, gotContract, err := CompileRuntimeSemanticAcceptedBatchBindings(*ledger.Manifest, ledger.Batches[0], *ledger.Document, ledger.Feasibilities)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatalf("accepted batch 编译产物不等价: %v", err)
	}
	after, err := json.Marshal(ledger)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("编译修改原始输入: %v", err)
	}
	for _, scenario := range []string{"manifest", "batch_hash", "batch_manifest", "batch_duplicate", "batch_pending", "batch_state", "request", "document_hash", "document_batch", "document_sequence", "document_coverage", "owner", "missing_owner", "duplicate_owner", "nonfeasible_owner"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := ledger.Manifest.Clone()
			batch := ledger.Batches[0].Clone()
			document := ledger.Document.Clone()
			facts := append([]RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...)
			switch scenario {
			case "manifest":
				manifest.ProofObligations[0].DeclarationRef = "drift"
			case "batch_hash":
				batch.Hash = "drift"
			case "batch_manifest":
				batch.ManifestHash = "drift"
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "batch_duplicate":
				batch.CoverageWorkItemIDs = append(batch.CoverageWorkItemIDs, batch.CoverageWorkItemIDs[0])
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "batch_pending":
				batch.PendingCoverageWorkItemIDs = append(batch.PendingCoverageWorkItemIDs, batch.CoverageWorkItemIDs[0])
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "batch_state":
				batch.State = RuntimeSemanticPlanBatchFeasibilityComplete
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "request":
				batch.RequestLogicalID = ""
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "document_hash":
				batch.DocumentHash = "drift"
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "document_batch":
				document.BatchID = "drift"
				batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "document_sequence":
				document.BatchSequence++
				batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "document_coverage":
				document.BatchCoverageWorkItemIDs = append(document.BatchCoverageWorkItemIDs, document.BatchCoverageWorkItemIDs[0])
				batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
				batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
			case "owner":
				facts[0].OwnerModuleID = RuntimeModuleShell
			case "missing_owner":
				facts = nil
			case "duplicate_owner":
				facts = append(facts, facts[0])
			case "nonfeasible_owner":
				facts[0].Status = RuntimeSemanticFeasibilityStatus("unsupported")
			}
			if _, _, err := CompileRuntimeSemanticAcceptedBatchBindings(manifest, batch, document, facts); err == nil {
				t.Fatal("同次复用绕过 batch/document/owner 来源反证")
			}
		})
	}
}

func BenchmarkCompileRuntimeSemanticAcceptedBatchBindings(b *testing.B) {
	ledger, _ := assessmentPurposeGenerationForTest(b, "cwi-source", "source-request", false)
	for _, mode := range []string{"independent", "call_local"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				if mode == "independent" {
					err = ledger.Batches[0].ValidateAgainstManifest(*ledger.Manifest)
					if err == nil {
						_, _, err = CompileRuntimeSemanticBindings(*ledger.Manifest, ledger.Batches[0].RequestLogicalID, *ledger.Document, ledger.Feasibilities)
					}
				} else {
					_, _, err = CompileRuntimeSemanticAcceptedBatchBindings(*ledger.Manifest, ledger.Batches[0], *ledger.Document, ledger.Feasibilities)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
