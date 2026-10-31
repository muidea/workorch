package orchestrator

import (
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the reviewer/provider is scripted. Source inventory, original bytes,
// process execution, receipt/raw storage, Run CAS and recovery are real owners.
// An unrelated failure with no observed implementation remains fail-closed.
func TestDriveRuntimeProjectMultiFailureOwnerMainline(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}
	for _, mode := range []string{"consistent", "project_history_failure"} {
		t.Run(mode, func(t *testing.T) {
			baseline := runValidationInputReviewMainline(t, mode, "", options)
			for _, cut := range []string{"after_review_document", "after_review_binding", "after_project_receipt"} {
				t.Run(cut, func(t *testing.T) {
					if calls := runValidationInputReviewMainline(t, mode, cut, options); calls != baseline {
						t.Fatalf("source review/receipt recovery repeated provider consumption: %d != %d", calls, baseline)
					}
				})
			}
		})
	}
}

func TestDriveRuntimeProjectMultiFailureConfirmedRepair(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}
	baseline := runValidationInputReviewMainline(t, "project_behavior_failure", "", options)
	for _, cut := range []string{"after_review_binding", "after_project_receipt", "after_repair_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "project_behavior_failure", cut, options); calls != baseline {
				t.Fatalf("confirmed repair restart repeated provider budget: %d != %d", calls, baseline)
			}
		})
	}
}

func assertMultiFailureConfirmedRepairFacts(t *testing.T, state runtimecontract.RuntimeStateRecord) {
	t.Helper()
	repairs := 0
	for _, dispatch := range state.CompletedDispatches {
		if dispatch.RepairCommitment == nil || dispatch.Candidate.StateChange == nil {
			continue
		}
		repairs++
		commitment := dispatch.RepairCommitment
		if dispatch.State != runtimecontract.RuntimeDispatchStateCommitted || len(dispatch.Candidate.StateChange.Operations) != 1 || len(dispatch.OperationExecutions) != 1 || dispatch.ImplementationWorkCommitment != nil || commitment.SourceReceiptHash == "" {
			t.Fatal("confirmed repair 缺少唯一原行为 commitment/fence")
		}
		operation := dispatch.Candidate.StateChange.Operations[0].Operation
		execution := dispatch.OperationExecutions[0]
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
		if !found || operation.Kind != runtimecontract.StepKindFileEdit || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed {
			t.Fatalf("confirmed repair 真实 File receipt 缺失、漂移或未结算: found=%t kind=%s state=%s hashMatch=%t outcome=%s changed=%t", found, operation.Kind, execution.State, execution.OwnerReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt), receipt.Outcome, receipt.Changed)
		}
		facts := 0
		for _, fact := range state.Facts.Evidence.Accepted {
			if fact.OperationID == operation.ID && fact.DispatchID == dispatch.ID {
				facts++
				if len(fact.CoverageWorkItemIDs) != 0 || fact.Kind != runtimecontract.EvidenceKindStateChange {
					t.Fatal("历史 repair effect 获得了 CWI 或普通 validation credit")
				}
			}
		}
		confirmations := 0
		for _, record := range state.RepairSurfaceConfirmations {
			if record.Signature == commitment.ConfirmationHash && record.IdentitySignature == commitment.ConfirmationIdentityHash && record.Confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceConfirmed && record.Confirmation.OwnerModuleID == runtimecontract.RuntimeModuleFile {
				confirmations++
			}
		}
		if facts != 1 || confirmations != 1 || len(receipt.TargetRefs) != 1 || receipt.TargetRefs[0] != "subject.go" {
			t.Fatal("真实 repair 事实、File confirmation 或精确 target 不唯一")
		}
	}
	if repairs != 1 {
		t.Fatalf("真实 repair dispatch 数量=%d，要求恰好一次", repairs)
	}
}
