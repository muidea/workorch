package workorchruntimecontract

import "testing"

func TestRuntimeOperationGovernanceGrantRequiresNextFenceAndExactBinding(t *testing.T) {
	grant := RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: "approval-1", ExecutionKey: "exec-1",
		GovernanceFence: 4, ResumeFence: 5, ResumeDispatchID: "resume-dispatch-1", DependencyKind: "approval", SubjectRef: "target.txt",
		RequestedAction: "file/write", AuthorizationHash: "authorization-hash",
		GovernanceReceiptHash: "receipt-hash", EvidenceRefs: []string{"approval://1"},
	}
	if err := grant.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := grant
	invalid.ResumeFence = grant.GovernanceFence + 2
	if err := invalid.Validate(); err == nil {
		t.Fatal("non-consecutive resume fence must be rejected")
	}
	binding := RuntimeOperationGovernanceBinding{
		ApprovalID: "approval-1", GovernanceDispatchID: "gov-dispatch", GovernanceSnapshotID: "gov-snapshot",
		OperationDispatchID: "operation-dispatch", OperationID: "op-1", ExecutionKey: "exec-1", Fence: 4,
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		ExecutionAuthorityHash: "execution-authority-hash", DependencyKind: "approval", SubjectRef: "target.txt",
		RequestedAction: "file/write", AuthorizationHash: "authorization-hash",
		GovernanceReceiptHash: "receipt-hash", State: RuntimeOperationGovernanceBindingGranted, Grant: &grant,
	}
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{binding}); err != nil {
		t.Fatal(err)
	}
	missingClass := binding.Clone()
	missingClass.IdempotencyClass = ""
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{missingClass}); err == nil {
		t.Fatal("governance binding accepted missing sealed idempotency")
	}
	invalidClass := binding.Clone()
	invalidClass.IdempotencyClass = "invalid"
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{invalidClass}); err == nil {
		t.Fatal("governance binding accepted invalid sealed idempotency")
	}
	drifted := binding.Clone()
	drifted.Grant.SubjectRef = "other.txt"
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{drifted}); err == nil {
		t.Fatal("grant subject drift must be rejected")
	}
}
