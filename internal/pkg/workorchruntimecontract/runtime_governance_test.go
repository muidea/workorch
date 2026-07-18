package workorchruntimecontract

import "testing"

func TestRuntimeOperationGovernanceGrantRequiresNextFenceAndExactBinding(t *testing.T) {
	grant := RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", TicketID: "ticket-1", ExecutionKey: "exec-1",
		GovernanceFence: 4, ResumeFence: 5, DependencyKind: "approval", SubjectRef: "target.txt",
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
		TicketID: "ticket-1", GovernanceDispatchID: "gov-dispatch", GovernanceSnapshotID: "gov-snapshot",
		OperationDispatchID: "operation-dispatch", OperationID: "op-1", ExecutionKey: "exec-1", Fence: 4,
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable, DependencyKind: "approval", SubjectRef: "target.txt",
		GovernanceReceiptHash: "receipt-hash", State: RuntimeOperationGovernanceBindingGranted, Grant: &grant,
	}
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{binding}); err != nil {
		t.Fatal(err)
	}
	drifted := binding.Clone()
	drifted.Grant.SubjectRef = "other.txt"
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{drifted}); err == nil {
		t.Fatal("grant subject drift must be rejected")
	}
}
