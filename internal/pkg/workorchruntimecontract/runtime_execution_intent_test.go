package workorchruntimecontract

import (
	"strings"
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
		{RuntimeExecutionIntentFreshUnresolved, RuntimeExecutionIntentReasonFreshResultUnknown},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonCreationFenceMissing},
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

func TestRuntimeExecutionIntentUnresolvedIdentityCannotAssertAcceptanceOrRun(t *testing.T) {
	a := RuntimeExecutionIntentAdmission{WorkspaceID: "ws", RequestID: "current", Kind: RuntimeExecutionIntentRestart,
		Outcome: RuntimeExecutionIntentFreshUnresolved, Reason: RuntimeExecutionIntentReasonFreshResultUnknown,
		PendingRequestID: "original", PendingStartupInputHash: "startup", PendingPayloadHash: strings.Repeat("a", 64)}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeExecutionIntentAdmission){
		"run":             func(v *RuntimeExecutionIntentAdmission) { v.RunID = "asserted-run" },
		"resume":          func(v *RuntimeExecutionIntentAdmission) { v.Kind = RuntimeExecutionIntentResume },
		"missing_request": func(v *RuntimeExecutionIntentAdmission) { v.PendingRequestID = "" },
		"control":         func(v *RuntimeExecutionIntentAdmission) { v.PendingRequestID = "old\nrequest" },
		"missing_startup": func(v *RuntimeExecutionIntentAdmission) { v.PendingStartupInputHash = "" },
		"missing_hash":    func(v *RuntimeExecutionIntentAdmission) { v.PendingPayloadHash = "" },
		"upper_hash":      func(v *RuntimeExecutionIntentAdmission) { v.PendingPayloadHash = strings.Repeat("A", 64) },
		"acceptance": func(v *RuntimeExecutionIntentAdmission) {
			v.Outcome = RuntimeExecutionIntentReconciliationRequested
			v.Reason = RuntimeExecutionIntentReasonNewRunActivated
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := a
			mutate(&invalid)
			if invalid.Validate() == nil {
				t.Fatal("invalid unresolved admission was accepted")
			}
		})
	}
}
