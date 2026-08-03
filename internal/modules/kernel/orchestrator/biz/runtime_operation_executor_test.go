package orchestrator

import (
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationExecutionResultUsesOnlyPersistedReceiptGovernance(t *testing.T) {
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "approval_required",
		Message: "ordinary operation text must not create governance",
	}
	withoutGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-1", Operation: runtimecontract.RuntimeOperation{ID: "operation-1"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{
			Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
			Feedback: &runtimecontract.RuntimeFeedback{
				Domain: runtimecontract.RuntimeFeedbackDomainOperation, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected),
				OriginalDiagnostic: runtimecontract.RuntimeOriginalDiagnosticFromFailureFact(runtimecontract.RuntimeModuleFile, *failure),
			},
		},
	)
	if withoutGovernance.Governance != nil {
		t.Fatalf("failure text unexpectedly inferred governance: %+v", withoutGovernance)
	}
	if withoutGovernance.Failure == nil || withoutGovernance.Failure.Code != "approval_required" {
		t.Fatalf("ordinary failure was not retained for repair: %+v", withoutGovernance)
	}
	if withoutGovernance.EventID != "runtime-operation|dispatch-1|operation-1" || withoutGovernance.SourceRef != "workorch://runtime-operations/dispatch-1/operation-1" || withoutGovernance.OperationID != "operation-1" || withoutGovernance.EvidenceKind != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("sealed operation identity was not projected by Orchestrator: %+v", withoutGovernance)
	}

	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: ".env", Reason: "sensitive workspace file",
		RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high",
	}
	withGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-2", Operation: runtimecontract.RuntimeOperation{ID: "operation-2"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, Governance: &requirement},
	)
	if withGovernance.Governance == nil || *withGovernance.Governance != requirement {
		t.Fatalf("persisted typed governance was not preserved: %+v", withGovernance)
	}
}
