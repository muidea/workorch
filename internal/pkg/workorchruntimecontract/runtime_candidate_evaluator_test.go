package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestRuntimeCandidateRejectionCodeDispositionMatrixIsTotal(t *testing.T) {
	t.Parallel()
	codes := []struct {
		code         string
		disposition  RuntimeCandidateCorrectionDisposition
		terminalCode string
	}{
		{code: "candidate_contract_invalid", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_schema_invalid", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_shape", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_partially_replays_rejected_shape", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_no_progress_shape", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_action_evidence_mismatch", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_target_outside_grounding", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_creation_scope_missing_file_target", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "operation_target_not_regular", disposition: RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_candidate_shape", disposition: RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "candidate_retry_exhausted", disposition: RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "workspace_resolution_failed", disposition: RuntimeCandidateCorrectionRejectedFinal, terminalCode: "workspace_resolution_failed"},
		{code: "operation_feasibility_probe_invalid", disposition: RuntimeCandidateCorrectionRejectedFinal, terminalCode: "operation_feasibility_probe_invalid"},
		{code: "runtime_contract_defect", disposition: RuntimeCandidateCorrectionRejectedFinal, terminalCode: "runtime_contract_defect"},
	}
	for _, testCase := range codes {
		disposition, ok := RuntimeCandidateRejectionCodeDisposition(testCase.code)
		if !ok || disposition != testCase.disposition {
			t.Fatalf("code %q disposition = %q ok=%t, want %q", testCase.code, disposition, ok, testCase.disposition)
		}
		if got := RuntimeCandidateRejectionTerminalCode(testCase.code); got != testCase.terminalCode {
			t.Fatalf("code %q terminal = %q, want %q", testCase.code, got, testCase.terminalCode)
		}
	}
	if _, ok := RuntimeCandidateRejectionCodeDisposition("candidate_correction_undefined"); ok {
		t.Fatal("candidate_correction_undefined must not remain an ordinary rejection code")
	}
}

func TestSealRuntimeCandidateRejectionCorrectableRequiresObjectives(t *testing.T) {
	t.Parallel()
	manifest := RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid",
		Diagnostic: &RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].kind", Actual: "shell/command", Allowed: []string{"patch/apply"}, ManifestHash: "manifest-hash",
		},
	}
	sealed, directive, err := SealRuntimeCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != RuntimeCandidateCorrectionCorrectableOnce || directive.TerminalCode != "" ||
		!reflect.DeepEqual(directive.Objectives, []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSelectOperationKind}) {
		t.Fatalf("correctable directive = %+v", directive)
	}
	if !RuntimeCandidateFailureIsCorrectableOnce(sealed) || RuntimeCandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("correctable classification drifted: %+v", sealed)
	}
}

func TestSealRuntimeCandidateRejectionWithoutObjectiveBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	// target outside without allowed set is not a safe same-manifest correction.
	failure := &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "outside",
		Diagnostic: &RuntimeCandidateFailureDiagnostic{Path: "$.operations[0].targetRefs", Actual: "other.go"},
	}
	sealed, directive, err := SealRuntimeCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal incomplete correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.Disposition != RuntimeCandidateCorrectionRejectedFinal ||
		directive.TerminalCode != "runtime_contract_defect" || len(directive.Objectives) != 0 {
		t.Fatalf("incomplete correctable must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
	if RuntimeCandidateFailureIsCorrectableOnce(sealed) || !RuntimeCandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("defect classification drifted: %+v", sealed)
	}
}

func TestSealRuntimeCandidateRejectionUnknownCodeBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassContract, Code: "candidate_correction_undefined", Signature: "undefined",
		Reason: "legacy catch-all must not remain ordinary",
	}
	sealed, directive, err := SealRuntimeCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal unknown = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.TerminalCode != "runtime_contract_defect" {
		t.Fatalf("unknown code must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
}

func TestSealRuntimeCandidateRejectionRejectedFinalHasTerminal(t *testing.T) {
	t.Parallel()
	manifest := RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &RuntimeCandidateFailure{
		Class: RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_candidate_shape", Signature: "replay-candidate",
	}
	sealed, directive, err := SealRuntimeCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal final = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != RuntimeCandidateCorrectionRejectedFinal || directive.TerminalCode != "candidate_retry_exhausted" ||
		len(directive.Objectives) != 0 || RuntimeCandidateFailureIsCorrectableOnce(sealed) {
		t.Fatalf("final directive = %+v sealed=%+v", directive, sealed)
	}
}
