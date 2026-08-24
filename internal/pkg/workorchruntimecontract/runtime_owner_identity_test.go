package workorchruntimecontract

import "testing"

func TestRuntimeOwnerIdentitiesNormalizeValidateAndHash(t *testing.T) {
	policy := RuntimeOwnerPolicyIdentity{OwnerModuleID: " blocks/file ", PolicyRevision: " file-policy-v1 ", CatalogSnapshotID: " catalog-1 "}.Normalized()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	input := RuntimeEquivalentAuthorityInputIdentity{
		DefinitionRevision: " def-1 ", FocusHash: " focus-1 ", BindingSetHash: " binding-set-1 ", EvidenceRevision: " evidence-1 ",
		CapabilityStateHash: " capability-1 ", CatalogRevisionHash: " catalog-1 ", PrerequisiteFreshnessHash: " fresh-1 ", OwnerObservationHash: " observation-1 ",
	}.Normalized()
	if err := input.Validate(); err != nil || StableRuntimeEquivalentAuthorityInputID(input) != StableRuntimeEquivalentAuthorityInputID(input.Normalized()) {
		t.Fatalf("authority input identity err=%v value=%+v", err, input)
	}
	contradiction := RuntimeBindingContradictionIdentity{
		WorkspaceID: "ws-1", RunID: "run-1", AuthorityInputID: StableRuntimeEquivalentAuthorityInputID(input), BindingID: "binding-1",
		BindingSetHash: "binding-set-1", ProposalHash: "proposal-1", ObservationHash: "observation-1", OwnerPolicy: policy, ObservedRevision: "revision-1",
	}
	if err := contradiction.Validate(); err != nil {
		t.Fatal(err)
	}
	fence := RuntimeOpaqueLifecycleFence{
		WorkspaceID: "ws-1", RunID: "run-1", RuntimeStateRevision: 7, RuntimeStateRef: "runtime-state/ws-1/run-1/7",
		RuntimeStateHash: "state-hash", BootstrapHash: "bootstrap-hash", TerminalFenceID: "terminal-1",
	}
	if err := fence.Validate(); err != nil {
		t.Fatal(err)
	}
	continuationFence := fence
	continuationFence.TerminalFenceID = ""
	continuationFence.GovernanceFenceID = " governance-1 "
	continuationFence.GovernanceResolutionID = " resolution-1 "
	continuationFence = continuationFence.Normalized()
	if err := continuationFence.Validate(); err != nil || continuationFence.GovernanceResolutionID != "resolution-1" {
		t.Fatalf("continuation fence=%+v err=%v", continuationFence, err)
	}
	invalidResolutionFence := fence
	invalidResolutionFence.GovernanceResolutionID = "resolution-without-governance"
	if err := invalidResolutionFence.Validate(); err == nil {
		t.Fatal("opaque resolution without governance fence was accepted")
	}
	dualModeFence := continuationFence
	dualModeFence.TerminalFenceID = "terminal-duplicate-mode"
	if err := dualModeFence.Validate(); err == nil {
		t.Fatal("governance and terminal lifecycle identities were accepted together")
	}
}
