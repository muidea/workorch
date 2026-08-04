package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCloneRuntimeOperationExecutionContextDetachesNestedValues(t *testing.T) {
	proof := RuntimeOperationReplayProof{OwnerModuleID: RuntimeModuleFile, OperationID: "write-1", TargetRefs: []string{"a.go"}, BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: "admissibility-1"}
	input := RuntimeOperationExecutionContext{
		Execution:   RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/write-1", Fence: 1, ExpectedEffectScope: RuntimeEffectWorkspaceDurable, ReplayProof: &proof},
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "write-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"a.go"},
		EvidencePolicies: []RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: StepKindFileWrite, ArgsContain: []string{"arg"}, CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{ID: "coverage-1", Scope: "direct", TargetRefs: []string{"a.go"}}}}},
	}

	cloned := CloneRuntimeOperationExecutionContext(input)
	cloned.CoverageWorkItemIDs[0] = "cwi-2"
	cloned.TargetRefs[0] = "b.go"
	cloned.Execution.ReplayProof.TargetRefs[0] = "b.go"
	cloned.EvidencePolicies[0].ArgsContain[0] = "changed"
	cloned.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0] = "b.go"

	if input.CoverageWorkItemIDs[0] != "cwi-1" || input.TargetRefs[0] != "a.go" || input.Execution.ReplayProof.TargetRefs[0] != "a.go" ||
		input.EvidencePolicies[0].ArgsContain[0] != "arg" || input.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0] != "a.go" {
		t.Fatalf("clone mutated source context: %+v", input)
	}
}
