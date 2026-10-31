package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeEvidenceDependencyPairPreservesIndependentSnapshotsAndIdentity(t *testing.T) {
	reads, _ := ownerDependencyUnionFixture()
	current := *CloneRuntimeOwnerRevisionSet(&reads)
	current.Entries[0].Revision = "actually-observed-postcondition"
	current = current.WithHash()
	proof := &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: current}
	receipt := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	receipt.Evidence.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(&reads)
	legacyHash := StableRuntimeOperationReceiptHash(receipt)
	receipt.Evidence.ApplicabilityDependencies = proof
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	fact := EvidenceFact{ID: "evidence", OwnerRevisionSet: CloneRuntimeOwnerRevisionSet(&reads), ApplicabilityDependencies: CloneRuntimeEvidenceApplicabilityDependencies(proof)}
	receiptHash, factHash := StableRuntimeOperationReceiptHash(receipt), StableRuntimeEvidenceFactHash(fact)
	if receiptHash == legacyHash {
		t.Fatal("new dependency proof is not sealed in receipt identity")
	}
	for _, copy := range []RuntimeOperationReceipt{receipt.Clone(), receipt.Normalized(), cloneRuntimeOperationReceiptRaw(receipt)} {
		copy.Evidence.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if receipt.Evidence.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != current.Entries[0].Revision {
			t.Fatal("receipt copy aliases current observations")
		}
	}
	for _, copy := range []EvidenceFact{cloneEvidenceFactRaw(fact), normalizeEvidenceFacts([]EvidenceFact{fact})[0]} {
		copy.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if fact.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != current.Entries[0].Revision {
			t.Fatal("fact copy aliases current observations")
		}
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeOperationReceipt
	if err := json.Unmarshal(body, &reloaded); err != nil || !reflect.DeepEqual(reloaded, receipt) || reloaded.Validate() != nil {
		t.Fatalf("pair reload: %v", err)
	}
	if !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &reads) {
		t.Fatal("current snapshot overwrote original reads")
	}
	for _, scenario := range []string{"delete", "protocol", "revision", "selector", "set_hash"} {
		changed := receipt.Clone()
		switch scenario {
		case "delete":
			changed.Evidence.ApplicabilityDependencies = nil
		case "protocol":
			changed.Evidence.ApplicabilityDependencies.ProtocolRef = "other"
		case "revision":
			changed.Evidence.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "other"
		case "selector":
			changed.Evidence.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].DependencyHash = "other"
		case "set_hash":
			changed.Evidence.ApplicabilityDependencies.OwnerRevisionSet.SetHash = "other"
		}
		changedFact := cloneEvidenceFactRaw(fact)
		changedFact.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(changed.Evidence.ApplicabilityDependencies)
		if StableRuntimeOperationReceiptHash(changed) == receiptHash || StableRuntimeEvidenceFactHash(changedFact) == factHash {
			t.Fatalf("unsealed applicability body: %s", scenario)
		}
	}
	for _, scenario := range []string{"no_history", "unknown_protocol", "raw", "definition", "empty", "hash", "conflict"} {
		changed := receipt.Clone()
		set := &changed.Evidence.ApplicabilityDependencies.OwnerRevisionSet
		switch scenario {
		case "no_history":
			changed.Evidence.OwnerRevisionSet = nil
		case "unknown_protocol":
			changed.Evidence.ApplicabilityDependencies.ProtocolRef += " "
		case "raw":
			set.Entries[0].ResourceID = " " + set.Entries[0].ResourceID
		case "definition":
			set.DefinitionRevision = "other"
			*set = set.WithHash()
		case "empty":
			set.Entries = nil
			*set = set.WithHash()
		case "hash":
			set.SetHash = "wrong"
		case "conflict":
			item := set.Entries[0]
			item.Revision = "conflict"
			set.Entries = append(set.Entries, item)
			*set = set.WithHash()
		}
		if err := changed.Validate(); err == nil {
			t.Fatalf("invalid pair accepted: %s", scenario)
		}
		if !reflect.DeepEqual(changed.Evidence.ApplicabilityDependencies, changed.Normalized().Evidence.ApplicabilityDependencies) {
			t.Fatal("normalization cleaned invalid current observation")
		}
	}
	if err := ValidateRuntimeEvidenceDependencyPair(&reads, proof, "other-definition"); err == nil {
		t.Fatal("caller Definition did not fence both sets")
	}
}
