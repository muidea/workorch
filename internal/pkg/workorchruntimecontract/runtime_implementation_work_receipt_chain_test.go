package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
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

func TestImplementationWorkReceiptChainBindingSurvivesReload(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	round := ledger.Rounds[0].Clone()
	raw, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeImplementationWorkRound
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(round, loaded) || loaded.Validate() != nil {
		t.Fatal("restart changed original working binding")
	}
	loaded.WorkingBinding = nil
	if _, err := RuntimeImplementationWorkDispositionForDispatchReceipt(loaded, RuntimeDispatchRecord{}, nil, RuntimeFacts{}, nil, RuntimeOperationReceipt{}); err == nil {
		t.Fatal("receipt accepted without working binding")
	}
}
