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

func TestCompileRuntimeSemanticPlanFocusBatchDoesNotTreatReopenAsPrerequisite(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-043", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-043", ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-046", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}},
		{ID: "cwi-046", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-046"},
		{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001"},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-043", "cwi-046", "cwi-001"},
	}
	batch, err := CompileRuntimeSemanticPlanFocusBatch(manifest, order, 0, "request-1",
		[]common.CoverageWorkItemID{"cwi-043"},
		[]common.CoverageWorkItemID{"cwi-043", "cwi-046", "cwi-001"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrderedCoverageWorkItemIDs(batch.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-043"}) {
		t.Fatalf("reopen edge expanded active batch: %+v", batch)
	}
	if !sameOrderedCoverageWorkItemIDs(batch.PendingCoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-046", "cwi-001"}) {
		t.Fatalf("pending coverage changed by reopen edge: %+v", batch)
	}
}

func TestCompileRuntimeSemanticPlanFocusBatchIncludesOnlyPrerequisiteClosure(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-046", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-046", ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-001", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}},
		{ID: "cwi-043", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-043", PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-046"}},
		{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001"},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-046", "cwi-043", "cwi-001"},
	}
	batch, err := CompileRuntimeSemanticPlanFocusBatch(manifest, order, 0, "request-1",
		[]common.CoverageWorkItemID{"cwi-043"},
		[]common.CoverageWorkItemID{"cwi-046", "cwi-043", "cwi-001"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrderedCoverageWorkItemIDs(batch.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-046", "cwi-043"}) {
		t.Fatalf("prerequisite closure was not admitted in sealed provider order: %+v", batch)
	}
	if !sameOrderedCoverageWorkItemIDs(batch.PendingCoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-001"}) {
		t.Fatalf("unrelated CWI entered prerequisite closure: %+v", batch)
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
	first := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-0", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
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
	if _, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest, []RuntimeSemanticPlanDocument{document}, nil, nil); err == nil {
		t.Fatal("feasibility merge accepted missing batch result")
	}
}

func TestMergeRuntimeSemanticPlanBatchFeasibilityResultsRejectsIncompleteBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-001"}}
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-0", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
	batch := RuntimeSemanticPlanBatch{BatchID: "batch-0", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageOrderHash: StableRuntimeSemanticPlanCoverageOrderHash(order), Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, RequestLogicalID: "request-1", DocumentHash: StableRuntimeSemanticPlanDocumentHash(document), State: RuntimeSemanticPlanBatchDocumentComplete}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	feasibility := RuntimeSemanticProofFeasibility{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: "catalog-1", Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "revision-1"}
	result := RuntimeSemanticPlanBatchFeasibilityResult{BatchID: batch.BatchID, BatchHash: batch.Hash, DocumentHash: StableRuntimeSemanticPlanDocumentHash(document), Feasibilities: []RuntimeSemanticProofFeasibility{feasibility}}
	result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
	if _, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest, []RuntimeSemanticPlanDocument{document}, []RuntimeSemanticPlanBatch{batch}, []RuntimeSemanticPlanBatchFeasibilityResult{result}); err == nil {
		t.Fatal("feasibility merge accepted a batch that was not accepted")
	}
}

func TestMergeRuntimeSemanticPlanBatchFeasibilityResultsRequiresExactBatchDocuments(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "second", Order: 2})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-001", "cwi-002"},
	}
	firstProposal := semanticContractTestProposal("cwi-001")
	secondProposal := semanticContractTestProposal("cwi-002")
	firstDocument := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-0", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{firstProposal},
	}
	secondDocument := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchSequence: 1, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"},
		Proposals: []RuntimeSemanticProofProposal{secondProposal},
	}
	firstBatch := RuntimeSemanticPlanBatch{
		BatchID: "batch-0", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: StableRuntimeSemanticPlanCoverageOrderHash(order), Sequence: 0,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, PendingCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"},
		RequestLogicalID: "request-0", DocumentHash: StableRuntimeSemanticPlanDocumentHash(firstDocument),
		State: RuntimeSemanticPlanBatchAccepted,
	}
	firstBatch.Hash = StableRuntimeSemanticPlanBatchHash(firstBatch)
	secondBatch := RuntimeSemanticPlanBatch{
		BatchID: "batch-1", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: StableRuntimeSemanticPlanCoverageOrderHash(order), Sequence: 1,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"}, RequestLogicalID: "request-1",
		DocumentHash: StableRuntimeSemanticPlanDocumentHash(secondDocument), State: RuntimeSemanticPlanBatchAccepted,
	}
	secondBatch.Hash = StableRuntimeSemanticPlanBatchHash(secondBatch)
	firstFact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: firstProposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(firstProposal), OwnerModuleID: firstProposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible,
		ObservedRevision: manifest.OwnerCatalogs[0].ObservedRevision, TargetRefs: []string{firstProposal.Target.ProspectiveTargetRef},
	}
	secondFact := firstFact.Clone()
	secondFact.CoverageWorkItemID = secondProposal.CoverageWorkItemID
	secondFact.ProposalHash = StableRuntimeSemanticProofProposalHash(secondProposal)
	secondFact.TargetRefs = []string{secondProposal.Target.ProspectiveTargetRef}
	firstResult := RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: firstBatch.BatchID, BatchHash: firstBatch.Hash, DocumentHash: StableRuntimeSemanticPlanDocumentHash(firstDocument),
		Feasibilities: []RuntimeSemanticProofFeasibility{firstFact},
	}
	firstResult.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(firstResult)
	secondResult := RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: secondBatch.BatchID, BatchHash: secondBatch.Hash, DocumentHash: StableRuntimeSemanticPlanDocumentHash(secondDocument),
		Feasibilities: []RuntimeSemanticProofFeasibility{secondFact},
	}
	secondResult.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(secondResult)
	merged, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(
		manifest,
		[]RuntimeSemanticPlanDocument{firstDocument, secondDocument},
		[]RuntimeSemanticPlanBatch{firstBatch, secondBatch},
		[]RuntimeSemanticPlanBatchFeasibilityResult{firstResult, secondResult},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 || merged[0].CoverageWorkItemID != "cwi-001" || merged[1].CoverageWorkItemID != "cwi-002" {
		t.Fatalf("merged owner facts = %+v", merged)
	}

	forged := secondDocument.Clone()
	forged.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001"}
	if _, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(
		manifest,
		[]RuntimeSemanticPlanDocument{firstDocument, forged},
		[]RuntimeSemanticPlanBatch{firstBatch, secondBatch},
		[]RuntimeSemanticPlanBatchFeasibilityResult{firstResult, secondResult},
	); err == nil {
		t.Fatal("feasibility merge accepted a document whose sealed batch membership was forged")
	}
}

func TestMergeRuntimeSemanticPlanBatchFeasibilityResultsRejectsMissingOriginalDocument(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-0", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	batch := RuntimeSemanticPlanBatch{
		BatchID: "batch-0", ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: "order", Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequestLogicalID: "request", DocumentHash: StableRuntimeSemanticPlanDocumentHash(document), State: RuntimeSemanticPlanBatchAccepted,
	}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	result := RuntimeSemanticPlanBatchFeasibilityResult{BatchID: batch.BatchID, BatchHash: batch.Hash, DocumentHash: batch.DocumentHash}
	result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
	if _, err := MergeRuntimeSemanticPlanBatchFeasibilityResults(
		manifest, nil, []RuntimeSemanticPlanBatch{batch}, []RuntimeSemanticPlanBatchFeasibilityResult{result},
	); err == nil {
		t.Fatal("feasibility merge accepted a batch without its original proof document")
	}
}
