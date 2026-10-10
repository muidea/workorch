package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeReceiptOwnerDependenciesPreserveRawHashAndCloneBoundaries(t *testing.T) {
	set, _ := ownerDependencyUnionFixture()
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.OwnerRevisionSet = &set
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	fact := EvidenceFact{ID: "evidence", ContentHash: receipt.Evidence.ContentHash, OwnerRevisionSet: CloneRuntimeOwnerRevisionSet(&set)}
	originalReceiptHash, originalFactHash := StableRuntimeOperationReceiptHash(receipt), StableRuntimeEvidenceFactHash(fact)
	for _, scenario := range []string{"definition", "owner", "resource", "revision", "snapshot", "fence", "dependency", "set_hash"} {
		t.Run(scenario, func(t *testing.T) {
			changed := receipt.Clone()
			dependency := changed.Evidence.OwnerRevisionSet
			switch scenario {
			case "definition":
				dependency.DefinitionRevision = "other"
			case "owner":
				dependency.Entries[0].OwnerInstanceID = "other"
			case "resource":
				dependency.Entries[0].ResourceID = "other"
			case "revision":
				dependency.Entries[0].Revision = "other"
			case "snapshot":
				dependency.Entries[0].SnapshotID = "other"
			case "fence":
				dependency.Entries[0].Fence++
			case "dependency":
				dependency.Entries[0].DependencyHash = "other"
			case "set_hash":
				dependency.SetHash = "other"
			}
			// 保留旧 set hash，证明 receipt/fact hash 不只消费调用方的 SetHash。
			changedFact := fact
			changedFact.OwnerRevisionSet = dependency
			if StableRuntimeOperationReceiptHash(changed) == originalReceiptHash || StableRuntimeEvidenceFactHash(changedFact) == originalFactHash {
				t.Fatal("receipt／fact identity 未绑定完整原依赖正文")
			}
		})
	}
	for _, scenario := range []string{"raw", "duplicate", "conflict", "hash"} {
		invalid := receipt.Clone()
		switch scenario {
		case "raw":
			invalid.Evidence.OwnerRevisionSet.Entries[0].ResourceID = " " + set.Entries[0].ResourceID
		case "duplicate", "conflict":
			invalid.Evidence.OwnerRevisionSet.Entries = append(invalid.Evidence.OwnerRevisionSet.Entries, set.Entries[0])
			if scenario == "conflict" {
				invalid.Evidence.OwnerRevisionSet.Entries[1].Revision = "conflict"
			}
			*invalid.Evidence.OwnerRevisionSet = invalid.Evidence.OwnerRevisionSet.WithHash()
		case "hash":
			invalid.Evidence.OwnerRevisionSet.SetHash = "forged"
		}
		if err := invalid.Validate(); err == nil {
			t.Fatalf("非法 receipt dependency 未拒绝: %s", scenario)
		}
		if !reflect.DeepEqual(invalid.Evidence.OwnerRevisionSet, invalid.Normalized().Evidence.OwnerRevisionSet) {
			t.Fatal("receipt normalization 清洗原依赖，可能绕过 raw 校验")
		}
	}
	original, _ := json.Marshal(receipt)
	var reloaded RuntimeOperationReceipt
	if err := json.Unmarshal(original, &reloaded); err != nil || reloaded.Validate() != nil || !reflect.DeepEqual(reloaded, receipt) {
		t.Fatalf("原依赖 reload 漂移: %v", err)
	}
	for _, copy := range []RuntimeOperationReceipt{receipt.Clone(), receipt.Normalized(), cloneRuntimeOperationReceiptRaw(receipt), reloaded} {
		copy.Evidence.OwnerRevisionSet.Entries[0].DependencyHash = "caller-change"
		if receipt.Evidence.OwnerRevisionSet.Entries[0].DependencyHash != set.Entries[0].DependencyHash {
			t.Fatal("receipt 返回副本共享实际依赖")
		}
	}
	for _, copy := range []EvidenceFact{cloneEvidenceFactRaw(fact), normalizeEvidenceFacts([]EvidenceFact{fact})[0]} {
		copy.OwnerRevisionSet.Entries[0].DependencyHash = "caller-change"
		if fact.OwnerRevisionSet.Entries[0].DependencyHash != set.Entries[0].DependencyHash {
			t.Fatal("fact 副本共享实际依赖")
		}
	}
	raw := &RuntimeOwnerRevisionSet{DefinitionRevision: " raw ", Entries: []RuntimeOwnerRevisionRef{}, SetHash: " unverified "}
	if cloned := CloneRuntimeOwnerRevisionSet(raw); !reflect.DeepEqual(cloned, raw) || cloned.Entries == nil {
		t.Fatal("raw clone 改写 nil／empty、身份或 hash")
	}
}

func TestRuntimeReceiptOwnerDependenciesRequireExactDispatchDefinition(t *testing.T) {
	set, _ := ownerDependencyUnionFixture()
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.OwnerRevisionSet = &set
	execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionOwnerCommitted)
	dispatch := RuntimeDispatchRecord{ID: "dispatch", Transaction: AuthorityTransaction{DefinitionRevision: set.DefinitionRevision}, OperationExecutions: []RuntimeOperationExecutionRecord{execution}}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(&dispatch, nil, []RuntimeOperationReceipt{receipt}); err != nil {
		t.Fatal(err)
	}
	dispatch.Transaction.DefinitionRevision = "other"
	if err := ValidateRuntimeOperationReceiptExecutionLineage(&dispatch, nil, []RuntimeOperationReceipt{receipt}); err == nil {
		t.Fatal("自洽 set／receipt hash 替代了 exact dispatch Definition")
	}
}
