package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestImplementationWorkResourceChainRequiresExactOrderedEdges(t *testing.T) {
	dep := RuntimeFreshnessDependency{OwnerModuleID: "blocks/resource", Domain: RuntimeResourceWorkspaceTree, SubjectRef: "deliverable", Revision: "R0"}
	receipt := func(before, after string) RuntimeOperationReceipt {
		return RuntimeOperationReceipt{ResourceChanges: []RuntimeResourceChange{{OwnerModuleID: dep.OwnerModuleID, Domain: dep.Domain, SubjectRef: dep.SubjectRef, BeforeRevision: before, AfterRevision: after}}}
	}
	for _, tc := range []struct {
		name  string
		edges []RuntimeOperationReceipt
		want  bool
	}{
		{"four consecutive", []RuntimeOperationReceipt{receipt("R0", "R1"), receipt("R1", "R2"), receipt("R2", "R3"), receipt("R3", "R4")}, true},
		{"duplicate", []RuntimeOperationReceipt{receipt("R0", "R1"), receipt("R0", "R1")}, false},
		{"fork from sealed root", []RuntimeOperationReceipt{receipt("R0", "R1"), receipt("R0", "R2")}, false},
		{"external gap", []RuntimeOperationReceipt{receipt("R0", "R1"), receipt("external", "R2")}, false},
		{"unordered", []RuntimeOperationReceipt{receipt("R1", "R2"), receipt("R0", "R1")}, false},
		{"cycle", []RuntimeOperationReceipt{receipt("R0", "R1"), receipt("R1", "R0")}, false},
		{"no change", []RuntimeOperationReceipt{receipt("R0", "R0")}, false},
		{"empty edge", []RuntimeOperationReceipt{receipt("R0", "")}, false},
		{"missing edge", []RuntimeOperationReceipt{{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(tc.edges)
			chain := newImplementationWorkResourceChain([]RuntimeFreshnessDependency{dep})
			ok := true
			for _, r := range tc.edges {
				if !chain.advance(r) {
					ok = false
					break
				}
			}
			if ok != tc.want {
				t.Fatalf("chain=%t want %t", ok, tc.want)
			}
			after, _ := json.Marshal(tc.edges)
			if !reflect.DeepEqual(input, after) || dep.Revision != "R0" {
				t.Fatal("chain rewrote original owner facts")
			}
		})
	}
	for _, mutate := range []func(*RuntimeResourceChange){func(c *RuntimeResourceChange) { c.OwnerModuleID = "foreign" }, func(c *RuntimeResourceChange) { c.SubjectRef = "foreign" }, func(c *RuntimeResourceChange) { c.Domain = RuntimeResourceWorkspaceTarget }} {
		chain := newImplementationWorkResourceChain([]RuntimeFreshnessDependency{dep})
		r := receipt("R0", "R1")
		mutate(&r.ResourceChanges[0])
		if chain.advance(r) {
			t.Fatal("foreign resource supplied chain edge")
		}
	}
	chain := newImplementationWorkResourceChain([]RuntimeFreshnessDependency{dep})
	r := receipt("R0", "R1")
	r.ResourceChanges = append(r.ResourceChanges, r.ResourceChanges[0])
	if chain.advance(r) {
		t.Fatal("duplicate revision edge accepted")
	}
}

func TestImplementationWorkReceiptProtocolIsSealedAndLegacyHashUnchanged(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	round := ledger.Rounds[0].Clone()
	before, _ := json.Marshal(round.WorkingBinding)
	if strings.Contains(string(before), "ReceiptProtocolRef") {
		t.Fatal("empty extension changed legacy bytes")
	}
	original := round.Hash
	round.WorkingBinding.ReceiptProtocolRef = RuntimeImplementationWorkReceiptChainV1
	rehashImplementationWorkRound(&round)
	if original == round.Hash || round.WorkingBinding.Hash == ledger.Rounds[0].WorkingBinding.Hash {
		t.Fatal("new protocol was not part of sealed identity")
	}
	if err := round.Validate(); err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeImplementationWorkRound
	raw, _ := json.Marshal(round)
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(round, reloaded) || reloaded.Validate() != nil {
		t.Fatal("restart lost sealed protocol")
	}
	round.WorkingBinding.ReceiptProtocolRef = "unknown"
	rehashImplementationWorkRound(&round)
	if round.Validate() == nil {
		t.Fatal("unknown protocol accepted")
	}
}
