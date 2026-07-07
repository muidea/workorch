package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestContractViolationNormalizedCarriesDiagnosticContract(t *testing.T) {
	violation := ContractViolation{
		ID:               " violation-001 ",
		WorkspaceID:      common.WorkspaceID("ws-001"),
		RunID:            common.RunID("run-001"),
		StepID:           common.StepID("step-001"),
		ViolationType:    " repair_no_state_changing_output ",
		ViolatedRule:     " state_changing_required_missing ",
		FailureSignature: "class=runtime_failed|stage=contract",
		Recoverability:   "unexpected",
		ForbiddenRepeat:  []string{" read-only graph ", "", "read-only graph"},
		SourceRefs:       []string{" step-001 ", "", "step-001", "evidence-001"},
	}

	normalized := violation.Normalized()
	if normalized.ID != "violation-001" {
		t.Fatalf("id = %q, want trimmed", normalized.ID)
	}
	if normalized.ViolationType != "repair_no_state_changing_output" ||
		normalized.ViolatedRule != "state_changing_required_missing" {
		t.Fatalf("normalized violation = %+v, want type and rule", normalized)
	}
	if normalized.Signature != normalized.FailureSignature {
		t.Fatalf("signature = %q, want failure signature fallback %q", normalized.Signature, normalized.FailureSignature)
	}
	if normalized.Recoverability != ContractViolationRecoverabilityRetrySameContract {
		t.Fatalf("recoverability = %q, want retry default", normalized.Recoverability)
	}
	if len(normalized.ForbiddenRepeat) != 1 || normalized.ForbiddenRepeat[0] != "read-only graph" {
		t.Fatalf("forbiddenRepeat = %+v, want compact list", normalized.ForbiddenRepeat)
	}
	if len(normalized.SourceRefs) != 2 || normalized.SourceRefs[0] != "step-001" || normalized.SourceRefs[1] != "evidence-001" {
		t.Fatalf("sourceRefs = %+v, want compact refs", normalized.SourceRefs)
	}
	if !normalized.HasScope() {
		t.Fatalf("HasScope = false, want true")
	}
}

func TestContractViolationRecoverableOnlyForRetryableValues(t *testing.T) {
	retryable := []string{
		ContractViolationRecoverabilityRetrySameContract,
		ContractViolationRecoverabilityNarrowContract,
		ContractViolationRecoverabilityNormalizeThenRetry,
		ContractViolationRecoverabilityRebuildValidationRequiredOutput,
	}
	for _, value := range retryable {
		if !(ContractViolation{Recoverability: value}).Recoverable() {
			t.Fatalf("recoverability %q should be retryable", value)
		}
	}
	for _, value := range []string{
		ContractViolationRecoverabilityDecisionRequired,
		ContractViolationRecoverabilityFailFast,
	} {
		if (ContractViolation{Recoverability: value}).Recoverable() {
			t.Fatalf("recoverability %q should not be retryable", value)
		}
	}
}
