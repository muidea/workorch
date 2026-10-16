package orchestrator

import (
	"context"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
)

// ExecuteRuntimeDispatch intentionally stops after the receipt/evidence CAS.
// Recovery tests must perform the same original-owner read stage as SM-4 before
// asserting assessment authority; they must not forge a current observation or
// weaken the prior exactly-once receipt/effect/assessment oracle.
func settleRuntimeOwnerApplicabilityForTest(t *testing.T, ctx context.Context, module *Biz, aggregate runtime.RuntimeAggregate) runtime.RuntimeAggregate {
	t.Helper()
	before := aggregate.CloneRaw()
	for n := 0; n <= len(before.Facts.Evidence.Accepted); n++ {
		updated, changed, err := module.driveRuntimeEvidenceApplicability(ctx, aggregate)
		if err != nil {
			t.Fatalf("recover original owner applicability: %v", err)
		}
		if !changed {
			if !reflect.DeepEqual(before.OperationReceipts, aggregate.OperationReceipts) ||
				!reflect.DeepEqual(before.Facts.Evidence, aggregate.Facts.Evidence) ||
				before.EvidenceSequence != aggregate.EvidenceSequence || !reflect.DeepEqual(before.ExecutionBudget, aggregate.ExecutionBudget) {
				t.Fatal("read-only recovery rewrote receipt/evidence/sequence/budget")
			}
			return aggregate
		}
		if updated.StateRevision <= aggregate.StateRevision {
			t.Fatal("owner applicability recovery made no persisted progress")
		}
		aggregate = updated
	}
	t.Fatal("owner applicability recovery exceeded original evidence inventory")
	return aggregate
}
