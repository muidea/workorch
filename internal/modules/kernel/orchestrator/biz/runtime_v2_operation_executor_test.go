package orchestrator

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationExecutionResultUsesOnlyPersistedReceiptGovernance(t *testing.T) {
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "approval_required",
		Message: "ordinary operation text must not create governance",
	}
	withoutGovernance := runtimeOperationExecutionResult(
		"event-1", "operation-1", runtimecontract.EvidenceKindStateChange,
		runtimecontract.RuntimeOperationExecutionStatusRejected, "source-1", failure,
		runtimecontract.RuntimeOperationEvidence{}, runtimecontract.RuntimeOperationReceipt{},
	)
	if withoutGovernance.Governance != nil {
		t.Fatalf("failure text unexpectedly inferred governance: %+v", withoutGovernance)
	}
	if withoutGovernance.Failure == nil || withoutGovernance.Failure.Code != "approval_required" {
		t.Fatalf("ordinary failure was not retained for repair: %+v", withoutGovernance)
	}

	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: ".env", Reason: "sensitive workspace file",
		RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high",
	}
	withGovernance := runtimeOperationExecutionResult(
		"event-2", "operation-2", runtimecontract.EvidenceKindStateChange,
		runtimecontract.RuntimeOperationExecutionStatusRejected, "source-2", failure,
		runtimecontract.RuntimeOperationEvidence{}, runtimecontract.RuntimeOperationReceipt{Governance: &requirement},
	)
	if withGovernance.Governance == nil || *withGovernance.Governance != requirement {
		t.Fatalf("persisted typed governance was not preserved: %+v", withGovernance)
	}
}
