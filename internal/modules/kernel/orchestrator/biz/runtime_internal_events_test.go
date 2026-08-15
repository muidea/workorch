package orchestrator

import (
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOwnerFeasibilityInternalEventKindRequiresExplicitConstraintImpact(t *testing.T) {
	candidateLocal := runtimecontract.RuntimeOwnerFeasibilityObservation{
		Outcome: runtimecontract.RuntimeOwnerFeasibilityInfeasible,
		OwnerPolicy: runtimecontract.RuntimeOwnerPolicyIdentity{
			OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-policy-v1", CatalogSnapshotID: "catalog-v1",
		},
	}
	if got := runtimeOwnerFeasibilityInternalEventKind(candidateLocal); got != runtime.RuntimeInternalEventOwnerFeasibilityObserved {
		t.Fatalf("owner policy identity selected SM-2 contradiction event: %q", got)
	}

	contradiction := candidateLocal
	contradiction.ConstraintImpact = runtimecontract.RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted
	if got := runtimeOwnerFeasibilityInternalEventKind(contradiction); got != runtime.RuntimeInternalEventSemanticBindingContradictionObserved {
		t.Fatalf("explicit sealed-target contradiction event = %q", got)
	}
}
