package workorchruntimecontract

import "testing"

func TestRuntimeFailureTargetObservationNormalizesAndValidates(t *testing.T) {
	value := RuntimeFailureTargetObservation{
		OwnerModuleID: " blocks/mcp ", SubjectRef: " server-a ", TargetRole: " implementation ",
		ObservedRevision: " revision-1 ", TargetRefs: []string{" b.go ", "a.go", "a.go", ""},
	}.Normalized()
	if value.OwnerModuleID != "blocks/mcp" || value.SubjectRef != "server-a" || value.TargetRole != "implementation" ||
		value.ObservedRevision != "revision-1" || len(value.TargetRefs) != 2 || value.TargetRefs[0] != "a.go" || value.TargetRefs[1] != "b.go" {
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
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("Validate(%+v) = nil, want error", invalid)
		}
	}
}

func TestFailureTargetObservationParticipatesInReceiptHashAndDirectiveEquality(t *testing.T) {
	observation := &RuntimeFailureTargetObservation{
		OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-1", TargetRefs: []string{"implementation.go"},
	}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/operation", Fence: 1, OperationID: "operation",
		Outcome: RuntimeOperationOutcomeRejected, ObservedEffectScope: RuntimeEffectUnknown,
		Failure: &FailureFact{Class: FailureClassExecutionFailure, Code: "failed", Signature: "failure", Message: "failed", TargetRefs: []string{"implementation.go"}, TargetObservation: observation},
	}
	changed := receipt
	changed.Failure = &FailureFact{Class: receipt.Failure.Class, Code: receipt.Failure.Code, Signature: receipt.Failure.Signature, Message: receipt.Failure.Message, TargetRefs: append([]string(nil), receipt.Failure.TargetRefs...), TargetObservation: &RuntimeFailureTargetObservation{
		OwnerModuleID: "blocks/mcp", SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "revision-2", TargetRefs: []string{"implementation.go"},
	}}
	if StableRuntimeOperationReceiptHash(receipt) == StableRuntimeOperationReceiptHash(changed) {
		t.Fatal("receipt hash did not include failure target observation")
	}
	directive := RuntimeRepairDirective{ID: "directive", FocusID: "focus", Trigger: RuntimeRepairTriggerExecutionRejected, RequiredAction: RuntimeActionProduceStateChange, CurrentFailure: receipt.Failure}
	clone := directive.Clone()
	if !SameRuntimeRepairDirective(&directive, &clone) {
		t.Fatalf("clone is not equal: left=%+v right=%+v", directive, clone)
	}
	clone.CurrentFailure.TargetObservation.TargetRefs[0] = "other.go"
	if directive.CurrentFailure.TargetObservation.TargetRefs[0] != "implementation.go" {
		t.Fatalf("clone mutated original observation: %+v", directive.CurrentFailure.TargetObservation)
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
