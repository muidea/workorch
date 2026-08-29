package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeExecutionIntentAdmissionNormalizesAndValidatesCanonicalFact(t *testing.T) {
	admission := RuntimeExecutionIntentAdmission{
		WorkspaceID: common.WorkspaceID("ws-admission"),
		RunID:       common.RunID("run-admission"),
		RequestID:   " request-admission ",
		Kind:        RuntimeExecutionIntentKind(" resume "),
		Outcome:     RuntimeExecutionIntentOutcome(" reconciliation_requested "),
		Reason:      RuntimeExecutionIntentReason(" current_run_continuation_accepted "),
	}.Normalized()
	if admission.RequestID != "request-admission" || admission.Kind != RuntimeExecutionIntentResume ||
		admission.Outcome != RuntimeExecutionIntentReconciliationRequested ||
		admission.Reason != RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("normalized admission = %+v", admission)
	}
	if err := admission.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeExecutionIntentAdmission){
		"workspace": func(value *RuntimeExecutionIntentAdmission) { value.WorkspaceID = "" },
		"request":   func(value *RuntimeExecutionIntentAdmission) { value.RequestID = "" },
		"kind":      func(value *RuntimeExecutionIntentAdmission) { value.Kind = "unknown" },
		"pair": func(value *RuntimeExecutionIntentAdmission) {
			value.Reason = RuntimeExecutionIntentReasonRunNotResumable
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := admission
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("invalid admission accepted: %+v", invalid)
			}
		})
	}
}

func TestRuntimeExecutionIntentAdmissionIsExhaustiveAndPreservesReason(t *testing.T) {
	cases := []struct {
		outcome RuntimeExecutionIntentOutcome
		reason  RuntimeExecutionIntentReason
	}{
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonCurrentRunContinuationAccepted},
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonPendingRunActivated},
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonNewRunActivated},
		{RuntimeExecutionIntentReplacementPending, RuntimeExecutionIntentReasonDuplicateRestart},
		{RuntimeExecutionIntentReplacementPending, RuntimeExecutionIntentReasonPredecessorSupersedeRequested},
		{RuntimeExecutionIntentReplacementPending, RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonSelectedRunAbsent},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonSelectedRunTerminal},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonRunNotResumable},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonRuntimeBootstrapMissing},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonDefinitionAdmissionMissing},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonDefinitionAdmissionMismatch},
		{RuntimeExecutionIntentDefinitionChanged, RuntimeExecutionIntentReasonDefinitionChanged},
		{RuntimeExecutionIntentGovernanceRequired, RuntimeExecutionIntentReasonGovernancePending},
		{RuntimeExecutionIntentGovernanceRequired, RuntimeExecutionIntentReasonDefinitionClarificationPending},
	}
	for _, testCase := range cases {
		if err := ValidateRuntimeExecutionIntentAdmission(testCase.outcome, testCase.reason); err != nil {
			t.Fatalf("valid outcome=%q reason=%q: %v", testCase.outcome, testCase.reason, err)
		}
		if RuntimeExecutionIntentReasonSummary(testCase.reason) == "" {
			t.Fatalf("reason %q lost its presentation summary", testCase.reason)
		}
	}
	if err := ValidateRuntimeExecutionIntentAdmission(RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonRunNotResumable); err == nil {
		t.Fatal("mismatched outcome/reason was accepted")
	}
	if summary := RuntimeExecutionIntentReasonSummary("unknown"); summary != "" {
		t.Fatalf("unknown reason summary = %q", summary)
	}
}
