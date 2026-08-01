package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateRejectionCodeDispositionMatrixIsTotal(t *testing.T) {
	t.Parallel()
	codes := []struct {
		code         string
		disposition  runtimecontract.RuntimeCandidateCorrectionDisposition
		terminalCode string
	}{
		{code: "candidate_contract_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_schema_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_partially_replays_rejected_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_no_progress_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_action_evidence_mismatch", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_target_outside_grounding", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_creation_scope_missing_file_target", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_candidate_shape", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "candidate_retry_exhausted", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "workspace_resolution_failed", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "workspace_resolution_failed"},
		{code: "operation_feasibility_probe_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "operation_feasibility_probe_invalid"},
		{code: "runtime_contract_defect", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "runtime_contract_defect"},
	}
	for _, testCase := range codes {
		disposition, ok := CandidateRejectionCodeDisposition(testCase.code)
		if !ok || disposition != testCase.disposition {
			t.Fatalf("code %q disposition = %q ok=%t, want %q", testCase.code, disposition, ok, testCase.disposition)
		}
		if got := CandidateRejectionTerminalCode(testCase.code); got != testCase.terminalCode {
			t.Fatalf("code %q terminal = %q, want %q", testCase.code, got, testCase.terminalCode)
		}
	}
	if _, ok := CandidateRejectionCodeDisposition("candidate_correction_undefined"); ok {
		t.Fatal("candidate_correction_undefined must not remain an ordinary rejection code")
	}
	if _, ok := CandidateRejectionCodeDisposition("operation_target_not_regular"); ok {
		t.Fatal("owner feasibility code must not enter the candidate correction matrix")
	}
}

func TestCandidateCorrectionObjectivesRequireAllowedOperationKind(t *testing.T) {
	t.Parallel()
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: "provider-private decoder detail",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].kind", Actual: "shell/command", Allowed: []string{"file/write", "patch/apply"},
		},
	}
	objectives := CandidateCorrectionObjectivesForFailure(&failure)
	if len(objectives) != 1 || objectives[0] != runtimecontract.RuntimeCandidateCorrectionSelectOperationKind {
		t.Fatalf("objectives = %+v, want select allowed operation kind", objectives)
	}
}

func TestSealCandidateRejectionCorrectableRequiresObjectives(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].kind", Actual: "shell/command", Allowed: []string{"patch/apply"}, ManifestHash: "manifest-hash",
		},
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || directive.TerminalCode != "" ||
		!reflect.DeepEqual(directive.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSelectOperationKind}) {
		t.Fatalf("correctable directive = %+v", directive)
	}
	if !CandidateFailureIsCorrectableOnce(sealed) || CandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("correctable classification drifted: %+v", sealed)
	}
}

func TestSealCandidateRejectionWithoutObjectiveBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "outside",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: "$.operations[0].targetRefs", Actual: "other.go"},
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal incomplete correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal ||
		directive.TerminalCode != "runtime_contract_defect" || len(directive.Objectives) != 0 {
		t.Fatalf("incomplete correctable must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
	if CandidateFailureIsCorrectableOnce(sealed) || !CandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("defect classification drifted: %+v", sealed)
	}
}

func TestSealCandidateRejectionUnknownCodeBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_correction_undefined", Signature: "undefined",
		Reason: "legacy catch-all must not remain ordinary",
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal unknown = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.TerminalCode != "runtime_contract_defect" {
		t.Fatalf("unknown code must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
}

func TestSealCandidateRejectionRejectedFinalHasTerminal(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_candidate_shape", Signature: "replay-candidate",
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal final = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || directive.TerminalCode != "candidate_retry_exhausted" ||
		len(directive.Objectives) != 0 || CandidateFailureIsCorrectableOnce(sealed) {
		t.Fatalf("final directive = %+v sealed=%+v", directive, sealed)
	}
}
