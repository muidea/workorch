package workorchruntimecontract

import "testing"

func TestRuntimeExecutionIntentAdmissionIsExhaustiveAndPreservesReason(t *testing.T) {
	cases := []struct {
		outcome RuntimeExecutionIntentOutcome
		reason  RuntimeExecutionIntentReason
	}{
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonCurrentRunContinuationAccepted},
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonPendingRunActivated},
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonNewRunActivated},
		{RuntimeExecutionIntentReconciliationRequested, RuntimeExecutionIntentReasonFreshActivationReplacement},
		{RuntimeExecutionIntentReplacementPending, RuntimeExecutionIntentReasonDuplicateRestart},
		{RuntimeExecutionIntentReplacementPending, RuntimeExecutionIntentReasonPredecessorSupersedeRequested},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonSelectedRunAbsent},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonSelectedRunTerminal},
		{RuntimeExecutionIntentRestartRequired, RuntimeExecutionIntentReasonRunNotResumable},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonRuntimeSchemaIncompatible},
		{RuntimeExecutionIntentFreshActivationRequired, RuntimeExecutionIntentReasonRuntimeBootstrapMissing},
		{RuntimeExecutionIntentDefinitionChanged, RuntimeExecutionIntentReasonDefinitionChanged},
		{RuntimeExecutionIntentGovernanceRequired, RuntimeExecutionIntentReasonGovernancePending},
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
