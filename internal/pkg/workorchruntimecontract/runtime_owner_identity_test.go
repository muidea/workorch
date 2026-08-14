package workorchruntimecontract

import "testing"

func TestRuntimeV8OwnerIdentitiesNormalizeValidateAndHash(t *testing.T) {
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
}

func TestRuntimeSchemaV7RequiresFreshActivation(t *testing.T) {
	err := ValidateRuntimeStateSchemaVersion(7)
	if err == nil {
		t.Fatal("schema v7 was accepted by the schema v8 writer")
	}
	incompatibility, ok := RuntimeStateIncompatibilityFromError(err)
	if !ok || incompatibility.ActualSchemaVersion != 7 || incompatibility.SupportedSchemaVersion != 8 || incompatibility.Reason != RuntimeStateIncompatibilityReasonSchemaUnsupported {
		t.Fatalf("schema v7 incompatibility = %+v ok=%v", incompatibility, ok)
	}
}
