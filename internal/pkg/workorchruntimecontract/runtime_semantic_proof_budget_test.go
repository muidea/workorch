package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticProofBatchUsageRetainsOriginalCWIThroughHistory(t *testing.T) {
	// Projection fixtures deliberately omit unrelated manifest authority: this
	// test proves counting, not permission or structural admission.
	ledger := RuntimeSemanticPlanningLedger{
		Batches: []RuntimeSemanticPlanBatch{{BatchID: "new", CoverageWorkItemIDs: []common.CoverageWorkItemID{"a", "b"}, PendingCoverageWorkItemIDs: []common.CoverageWorkItemID{"c"}}},
		History: []RuntimeSemanticPlanningGeneration{{Snapshot: RuntimeSemanticPlanningLedger{
			Batches: []RuntimeSemanticPlanBatch{{BatchID: "old", CoverageWorkItemIDs: []common.CoverageWorkItemID{"a"}, PendingCoverageWorkItemIDs: []common.CoverageWorkItemID{"b", "c"}}},
		}}},
	}
	want := map[common.CoverageWorkItemID]int{"a": 2, "b": 1}
	if got := RuntimeSemanticProofBatchUsage(ledger); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage=%v", got)
	}
	limits := RuntimeExecutionLimits{ProtocolRef: RuntimeExecutionLimitsV1, MaxProviderCalls: 32, MaxCapabilityOperations: 16, MaxProofBatchesPerCWI: 2}
	if err := limits.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeSemanticProofBatchLimits(ledger, &limits); err != nil {
		t.Fatal(err)
	}
	before := StableRuntimeExecutionLimitsHash(limits)
	limits.MaxProofBatchesPerCWI = 1
	if StableRuntimeExecutionLimitsHash(limits) == before || ValidateRuntimeSemanticProofBatchLimits(ledger, &limits) == nil {
		t.Fatal("proof budget omitted from identity or invariant")
	}
	encoded, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if got := RuntimeSemanticProofBatchUsage(restored); !reflect.DeepEqual(got, want) {
		t.Fatal("reload reset usage")
	}
	for _, batch := range []*RuntimeSemanticPlanBatch{&restored.Batches[0], &restored.History[0].Snapshot.Batches[0]} {
		batch.ManifestHash, batch.RequestLogicalID, batch.BatchID = "changed-manifest", "changed-request", "changed-batch"
	}
	if !reflect.DeepEqual(RuntimeSemanticProofBatchUsage(restored), want) {
		t.Fatal("mutable identities refreshed budget")
	}
	got := RuntimeSemanticProofBatchUsage(restored)
	got["a"] = 0
	if RuntimeSemanticProofBatchUsage(restored)["a"] != 2 {
		t.Fatal("projection aliases authority")
	}
	limits.MaxProofBatchesPerCWI = 0
	if err := ValidateRuntimeSemanticProofBatchLimits(ledger, &limits); err != nil {
		t.Fatal("absent optional cap changed existing policy")
	}
}
