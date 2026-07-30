package events

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCapabilityOperationResultCarriesOnlyReceiptIdentity(t *testing.T) {
	results := []reflect.Type{
		reflect.TypeOf(ExecuteFileEditOperationResult{}), reflect.TypeOf(ExecuteFileWriteOperationResult{}), reflect.TypeOf(ExecuteFileListOperationResult{}),
		reflect.TypeOf(ExecuteFileReadOperationResult{}), reflect.TypeOf(ExecutePatchApplyOperationResult{}), reflect.TypeOf(ExecutePatchPreviewOperationResult{}),
		reflect.TypeOf(ExecuteShellOperationResult{}), reflect.TypeOf(ExecuteGitOperationResult{}), reflect.TypeOf(ExecuteMCPOperationResult{}),
		reflect.TypeOf(ExecuteSSHOperationResult{}), reflect.TypeOf(ExecuteWebOperationResult{}),
	}
	for _, typeOf := range results {
		for _, forbidden := range []string{"Status", "Data", "Evidence", "Failure"} {
			if _, found := typeOf.FieldByName(forbidden); found {
				t.Fatalf("%s has a second operation outcome field %q", typeOf.Name(), forbidden)
			}
		}
		if typeOf.NumField() != 5 {
			t.Fatalf("%s field count = %d, want receipt identity only", typeOf.Name(), typeOf.NumField())
		}
	}
}

func TestRuntimeOperationExecutionEnvelopeValidatesGovernanceGrantIdentity(t *testing.T) {
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", TicketID: common.TicketID("ticket-1"),
		ExecutionKey: "exec/ws/run/dispatch/op", GovernanceFence: 1, ResumeFence: 2,
		DependencyKind: "approval", SubjectRef: "target.txt", GovernanceReceiptHash: "receipt-hash",
	}
	envelope := RuntimeOperationExecutionEnvelope{
		ExecutionKey: grant.ExecutionKey, Fence: grant.ResumeFence,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, GovernanceGrant: &grant,
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("valid governance grant envelope: %v", err)
	}
	stale := grant
	stale.ResumeFence++
	envelope.GovernanceGrant = &stale
	if err := envelope.Validate(); err == nil {
		t.Fatal("stale governance grant fence must be rejected")
	}
}

func TestNewRuntimeOperationReceiptCarriesExecutionFence(t *testing.T) {
	execution := RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 3,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
	}
	receipt := NewRuntimeOperationReceipt(execution, "op-1", runtimecontract.RuntimeOperationExecutionStatusAccepted, nil,
		runtimecontract.RuntimeOperationEvidence{Summary: "changed"}, nil, nil, "2026-07-20T00:00:00Z")
	if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
		t.Fatalf("receipt identity = %+v", receipt)
	}
	if receipt.CompletedAt != "2026-07-20T00:00:00Z" {
		t.Fatalf("completedAt = %q", receipt.CompletedAt)
	}
}

func TestAttachRuntimeOperationReceiptFeedbackPreservesOwnerDiagnostic(t *testing.T) {
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_code",
		Signature: "owner/signature", Message: "original owner message", TargetRefs: []string{"a", "b"},
	}
	receipt := NewRuntimeOperationReceipt(RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
	}, "op-1", runtimecontract.RuntimeOperationExecutionStatusRejected, []string{"target.txt"}, runtimecontract.RuntimeOperationEvidence{}, failure, nil, "2026-07-20T00:00:00Z")
	attached, err := AttachRuntimeOperationReceiptFeedback(receipt, runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1",
	}, runtimecontract.RuntimeModuleFile)
	if err != nil {
		t.Fatal(err)
	}
	if attached.Feedback == nil || attached.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation || attached.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected) ||
		attached.Feedback.WorkspaceID != "ws" || attached.Feedback.RunID != "run" || attached.Feedback.AuthoritySnapshotID != "authority-1" || attached.Feedback.DispatchID != "dispatch-1" ||
		attached.Feedback.OriginalDiagnostic == nil || attached.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleFile || attached.Feedback.OriginalDiagnostic.OperationID != "op-1" ||
		attached.Feedback.OriginalDiagnostic.OriginalCode != failure.Code || attached.Feedback.OriginalDiagnostic.OriginalMessage != failure.Message || len(attached.Feedback.OriginalDiagnostic.TargetRefs) != 2 {
		t.Fatalf("operation feedback lost original diagnostic: %+v", attached.Feedback)
	}
	clone := attached.Clone()
	if runtimecontract.StableRuntimeOperationReceiptHash(clone) != runtimecontract.StableRuntimeOperationReceiptHash(attached) {
		t.Fatal("receipt feedback hash changed across clone")
	}
	clone.Feedback.OriginalDiagnostic.OriginalMessage = "changed"
	if attached.Feedback.OriginalDiagnostic.OriginalMessage != failure.Message {
		t.Fatal("receipt clone aliases original diagnostic")
	}
}
