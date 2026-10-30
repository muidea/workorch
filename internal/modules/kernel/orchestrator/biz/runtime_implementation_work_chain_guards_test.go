package orchestrator

import (
	"encoding/json"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"slices"
	"testing"
)

// Use complete production-driver output, not round labels or invented receipts.
func assertImplementationWorkChainGuards(t *testing.T, source contract.RuntimeStateRecord) {
	t.Helper()
	before, _ := json.Marshal(source)
	for _, tc := range []struct {
		name   string
		mutate func(*contract.RuntimeStateRecord, int, int)
		want   contract.RuntimeImplementationWorkReceiptDisposition
		fail   bool
	}{
		{name: "exact persisted prefix"},
		{name: "missing predecessor", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.OperationReceipts = slices.Delete(s.OperationReceipts, i, i+1)
		}, fail: true},
		{name: "duplicate predecessor", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.OperationReceipts = append(s.OperationReceipts, s.OperationReceipts[i])
		}, fail: true},
		{name: "uncommitted predecessor", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.CompletedDispatches[0].OperationExecutions[0].State = contract.OperationExecutionOwnerCommitted
		}, fail: true},
		{name: "predecessor evidence omitted", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.Facts.Evidence.Accepted = nil }, fail: true},
		{name: "predecessor unknown", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.OperationReceipts[i].Outcome = contract.RuntimeOperationOutcomeUnknown
			refreshChainReceiptHash(s, i)
		}, fail: true},
		{name: "predecessor foreign key", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.OperationReceipts[i].ExecutionKey = "exec/foreign" }, fail: true},
		{name: "predecessor foreign fence", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.OperationReceipts[i].Fence++ }, fail: true},
		{name: "cross workspace", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.CompletedDispatches[0].WorkspaceID = "foreign" }, fail: true},
		{name: "cross run", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.CompletedDispatches[0].RunID = "foreign" }, fail: true},
		{name: "cross dispatch", mutate: func(s *contract.RuntimeStateRecord, i, j int) { s.CompletedDispatches[0].ID = "foreign" }, fail: true},
		{name: "unknown protocol", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.ImplementationWork.Rounds[0].WorkingBinding.ReceiptProtocolRef = "unsupported"
		}, fail: true},
		{name: "missing current change", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.OperationReceipts[j].ResourceChanges = nil
			refreshChainReceiptHash(s, j)
		}, want: contract.RuntimeImplementationWorkReceiptInvalidationMissing},
		{name: "external revision gap", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			mutateChainEdge(s, j, func(c *contract.RuntimeResourceChange) { c.BeforeRevision = "external" })
		}, want: contract.RuntimeImplementationWorkReceiptInvalidationMissing},
		{name: "reuse original root", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			root := s.ImplementationWork.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0].Revision
			mutateChainEdge(s, j, func(c *contract.RuntimeResourceChange) { c.BeforeRevision = root })
		}, want: contract.RuntimeImplementationWorkReceiptInvalidationMissing},
		{name: "revision cycle", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			root := s.ImplementationWork.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0].Revision
			mutateChainEdge(s, j, func(c *contract.RuntimeResourceChange) { c.AfterRevision = root })
		}, want: contract.RuntimeImplementationWorkReceiptInvalidationMissing},
		{name: "foreign resource owner", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			mutateChainEdge(s, j, func(c *contract.RuntimeResourceChange) { c.OwnerModuleID = "blocks/foreign" })
		}, want: contract.RuntimeImplementationWorkReceiptInvalidationMissing},
		{name: "current outcome unknown", mutate: func(s *contract.RuntimeStateRecord, i, j int) {
			s.OperationReceipts[j].Outcome = contract.RuntimeOperationOutcomeUnknown
			refreshChainReceiptHash(s, j)
		}, want: contract.RuntimeImplementationWorkReceiptUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := source.Clone()
			d := &s.CompletedDispatches[0]
			first, second := -1, -1
			for i, r := range s.OperationReceipts {
				if r.ExecutionKey == d.OperationExecutions[0].ExecutionKey {
					first = i
				}
				if r.ExecutionKey == d.OperationExecutions[1].ExecutionKey {
					second = i
				}
			}
			if first < 0 || second < 0 {
				t.Fatal("original receipts absent")
			}
			current := s.OperationReceipts[second].Clone()
			if tc.mutate != nil {
				tc.mutate(&s, first, second)
			}
			// Deleting a predecessor must not accidentally select a different current.
			for _, r := range s.OperationReceipts {
				if r.ExecutionKey == current.ExecutionKey && r.Fence == current.Fence {
					current = r
				}
			}
			got, err := contract.RuntimeImplementationWorkDispositionForDispatchReceipt(s.ImplementationWork.Rounds[0], s.CompletedDispatches[0], s.OperationReceipts, s.Facts, s.OperationGovernanceBindings, current)
			if tc.fail {
				if err == nil {
					t.Fatalf("invalid source accepted: %s", got)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q / %v want %q", got, err, tc.want)
			}
		})
	}
	round := source.ImplementationWork.Rounds[0].Clone()
	round.CompletedReceiptSetHash = ""
	round.Hash = contract.StableRuntimeImplementationWorkRoundHash(round)
	members, err := contract.RuntimeImplementationWorkReceiptMembers(source.CompletedDispatches[0], source.OperationReceipts, source.Facts, source.OperationGovernanceBindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = contract.ValidateRuntimeImplementationWorkReceiptSet(round, members); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"reverse", "missing edge", "branch"} {
		t.Run("collective/"+mode, func(t *testing.T) {
			s := source.Clone()
			d := s.CompletedDispatches[0]
			m, err := contract.RuntimeImplementationWorkReceiptMembers(d, s.OperationReceipts, s.Facts, s.OperationGovernanceBindings)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "reverse":
				slices.Reverse(m)
			case "missing edge":
				m[1].Receipt.ResourceChanges = nil
			case "branch":
				for i := range m[1].Receipt.ResourceChanges {
					c := &m[1].Receipt.ResourceChanges[i]
					if c.Domain == contract.RuntimeResourceWorkspaceTree {
						c.BeforeRevision = round.WorkingBinding.CatalogObservation.Dependencies[0].Revision
					}
				}
			}
			for i := range m {
				m[i].Execution.OwnerReceiptHash = contract.StableRuntimeOperationReceiptHash(m[i].Receipt)
			}
			if _, err = contract.ValidateRuntimeImplementationWorkReceiptSet(round, m); err == nil {
				t.Fatal("collective chain bypassed")
			}
		})
	}
	after, _ := json.Marshal(source)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("chain gate mutated original raw source")
	}
}

func refreshChainReceiptHash(s *contract.RuntimeStateRecord, index int) {
	r := s.OperationReceipts[index]
	for i := range s.CompletedDispatches[0].OperationExecutions {
		e := &s.CompletedDispatches[0].OperationExecutions[i]
		if e.ExecutionKey == r.ExecutionKey {
			e.OwnerReceiptHash = contract.StableRuntimeOperationReceiptHash(r)
		}
	}
}

func mutateChainEdge(s *contract.RuntimeStateRecord, index int, change func(*contract.RuntimeResourceChange)) {
	for i := range s.OperationReceipts[index].ResourceChanges {
		c := &s.OperationReceipts[index].ResourceChanges[i]
		if c.Domain == contract.RuntimeResourceWorkspaceTree {
			change(c)
		}
	}
	refreshChainReceiptHash(s, index)
}
