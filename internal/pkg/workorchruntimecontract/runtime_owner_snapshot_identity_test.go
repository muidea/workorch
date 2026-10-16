package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeReceiptOwnerSnapshotIdentityHashRawCloneAndReload(t *testing.T) {
	set, _ := ownerDependencyUnionFixture()
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.OwnerRevisionSet = &set
	digest := strings.Repeat("a", 64)
	receipt.Evidence.ContentHash = digest
	receipt.Evidence.ContentAddress = &RuntimeEvidenceContentAddress{Algorithm: RuntimeEvidenceDigestSHA256, Digest: digest, Ref: "original-archive"}
	receipt.Evidence.EvidenceRefs = []string{"original-archive"}
	legacy, _ := json.Marshal(receipt)
	legacyHash := StableRuntimeOperationReceiptHash(receipt)
	if strings.Contains(string(legacy), "OwnerSnapshot") {
		t.Fatal("absent snapshot changed legacy encoding")
	}
	receipt.Evidence.OwnerSnapshot = &RuntimeOwnerSnapshotIdentity{ProtocolRef: RuntimeTerminalOwnerSnapshotProtocol, SnapshotID: "original-snapshot", RootSnapshotID: "original-root"}
	if receipt.Validate() != nil || StableRuntimeOperationReceiptHash(receipt) == legacyHash {
		t.Fatal("original snapshot not bound to receipt")
	}
	for _, mutate := range []func(*RuntimeOperationReceipt){
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.SnapshotID = "other" },
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.RootSnapshotID = "other" },
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.ProtocolRef = "other" },
	} {
		copy := receipt.Clone()
		mutate(&copy)
		if StableRuntimeOperationReceiptHash(copy) == StableRuntimeOperationReceiptHash(receipt) {
			t.Fatal("receipt omitted full snapshot identity")
		}
	}
	for _, copy := range []RuntimeOperationReceipt{receipt.Clone(), receipt.Normalized(), cloneRuntimeOperationReceiptRaw(receipt)} {
		copy.Evidence.OwnerSnapshot.SnapshotID = "caller"
		if receipt.Evidence.OwnerSnapshot.SnapshotID == "caller" {
			t.Fatal("snapshot pointer aliases original receipt")
		}
	}
	body, _ := json.Marshal(receipt)
	var reload RuntimeOperationReceipt
	if json.Unmarshal(body, &reload) != nil || reload.Validate() != nil || !reflect.DeepEqual(receipt, reload) {
		t.Fatal("immutable snapshot identity lost on reload")
	}
	for _, mutate := range []func(*RuntimeOperationReceipt){
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.SnapshotID = " original-snapshot" },
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.RootSnapshotID = "" },
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerSnapshot.ProtocolRef = "unknown" },
		func(r *RuntimeOperationReceipt) { r.Evidence.ContentAddress = nil },
		func(r *RuntimeOperationReceipt) { r.Evidence.OwnerRevisionSet = nil },
	} {
		bad := receipt.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("unanchored/noncanonical snapshot accepted")
		}
	}
}

func TestRuntimeTerminalSnapshotRuleClosedVersionAndSourceUnion(t *testing.T) {
	rule := RuntimeLineageAssertionRule{Ref: "final", Kind: RuntimeLineageTerminalOwnerSnapshot, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalProofContractV2, OwnerModuleID: RuntimeModuleGit, OperationKind: StepKindGitWrite, OperationVariantID: "commit", RequiredEffectScope: RuntimeEffectWorkspaceDurable, ReceiptSourceScope: "deliverable", ReceiptSourceID: "repository", ValidationSourceScope: "dodItem", ValidationSourceID: "behavior"}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeLineageAssertionRule){
		func(r *RuntimeLineageAssertionRule) { r.TerminalContractRef = RuntimeTerminalProofContractV1 },
		func(r *RuntimeLineageAssertionRule) { r.ValidationSourceID = "" },
		func(r *RuntimeLineageAssertionRule) { r.ValidationSourceScope = "*" },
		func(r *RuntimeLineageAssertionRule) {
			r.OwnerModuleID = RuntimeModuleFile
			r.OperationKind = StepKindFileWrite
			r.OperationVariantID = "file_write"
		},
		func(r *RuntimeLineageAssertionRule) {
			r.Kind = RuntimeLineageTerminalOwnerReceipt
			r.TerminalContractRef = RuntimeTerminalProofContractV1
		},
		func(r *RuntimeLineageAssertionRule) { r.ConsumptionPoint = RuntimeProofPhaseExecution },
	} {
		bad := rule
		mutate(&bad)
		if bad.Validate() == nil || StableRuntimeLineageAssertionRuleHash(bad) == StableRuntimeLineageAssertionRuleHash(rule) {
			t.Fatal("unsupported/downgraded snapshot rule admitted")
		}
	}
}
