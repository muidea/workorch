package events

import (
	"reflect"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCapabilityOperationResultCarriesOnlyReceiptIdentity(t *testing.T) {
	results := []reflect.Type{
		reflect.TypeOf(fileevents.ExecuteOperationResult{}),
		reflect.TypeOf(shellevents.ExecuteOperationResult{}), reflect.TypeOf(gitevents.ExecuteOperationResult{}), reflect.TypeOf(mcpevents.ExecuteOperationResult{}),
		reflect.TypeOf(sshevents.ExecuteOperationResult{}), reflect.TypeOf(webevents.ExecuteOperationResult{}),
	}
	for _, typeOf := range results {
		for _, forbidden := range []string{"Status", "Data", "Evidence", "Failure", "EventID", "OperationID", "EvidenceKind", "SourceRef"} {
			if _, found := typeOf.FieldByName(forbidden); found {
				t.Fatalf("%s has a second operation outcome field %q", typeOf.Name(), forbidden)
			}
		}
		if typeOf.NumField() != 1 {
			t.Fatalf("%s field count = %d, want canonical receipt only", typeOf.Name(), typeOf.NumField())
		}
	}
}

func TestRuntimeOperationExecutionEnvelopeValidatesGovernanceGrantIdentity(t *testing.T) {
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ApprovalID: common.ApprovalID("approval-1"),
		ExecutionKey: "exec/ws/run/dispatch/op", GovernanceFence: 1, ResumeFence: 2,
		DependencyKind: "approval", SubjectRef: "target.txt", RequestedAction: "file/write", AuthorizationHash: "authorization-hash", GovernanceReceiptHash: "receipt-hash",
	}
	envelope := runtimecontract.RuntimeOperationExecutionEnvelope{
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
	execution := runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 3,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
	}
	receipt := NewRuntimeOperationReceipt(execution, runtimecontract.RuntimeModuleFile, "op-1", runtimecontract.RuntimeOperationExecutionStatusAccepted, nil,
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
	receipt := NewRuntimeOperationReceipt(runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
	}, runtimecontract.RuntimeModuleFile, "op-1", runtimecontract.RuntimeOperationExecutionStatusRejected, []string{"target.txt"}, runtimecontract.RuntimeOperationEvidence{}, failure, nil, "2026-07-20T00:00:00Z")
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
