package semanticplan

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompileBatchUsesAcceptedProviderOrderAndExplicitOffset(t *testing.T) {
	items := []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-b", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "b", TargetText: "b", Order: 2},
		{ID: "cwi-a", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "a", TargetText: "a", Order: 1},
		{ID: "cwi-c", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "c", TargetText: "c", Order: 3},
	}
	manifest := batchCompilerTestManifest(t, items)
	order := batchCompilerTestOrder(t, manifest, []common.CoverageWorkItemID{"cwi-b", "cwi-a", "cwi-c"})

	first, err := CompileBatch(manifest, order, 0, "request-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-b"}) || !reflect.DeepEqual(first.PendingCoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-a", "cwi-c"}) {
		t.Fatalf("first batch reordered accepted provider order: %+v", first)
	}
	second, err := CompileBatchFromOffset(manifest, order, 1, 2, "request-2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-c"}) || len(second.PendingCoverageWorkItemIDs) != 0 {
		t.Fatalf("offset batch replayed provider order: %+v", second)
	}
	for _, batch := range []runtimecontract.RuntimeSemanticPlanBatch{first, second} {
		if batch.State != runtimecontract.RuntimeSemanticPlanBatchPending || batch.Hash != runtimecontract.StableRuntimeSemanticPlanBatchHash(batch) {
			t.Fatalf("owner-private compiler did not seal initial pending batch: %+v", batch)
		}
	}
}

func TestCompileFocusBatchUsesOnlyPrerequisiteClosureNotReopenEdges(t *testing.T) {
	items := []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-prerequisite", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "prerequisite", TargetText: "prerequisite", Order: 1, ReopenDependencies: []runtimecontract.RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-unrelated", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}}},
		{ID: "cwi-active", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "active", TargetText: "active", Order: 2, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-prerequisite"}},
		{ID: "cwi-unrelated", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "unrelated", TargetText: "unrelated", Order: 3},
	}
	manifest := batchCompilerTestManifest(t, items)
	orderIDs := []common.CoverageWorkItemID{"cwi-prerequisite", "cwi-active", "cwi-unrelated"}
	order := batchCompilerTestOrder(t, manifest, orderIDs)
	batch, err := CompileFocusBatch(manifest, order, 0, "request-1", []common.CoverageWorkItemID{"cwi-active"}, orderIDs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(batch.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-prerequisite", "cwi-active"}) || !reflect.DeepEqual(batch.PendingCoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-unrelated"}) {
		t.Fatalf("focus batch closure = %+v", batch)
	}
}

func TestCompileFocusBatchDrainsLargeProviderOrderWithoutIDBias(t *testing.T) {
	const (
		coverageCount = 64
		batchSize     = 4
	)
	items := make([]runtimecontract.RuntimeCoverageItem, 0, coverageCount)
	orderIDs := make([]common.CoverageWorkItemID, 0, coverageCount)
	for index := 0; index < coverageCount; index++ {
		prefix := "z"
		if index%2 != 0 {
			prefix = "a"
		}
		id := common.CoverageWorkItemID(fmt.Sprintf("cwi-%s-%02d", prefix, index))
		item := runtimecontract.RuntimeCoverageItem{ID: id, SliceID: "slice-1", SourceScope: "dodItem", SourceID: fmt.Sprintf("dod-%02d", index), TargetText: "deliver", Order: coverageCount - index}
		if index%batchSize != 0 {
			item.PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{orderIDs[index-1]}
		}
		items = append(items, item)
		orderIDs = append(orderIDs, id)
	}
	manifestItems := make([]runtimecontract.RuntimeCoverageItem, len(items))
	for index := range items {
		manifestItems[len(items)-1-index] = items[index]
	}
	manifest := batchCompilerTestManifest(t, manifestItems)
	order := batchCompilerTestOrder(t, manifest, orderIDs)
	pending := append([]common.CoverageWorkItemID(nil), orderIDs...)
	seen := map[common.CoverageWorkItemID]int{}
	for sequence := 0; sequence < coverageCount/batchSize; sequence++ {
		leaf := orderIDs[sequence*batchSize+batchSize-1]
		batch, err := CompileFocusBatch(manifest, order, sequence, fmt.Sprintf("request-%02d", sequence), []common.CoverageWorkItemID{leaf}, pending, batchSize)
		if err != nil {
			t.Fatalf("batch %d: %v", sequence, err)
		}
		for _, id := range batch.CoverageWorkItemIDs {
			seen[id]++
		}
		pending = append([]common.CoverageWorkItemID(nil), batch.PendingCoverageWorkItemIDs...)
	}
	if len(pending) != 0 || len(seen) != coverageCount {
		t.Fatalf("provider order not drained: pending=%d seen=%d", len(pending), len(seen))
	}
	for _, id := range orderIDs {
		if seen[id] != 1 {
			t.Fatalf("coverage %s selected %d times", id, seen[id])
		}
	}
}

func batchCompilerTestManifest(t *testing.T, items []runtimecontract.RuntimeCoverageItem) runtimecontract.RuntimeSemanticPlanManifest {
	t.Helper()
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"catalog-file", runtimecontract.RuntimeModuleFile,
		[]string{runtimecontract.StepKindFileWrite}, []string{"main.go"}, []string{"new-target"}, []string{"path"}, "tree-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path"},
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	obligations := make([]runtimecontract.RuntimeSemanticProofObligation, 0, len(items))
	for _, item := range items {
		obligations = append(obligations, runtimecontract.RuntimeSemanticProofObligation{
			CoverageWorkItemID: item.ID,
			DeclarationRef:     runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
			RequiredEvidenceKinds: []string{
				runtimecontract.EvidenceKindStateChange,
			},
		})
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(), CoverageItems: append([]runtimecontract.RuntimeCoverageItem(nil), items...),
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations:      obligations, OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("manifest fixture: %v", err)
	}
	return manifest
}

func batchCompilerTestOrder(t *testing.T, manifest runtimecontract.RuntimeSemanticPlanManifest, ids []common.CoverageWorkItemID) runtimecontract.RuntimeSemanticPlanDocument {
	t.Helper()
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: append([]common.CoverageWorkItemID(nil), ids...),
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("coverage order fixture: %v", err)
	}
	return order
}
