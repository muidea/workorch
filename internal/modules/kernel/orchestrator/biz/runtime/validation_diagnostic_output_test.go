package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRejectedValidationUnavailableOutputTerminatesFromOriginalReceipt(t *testing.T) {
	for _, reason := range []string{"capacity", "storage"} {
		t.Run(reason, func(t *testing.T) {
			aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
			repository := &durableRuntimeStateRepository{aggregate: aggregate}
			coordinator, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			sealed := sealCandidateForTest(t, repository, validationCandidate())
			evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-unavailable-output", runtimecontract.ValidationDiagnosticTestFailed)
			observation := validationConjunctionReceiptFixture()
			conjunction := observation.ExecutionConjunction
			conjunction.Complete = false
			stage := &conjunction.Stages[0]
			stage.ExitCode = 1
			stage.KnownFailure = true
			stage.FailureCode = "validation_project_stage_failed"
			stage.PassedTests = 0
			if reason == "capacity" {
				stage.Stdout.Bytes = 2652869
				stage.Stdout.Complete = false
				stage.Stdout.OmittedReason = "shell_execution_raw_output_capacity_exceeded"
			} else {
				stage.Stdout.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "shell_stage_raw_storage_unavailable"}
			}
			evidence.Evidence.Validation = &observation
			attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
			original := evidence.Receipt.Clone()
			originalBudget := repository.aggregate.ExecutionBudget.Clone()
			committed, err := coordinator.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			got := committed.Aggregate
			if got.Decision.Terminal == nil || got.Decision.Terminal.Code != runtimeValidationDiagnosticOutputUnavailable || got.Decision.State != runtimecontract.ConvergenceStateFailed || got.ActiveDispatch != nil || got.Facts.RepairDirective != nil || got.Facts.Resources.FailureTarget != nil {
				t.Fatalf("incomplete diagnostic retried: terminal=%+v", got.Decision.Terminal)
			}
			if len(got.Facts.Evidence.Rejected) != 1 || len(got.Facts.Evidence.Accepted) != len(sealed.Facts.Evidence.Accepted) {
				t.Fatal("missing diagnostic obtained credit")
			}
			rejected := got.Facts.Evidence.Rejected[0]
			if rejected.FailureCode == runtimeValidationDiagnosticOutputUnavailable || !reflect.DeepEqual(rejected.Validation, &observation) || !runtimecontract.SameRuntimeOperationReceipt(got.OperationReceipts[len(got.OperationReceipts)-1], original) {
				t.Fatal("current failure overwrote original receipt/failure fact")
			}
			if got.Facts.Failure.ValidationDiagnostic == nil || !strings.Contains(got.Facts.Failure.Message, "exit=1 knownFailure=true") || !strings.Contains(got.Facts.Failure.Message, "original failure=") || !reflect.DeepEqual(originalBudget, got.ExecutionBudget) {
				t.Fatal("lost original exit/diagnostic or changed execution budget")
			}
			reloaded := runtimeTestEvidenceAggregateRoundTrip(t, got)
			if err := reloaded.Validate(); err != nil {
				t.Fatal(err)
			}
			repository.aggregate = reloaded
			resumed, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := resumed.commitEvidence(context.Background(), evidence)
			if err != nil || !reflect.DeepEqual(ownerStateRecord(duplicate.Aggregate), ownerStateRecord(reloaded)) {
				t.Fatalf("duplicate reclassified or reexecuted terminal receipt: %v", err)
			}
		})
	}
}

func TestCompleteRejectedValidationDoesNotBecomeOutputUnavailable(t *testing.T) {
	observation := validationConjunctionReceiptFixture()
	aggregate := RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "native_exit1"}}}
	admission := executionEvidenceAdmission{Evidence: ExecutionEvidence{Status: ExecutionEvidenceStatusRejected, Receipt: &runtimecontract.RuntimeOperationReceipt{Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &observation}}}}
	before := aggregate.CloneRaw()
	if marked, err := markValidationDiagnosticOutputUnavailable(&aggregate, admission, runtimecontract.EvidenceFact{Kind: runtimecontract.EvidenceKindValidation}); err != nil || marked || !reflect.DeepEqual(aggregate, before) {
		t.Fatal("complete original failure reclassified")
	}
	admission.Evidence.Status = ExecutionEvidenceStatusAccepted
	observation.ExecutionConjunction.Stages[0].Stdout.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "not stored"}
	if marked, err := markValidationDiagnosticOutputUnavailable(&aggregate, admission, runtimecontract.EvidenceFact{Kind: runtimecontract.EvidenceKindValidation}); err != nil || marked {
		t.Fatal("accepted evidence reclassified")
	}
}
