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
			cuts := []string{"after_review_document", "after_review_binding", "after_project_receipt"}
			if mode == "project_history_failure" {
				cuts = append(cuts, "after_failure_input_review_open", "after_failure_input_review_document")
			}
			for _, cut := range cuts {
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

func assertMultiFailureConfirmedRepairFacts(t *testing.T, state runtimecontract.RuntimeStateRecord, multiFile bool) {
	t.Helper()
	repairs := 0
	for _, dispatch := range state.CompletedDispatches {
		if dispatch.RepairCommitment == nil || dispatch.Candidate.StateChange == nil {
			continue
		}
		repairs++
		commitment := dispatch.RepairCommitment
		if dispatch.State != runtimecontract.RuntimeDispatchStateCommitted || len(dispatch.Candidate.StateChange.Operations) != 1+boolCount(multiFile) || len(dispatch.OperationExecutions) != 1+boolCount(multiFile) || dispatch.ImplementationWorkCommitment != nil || commitment.SourceReceiptHash == "" {
			t.Fatal("confirmed repair 缺少唯一原行为 commitment/fence")
		}
		confirmations := 0
		for _, record := range state.RepairSurfaceConfirmations {
			if record.Signature == commitment.ConfirmationHash && record.IdentitySignature == commitment.ConfirmationIdentityHash && record.Confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceConfirmed && record.Confirmation.OwnerModuleID == runtimecontract.RuntimeModuleFile {
				confirmations++
				if multiFile && len(record.Confirmation.ConfirmedTargetRefs) != 9 {
					t.Fatal("confirmation lost part of the complete nine-target observation")
				}
			}
		}
		if confirmations != 1 {
			t.Fatal("repair requires exactly one original File confirmation")
		}
		seen := map[string]bool{}
		for _, entry := range dispatch.Candidate.StateChange.Operations {
			operation := entry.Operation
			var execution runtimecontract.RuntimeOperationExecutionRecord
			for _, e := range dispatch.OperationExecutions {
				if e.OperationID == operation.ID {
					execution = e
				}
			}
			receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if !found || operation.Kind != runtimecontract.StepKindFileEdit || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed || receipt.BeforeHash == "" || receipt.AfterHash == "" || receipt.BeforeHash == receipt.AfterHash {
				t.Fatal("confirmed repair lost its exact committed File version edge")
			}
			facts := 0
			for _, fact := range state.Facts.Evidence.Accepted {
				if fact.OperationID == operation.ID && fact.DispatchID == dispatch.ID {
					facts++
					if len(fact.CoverageWorkItemIDs) != 0 || fact.Kind != runtimecontract.EvidenceKindStateChange {
						t.Fatal("repair effect acquired validation or CWI credit")
					}
				}
			}
			if facts != 1 || len(receipt.TargetRefs) != 1 || seen[receipt.TargetRefs[0]] || receipt.TargetRefs[0] != "subject.go" && (!multiFile || receipt.TargetRefs[0] != "offset.go") {
				t.Fatal("repair operation/fact/target is not unique")
			}
			seen[receipt.TargetRefs[0]] = true
		}
	}
	if repairs != 1 {
		t.Fatalf("真实 repair dispatch 数量=%d，要求恰好一次", repairs)
	}
}

// The real producer/owners must retain a late assertion, confirm its repair,
// obtain fresh all-of validation and assessment, and deliver all three ACKs.
func TestDriveRuntimeProjectLargeOutputConfirmedRepair(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, LargeProjectOutput: true}
	baseline := runValidationInputReviewMainline(t, "project_behavior_failure", "", options)
	for _, cut := range []string{"after_project_receipt", "after_repair_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "project_behavior_failure", cut, options); calls != baseline {
				t.Fatal("recovery repeated provider consumption")
			}
		})
	}
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Four independent failures observe a complete nine-file surface. Two distinct
// File edits are required: either alone still fails the original assertions.
func TestDriveRuntimeNineTargetMultiFileRepairAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, LargeProjectOutput: true, MultiFileProject: true}
	baseline := runValidationInputReviewMainline(t, "project_behavior_failure", "", options)
	for _, cut := range []string{"after_project_receipt", "after_repair_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "project_behavior_failure", cut, options); calls != baseline {
				t.Fatalf("recovery repeated provider consumption: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeNineTargetMultiFileDeliveryRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, LargeProjectOutput: true, MultiFileProject: true}
	baseline := runValidationInputReviewMainline(t, "project_behavior_failure", "", options)
	for _, cut := range []string{"after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "project_behavior_failure", cut, options); calls != baseline {
				t.Fatalf("delivery recovery repeated provider consumption: %d != %d", calls, baseline)
			}
		})
	}
}

// No owner result/target/credit is scripted: changing native observations must
// survive receipt loss, exact input re-review, confirmed File repair, fresh
// ordinary all-of, assessment and ordered terminal delivery without extra calls.
func TestDriveRuntimeDynamicObservationsRepairAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, DynamicProjectObservations: true}
	baseline := runValidationInputReviewMainline(t, "project_behavior_failure", "", options)
	for _, cut := range []string{"after_project_receipt", "after_failure_input_review_document", "after_repair_receipt", "after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "project_behavior_failure", cut, options); calls != baseline {
				t.Fatalf("recovery added provider consumption: %d/%d", calls, baseline)
			}
		})
	}
}
