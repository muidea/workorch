package orchestrator

import (
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Same-Run input maintenance, real multi-source failures, non-reproduction,
// confirmed File repair, fresh all-of, assessment and all terminal owners.
// Only provider documents are scripted; no failure/target/receipt is injected.
func TestDriveRuntimeNonReproducingFailureToAcceptanceAndRecovery(t *testing.T) {
	t.Parallel()
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol, DynamicProjectObservations: true, SequentialProjectFailures: true, NativeProjectSources: true, IncrementalNativeSources: true, NonReproducingProjectFailures: true}
	baseline := runValidationInputReviewMainline(t, "input_revision_after_failure", "", options)
	for _, cut := range []string{"after_project_receipt", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "input_revision_after_failure", cut, options); calls != baseline {
				t.Fatalf("recovery repeated provider request: %d != %d", calls, baseline)
			}
		})
	}
}

func assertNonReproducingFailureClosure(t *testing.T, s rc.RuntimeStateRecord) {
	t.Helper()
	count := 0
	for _, r := range s.OperationReceipts {
		o := r.Evidence.Validation
		if o == nil || o.ExecutionConjunction == nil {
			continue
		}
		for _, stage := range o.ExecutionConjunction.Stages {
			if stage.DiagnosticReproduction != rc.ValidationReproductionAbsent {
				continue
			}
			count++
			if r.Outcome != rc.RuntimeOperationOutcomeRejected || o.ProofClass != "" || stage.ExitCode != 0 || stage.KnownFailure || stage.DiagnosticSourceRef == "" || len(stage.BehaviorTargetRefs) == 0 || r.Feedback == nil || r.Feedback.OriginalDiagnostic.ValidationFailureObservation == nil {
				t.Fatal("non-reproduction erased original failure or bypassed source-bound grounding")
			}
			f := rc.ProjectRuntimeValidationExecutionFeedback(o)
			if f.Validate() != nil || len(f.DiagnosticComparisons) == 0 || len(f.FailedChecks) == 0 || len(f.CheckFailures) == 0 {
				t.Fatal("independent observation or original assertion lost from feedback")
			}
		}
	}
	if count < 2 {
		t.Fatalf("sequence did not exercise two non-reproducing failures: %d", count)
	}
}

// The same independent PASS must stop when it has no implementation behavior.
// This uses real Shell raw/receipt persistence and all three terminal owners.
func TestDriveRuntimeNonReproducingFailureWithoutBehaviorStops(t *testing.T) {
	t.Parallel()
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol, NonReproducingProjectFailures: true}
	baseline := runValidationInputReviewMainline(t, "project_history_failure", "", options)
	if calls := runValidationInputReviewMainline(t, "project_history_failure", "after_project_receipt", options); calls != baseline {
		t.Fatal("unattributed failure recovery repeated provider request")
	}
}

func assertUnattributedNonReproducingFailure(t *testing.T, s rc.RuntimeStateRecord) {
	t.Helper()
	observed := false
	for _, r := range s.OperationReceipts {
		o := r.Evidence.Validation
		if o == nil || o.ExecutionConjunction == nil {
			continue
		}
		for _, stage := range o.ExecutionConjunction.Stages {
			if stage.DiagnosticReproduction != rc.ValidationReproductionAbsent {
				continue
			}
			observed = true
			if stage.ExitCode != 0 || stage.DiagnosticSourceRef != "" || len(stage.BehaviorTargetRefs) != 0 || o.ProofClass != "" || r.Outcome != rc.RuntimeOperationOutcomeRejected || r.Feedback == nil || r.Feedback.OriginalDiagnostic.ValidationFailureObservation != nil {
				t.Fatal("unattributed independent PASS gained repair or validation authority")
			}
		}
	}
	if !observed {
		t.Fatal("negative fixture did not actually observe non-reproduction")
	}
}
