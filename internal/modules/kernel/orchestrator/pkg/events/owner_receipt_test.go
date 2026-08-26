package events

import (
	"errors"
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

func validOwnerReceiptResultFixture() runtimecontract.RuntimeOperationReceipt {
	return runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 2, OperationID: "op-1",
		Outcome: runtimecontract.RuntimeOperationOutcomeNoOp, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "no change"},
	}
}

func TestValidateQueryRuntimeOperationReceiptResultRejectsMalformedAndStaleIdentity(t *testing.T) {
	valid := validOwnerReceiptResultFixture()
	wrongExecution := valid
	wrongExecution.ExecutionKey = "exec/other"
	wrongFence := valid
	wrongFence.Fence++
	wrongOperation := valid
	wrongOperation.OperationID = "op-other"
	invalid := valid
	invalid.Outcome = "invalid"

	tests := []struct {
		name      string
		result    QueryRuntimeOperationReceiptResult
		wantFound bool
		wantErr   bool
	}{
		{name: "authorized not found", result: QueryRuntimeOperationReceiptResult{}},
		{name: "exact persisted receipt", result: QueryRuntimeOperationReceiptResult{Found: true, Receipt: &valid}, wantFound: true},
		{name: "found without receipt", result: QueryRuntimeOperationReceiptResult{Found: true}, wantErr: true},
		{name: "receipt while not found", result: QueryRuntimeOperationReceiptResult{Receipt: &valid}, wantErr: true},
		{name: "invalid receipt", result: QueryRuntimeOperationReceiptResult{Found: true, Receipt: &invalid}, wantErr: true},
		{name: "wrong execution key", result: QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongExecution}, wantErr: true},
		{name: "wrong fence", result: QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongFence}, wantErr: true},
		{name: "wrong operation", result: QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongOperation}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt, found, err := ValidateQueryRuntimeOperationReceiptResult(test.result, valid.ExecutionKey, valid.Fence, valid.OperationID)
			if test.wantErr {
				if !errors.Is(err, common.ErrConflict) || found {
					t.Fatalf("found=%v receipt=%+v err=%v, want conflict", found, receipt, err)
				}
				return
			}
			if err != nil || found != test.wantFound {
				t.Fatalf("found=%v receipt=%+v err=%v", found, receipt, err)
			}
			if found && !runtimecontract.SameRuntimeOperationReceipt(receipt, valid) {
				t.Fatalf("receipt=%+v, want %+v", receipt, valid)
			}
		})
	}
}

func TestValidateCommitRuntimeOperationReceiptResultRequiresExactReceipt(t *testing.T) {
	valid := validOwnerReceiptResultFixture()
	invalid := valid
	invalid.Outcome = "invalid"
	drifted := valid
	drifted.Evidence.Summary = "different"

	for _, test := range []struct {
		name    string
		result  CommitRuntimeOperationReceiptResult
		wantErr bool
	}{
		{name: "exact", result: CommitRuntimeOperationReceiptResult{Receipt: valid, StateRevision: 7}},
		{name: "missing", result: CommitRuntimeOperationReceiptResult{}, wantErr: true},
		{name: "invalid", result: CommitRuntimeOperationReceiptResult{Receipt: invalid}, wantErr: true},
		{name: "payload drift", result: CommitRuntimeOperationReceiptResult{Receipt: drifted}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := ValidateCommitRuntimeOperationReceiptResult(test.result, valid)
			if test.wantErr {
				if !errors.Is(err, common.ErrConflict) {
					t.Fatalf("receipt=%+v err=%v, want conflict", receipt, err)
				}
				return
			}
			if err != nil || !runtimecontract.SameRuntimeOperationReceipt(receipt, valid) {
				t.Fatalf("receipt=%+v err=%v", receipt, err)
			}
		})
	}
}

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
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: common.ApprovalID("approval-1"),
		ExecutionKey: "exec/ws/run/dispatch/op", GovernanceFence: 1, ResumeFence: 2, ResumeDispatchID: "dispatch",
		DependencyKind: "approval", SubjectRef: "target.txt", RequestedAction: "file/write", AuthorizationHash: "authorization-hash", GovernanceReceiptHash: "receipt-hash",
	}
	envelope := runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: grant.ExecutionKey, Fence: grant.ResumeFence,
		OperationHash:       runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "op", Kind: runtimecontract.StepKindFileWrite}),
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    runtimecontract.RuntimeOperationIdempotencyAtMostOnce, GovernanceGrant: &grant,
	}
	envelope.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "op", OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey,
		ExpectedEffectScope: envelope.ExpectedEffectScope, IdempotencyClass: envelope.IdempotencyClass,
	})
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
		OperationHash:       runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "op-1", Kind: runtimecontract.StepKindFileWrite}),
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
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
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 1,
		OperationHash:       runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "op-1", Kind: runtimecontract.StepKindFileRead}),
		ExpectedEffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
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
