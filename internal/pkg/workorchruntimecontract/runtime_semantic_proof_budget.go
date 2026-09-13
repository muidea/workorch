package workorchruntimecontract

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSemanticProofBatchUsage is a detached projection of the validated
// semantic ledger, not a consumption authority. A batch reserves one unit for
// each selected CWI; its pending window, sessions and corrections reserve none.
// Retiring a generation cannot release reservations or rename the original CWI.
func RuntimeSemanticProofBatchUsage(ledger RuntimeSemanticPlanningLedger) map[common.CoverageWorkItemID]int {
	usage := make(map[common.CoverageWorkItemID]int)
	add := func(batches []RuntimeSemanticPlanBatch) {
		for _, batch := range batches {
			for _, id := range batch.CoverageWorkItemIDs {
				usage[id]++
			}
		}
	}
	for _, history := range ledger.History {
		add(history.Snapshot.Batches)
	}
	add(ledger.Batches)
	return usage
}

// ValidateRuntimeSemanticProofBatchLimits is shared by current-shape aggregate
// and opaque-state validation. Structural/lineage validation must also run; this
// projection must not normalize or deduplicate corrupt stored batch identities.
func ValidateRuntimeSemanticProofBatchLimits(ledger RuntimeSemanticPlanningLedger, limits *RuntimeExecutionLimits) error {
	if limits == nil || limits.MaxProofBatchesPerCWI == 0 {
		return nil
	}
	usage := RuntimeSemanticProofBatchUsage(ledger)
	ids := make([]string, 0, len(usage))
	for id := range usage {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		if usage[common.CoverageWorkItemID(id)] > limits.MaxProofBatchesPerCWI {
			return fmt.Errorf("semantic proof batches for CWI %q exceed sealed limit %d", id, limits.MaxProofBatchesPerCWI)
		}
	}
	return nil
}
