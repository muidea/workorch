package workorchruntimecontract

import "testing"

// The correction disposition is an SM-3 policy owned by Orchestrator. The
// shared contract package only owns the value object's structural invariant.
func TestRuntimeCandidateCorrectionDirectiveValidate(t *testing.T) {
	t.Parallel()
	if err := (RuntimeCandidateCorrectionDirective{
		Disposition: RuntimeCandidateCorrectionCorrectableOnce,
		Objectives:  []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSelectOperationKind},
	}).Validate(); err != nil {
		t.Fatalf("correctable directive rejected: %v", err)
	}
	if err := (RuntimeCandidateCorrectionDirective{
		Disposition:  RuntimeCandidateCorrectionRejectedFinal,
		TerminalCode: "runtime_contract_defect",
	}).Validate(); err != nil {
		t.Fatalf("rejected-final directive rejected: %v", err)
	}
	if err := (RuntimeCandidateCorrectionDirective{Disposition: RuntimeCandidateCorrectionCorrectableOnce}).Validate(); err == nil {
		t.Fatal("empty correctable directive accepted")
	}
}
