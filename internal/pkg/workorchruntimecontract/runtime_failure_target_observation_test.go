package workorchruntimecontract

import (
	"strings"
	"testing"
)

func testRejectedOperationFeedback(t *testing.T, operationID string, failure FailureFact) *RuntimeFeedback {
	t.Helper()
	ownerModuleID := RuntimeModuleFile
	if failure.TargetObservation != nil && failure.TargetObservation.OwnerModuleID != "" {
		ownerModuleID = failure.TargetObservation.OwnerModuleID
	}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch"}, RuntimeFeedbackDomainOperation, RuntimeFeedbackCategory(RuntimeOperationOutcomeRejected), ownerModuleID, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = operationID
	return &feedback
}

func TestRuntimeFailureTargetObservationNormalizesAndValidates(t *testing.T) {
	value := RuntimeFailureTargetObservation{
		OwnerModuleID: " blocks/mcp ", SubjectRef: " server-a ", TargetRole: " implementation ",
		ObservedRevision: " revision-1 ", TargetRefs: []string{" b.go ", "a.go", "a.go", ""},
		DiagnosticClass: " build_failed ", EvidenceRefs: []string{" evidence-b ", "evidence-a", "evidence-a"}, ContentHash: strings.Repeat("a", 64),
	}.Normalized()
	if value.OwnerModuleID != "blocks/mcp" || value.SubjectRef != "server-a" || value.TargetRole != "implementation" ||
		value.ObservedRevision != "revision-1" || len(value.TargetRefs) != 2 || value.TargetRefs[0] != "a.go" || value.TargetRefs[1] != "b.go" ||
		value.DiagnosticClass != "build_failed" || len(value.EvidenceRefs) != 2 || value.EvidenceRefs[0] != "evidence-a" || value.ContentHash != strings.Repeat("a", 64) {
		t.Fatalf("normalized observation = %+v", value)
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	for _, invalid := range []RuntimeFailureTargetObservation{
		{SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"a.go"}},
		{OwnerModuleID: "blocks/mcp", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"a.go"}},
		{OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", ObservedRevision: "revision-1", TargetRefs: []string{"a.go"}},
		{OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", TargetRefs: []string{"a.go"}},
		{OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1"},
		{OwnerModuleID: "blocks/shell", SubjectRef: "shell_runtime_policy", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"a.go"}, DiagnosticClass: "build_failed"},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("Validate(%+v) = nil, want error", invalid)
		}
	}
}

func TestFailureTargetObservationParticipatesInReceiptHashAndDirectiveEquality(t *testing.T) {
	observation := &RuntimeFailureTargetObservation{
		OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"implementation.go"},
		DiagnosticClass: "build_failed", EvidenceRefs: []string{"workorch://mcp/failure/1"}, ContentHash: strings.Repeat("a", 64),
	}
	failure := FailureFact{Class: FailureClassExecutionFailure, Code: "failed", Signature: "failure", Message: "failed", TargetRefs: []string{"implementation.go"}, TargetObservation: observation}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation", Fence: 1, OperationID: "operation",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectUnknown,
		Feedback: testRejectedOperationFeedback(t, "operation", failure),
	}
	changedFailure := failure.Clone()
	changedFailure.TargetObservation = &RuntimeFailureTargetObservation{
		OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-2", TargetRefs: []string{"implementation.go"},
		DiagnosticClass: "build_failed", EvidenceRefs: []string{"workorch://mcp/failure/1"}, ContentHash: strings.Repeat("a", 64),
	}
	changed := receipt
	changed.Feedback = testRejectedOperationFeedback(t, "operation", changedFailure)
	if StableRuntimeOperationReceiptHash(receipt) == StableRuntimeOperationReceiptHash(changed) {
		t.Fatal("receipt hash did not include failure target observation")
	}
	directive := RuntimeRepairDirective{ID: "directive", FocusID: "focus", Trigger: RuntimeRepairTriggerExecutionRejected, RequiredState: ConvergenceStateNeedStateChange, CurrentFailure: &failure}
	clone := directive.Clone()
	if !SameRuntimeRepairDirective(&directive, &clone) {
		t.Fatalf("clone is not equal: left=%+v right=%+v", directive, clone)
	}
	clone.CurrentFailure.TargetObservation.TargetRefs[0] = "other.go"
	if directive.CurrentFailure.TargetObservation.TargetRefs[0] != "implementation.go" {
		t.Fatalf("clone mutated original observation: %+v", directive.CurrentFailure.TargetObservation)
	}
	clone = directive.Clone()
	clone.CurrentFailure.TargetObservation.EvidenceRefs[0] = "workorch://changed"
	if directive.CurrentFailure.TargetObservation.EvidenceRefs[0] != "workorch://mcp/failure/1" {
		t.Fatalf("clone mutated original observation evidence refs: %+v", directive.CurrentFailure.TargetObservation)
	}
	if SameRuntimeRepairDirective(&directive, &clone) {
		t.Fatal("directive equality ignored failure target observation")
	}
}

func TestFailureTargetObservationBindingRejectsMismatchedReceiptTargets(t *testing.T) {
	failure := FailureFact{
		Class: FailureClassExecutionFailure, Code: "failed", Signature: "failure", Message: "failed", TargetRefs: []string{"implementation.go"},
		TargetObservation: &RuntimeFailureTargetObservation{
			OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"other.go"},
		},
	}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation", Fence: 1, OperationID: "operation",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectUnknown,
		Feedback: testRejectedOperationFeedback(t, "operation", failure),
	}
	if err := receipt.Validate(); err == nil {
		t.Fatal("receipt with mismatched observation target validated")
	}
	if err := ValidateRuntimeFailureTargetObservationBinding(failure); err == nil {
		t.Fatal("mismatched failure observation binding validated")
	}
}

func TestRejectedOperationReceiptPreservesTypedDiagnosticAndOmitsRawValidationOutput(t *testing.T) {
	failure := FailureFact{
		Class: FailureClassBuildFailure, Code: "validation_failed", Signature: "validation-failed", Message: "validation failed",
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &RuntimeFailureTargetObservation{
			OwnerModuleID: RuntimeModuleShell, SubjectRef: "shell-policy", TargetRole: "implementation",
			ObservedRevision: "policy-1", TargetRefs: []string{"implementation.go"}, DiagnosticClass: ValidationDiagnosticBuildFailed,
			EvidenceRefs: []string{"workorch://shell/validation/validate"}, ContentHash: strings.Repeat("b", 64),
		},
		ValidationDiagnostic: &ValidationFailureDiagnostic{
			SchemaVersion: ValidationFailureDiagnosticSchemaVersion, Class: ValidationDiagnosticBuildFailed,
			Reason: "build failed", Operation: StepKindShellCommand, TargetRefs: []string{"implementation.go"},
			StdoutPreview: "raw stdout", StderrPreview: "raw stderr",
		},
		ValidationFailureObservation: &RuntimeValidationFailureObservation{
			WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate",
			FailureSignature: "validation-failed", TargetRole: "implementation", DiagnosticClass: ValidationDiagnosticBuildFailed,
			CandidateTargetRefs: []string{"implementation.go"}, ValidationScopeTargetRefs: []string{"implementation.go"}, DiagnosticHash: strings.Repeat("a", 64),
		},
	}
	feedback := testRejectedOperationFeedback(t, "validate", failure)
	diagnostic := feedback.OriginalDiagnostic
	if diagnostic == nil || diagnostic.ValidationDiagnostic == nil || diagnostic.ValidationFailureObservation == nil || diagnostic.TargetObservation == nil {
		t.Fatalf("typed diagnostic was not preserved: %+v", diagnostic)
	}
	if diagnostic.ValidationDiagnostic.StdoutPreview != "" || diagnostic.ValidationDiagnostic.StderrPreview != "" ||
		diagnostic.OmittedReason != "raw_validation_output" || !diagnostic.Redacted {
		t.Fatalf("raw validation output was not safely omitted: %+v", diagnostic)
	}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/validate", Fence: 1, OperationID: "validate",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectNone, Feedback: feedback,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("receipt validation: %v", err)
	}
	unsafe := receipt.Clone()
	unsafe.Feedback.OriginalDiagnostic.ValidationDiagnostic.StdoutPreview = "raw stdout"
	if err := unsafe.Validate(); err == nil {
		t.Fatal("receipt accepted raw validation output in original diagnostic")
	}
	reloaded := receipt.Clone()
	if reloaded.Feedback == nil || StableRuntimeFeedbackHash(*receipt.Feedback) != StableRuntimeFeedbackHash(*reloaded.Feedback) ||
		StableRuntimeOperationReceiptHash(receipt) != StableRuntimeOperationReceiptHash(reloaded) {
		t.Fatalf("receipt reload lost original diagnostic: before=%+v after=%+v", receipt.Feedback, reloaded.Feedback)
	}
}
