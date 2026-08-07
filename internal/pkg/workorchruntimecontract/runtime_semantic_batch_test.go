package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticPlanBatchRejectsCoverageOutsideManifest(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	batch := RuntimeSemanticPlanBatch{
		BatchID: "batch-1", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageOrderHash: "order-1",
		Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-unknown"},
		RequestLogicalID: "request-1", State: RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	if err := batch.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("batch accepted coverage outside manifest")
	}
}

func TestRuntimeSemanticPlanBatchRejectsPendingCoverageOutsideBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	batch := RuntimeSemanticPlanBatch{
		BatchID: "batch-1", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageOrderHash: "order-1",
		Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		PendingCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"}, RequestLogicalID: "request-1", State: RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	if err := batch.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("batch accepted pending coverage outside batch")
	}
}

func TestRuntimeSemanticPlanBatchHashIsStable(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	batch := RuntimeSemanticPlanBatch{
		BatchID: "batch-1", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: "order-1", Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequestLogicalID: "request-1", State: RuntimeSemanticPlanBatchRequesting,
	}
	first := StableRuntimeSemanticPlanBatchHash(batch)
	batch.Hash = first
	if second := StableRuntimeSemanticPlanBatchHash(batch); second != first {
		t.Fatalf("batch hash drifted: %q -> %q", first, second)
	}
}

func TestCompileRuntimeSemanticPlanBatchUsesProviderOrderWithoutResorting(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-b", SliceID: "slice-1", SourceScope: "test", SourceID: "b"},
		{ID: "cwi-a", SliceID: "slice-1", SourceScope: "test", SourceID: "a"},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-b", "cwi-a"}}
	batch, err := CompileRuntimeSemanticPlanBatch(manifest, order, 0, "request-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.CoverageWorkItemIDs) != 1 || batch.CoverageWorkItemIDs[0] != "cwi-b" || len(batch.PendingCoverageWorkItemIDs) != 1 || batch.PendingCoverageWorkItemIDs[0] != "cwi-a" {
		t.Fatalf("batch did not preserve provider order: %+v", batch)
	}
}

func TestCompileRuntimeSemanticPlanBatchAdvancesThroughProviderOrder(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-b", SliceID: "slice-1", SourceScope: "test", SourceID: "b"},
		{ID: "cwi-a", SliceID: "slice-1", SourceScope: "test", SourceID: "a"},
		{ID: "cwi-c", SliceID: "slice-1", SourceScope: "test", SourceID: "c"},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-b", "cwi-a", "cwi-c"}}
	batch, err := CompileRuntimeSemanticPlanBatchFromOffset(manifest, order, 1, 2, "request-2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.CoverageWorkItemIDs) != 1 || batch.CoverageWorkItemIDs[0] != "cwi-c" || len(batch.PendingCoverageWorkItemIDs) != 0 {
		t.Fatalf("second batch replayed or reordered provider coverage: %+v", batch)
	}
}

func TestAdvanceRuntimeSemanticPlanBatchRejectsLifecycleSkip(t *testing.T) {
	batch := RuntimeSemanticPlanBatch{BatchID: "batch-1", State: RuntimeSemanticPlanBatchPending}
	if _, err := AdvanceRuntimeSemanticPlanBatch(batch, RuntimeSemanticPlanBatchAccepted); err == nil {
		t.Fatal("batch lifecycle skipped intermediate states")
	}
}

func TestMergeRuntimeSemanticPlanBatchDocumentsRejectsDuplicateCoverage(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "test", SourceID: "two"})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-001", "cwi-002"}}
	proposal := semanticContractTestProposal("cwi-001")
	first := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-0", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, CoverageOrder: order.CoverageOrder, Proposals: []RuntimeSemanticProofProposal{proposal}}
	second := first.Clone()
	second.BatchID = "batch-1"
	second.BatchSequence = 1
	second.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-002"}
	if _, err := MergeRuntimeSemanticPlanBatchDocuments(manifest, order, []RuntimeSemanticPlanDocument{first, second}); err == nil {
		t.Fatal("merge accepted documents with incomplete proposal coverage")
	}
}

func TestValidateRuntimeSemanticPlanBatchFeasibilitiesRejectsMissingOwnerFact(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-0", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, CoverageOrder: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
	}
	if err := ValidateRuntimeSemanticPlanBatchFeasibilities(manifest, document, nil); err == nil {
		t.Fatal("batch feasibility accepted without owner fact")
	}
}

func TestMergeRuntimeSemanticPlanBatchFeasibilityResultsRejectsMissingBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, CoverageOrder: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")}}
	if _, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest, document, nil, nil); err == nil {
		t.Fatal("feasibility merge accepted missing batch result")
	}
}
