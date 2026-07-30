package workorchruntimecontract

import (
	"strings"
	"testing"
)

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
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation", Fence: 1, OperationID: "operation",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectUnknown,
		Failure: &FailureFact{Class: FailureClassExecutionFailure, Code: "failed", Signature: "failure", Message: "failed", TargetRefs: []string{"implementation.go"}, TargetObservation: observation},
	}
	changed := receipt
	changed.Failure = &FailureFact{Class: receipt.Failure.Class, Code: receipt.Failure.Code, Signature: receipt.Failure.Signature, Message: receipt.Failure.Message, TargetRefs: append([]string(nil), receipt.Failure.TargetRefs...), TargetObservation: &RuntimeFailureTargetObservation{
		OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-2", TargetRefs: []string{"implementation.go"},
		DiagnosticClass: "build_failed", EvidenceRefs: []string{"workorch://mcp/failure/1"}, ContentHash: strings.Repeat("a", 64),
	}}
	if StableRuntimeOperationReceiptHash(receipt) == StableRuntimeOperationReceiptHash(changed) {
		t.Fatal("receipt hash did not include failure target observation")
	}
	directive := RuntimeRepairDirective{ID: "directive", FocusID: "focus", Trigger: RuntimeRepairTriggerExecutionRejected, RequiredState: ConvergenceStateNeedStateChange, CurrentFailure: receipt.Failure}
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
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation", Fence: 1, OperationID: "operation",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectUnknown,
		Failure: &FailureFact{
			Class: FailureClassExecutionFailure, Code: "failed", Signature: "failure", Message: "failed", TargetRefs: []string{"implementation.go"},
			TargetObservation: &RuntimeFailureTargetObservation{
				OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"other.go"},
			},
		},
	}
	if err := receipt.Validate(); err == nil {
		t.Fatal("receipt with mismatched observation target validated")
	}
	if err := ValidateRuntimeFailureTargetObservationBinding(*receipt.Failure); err == nil {
		t.Fatal("mismatched failure observation binding validated")
	}
}
