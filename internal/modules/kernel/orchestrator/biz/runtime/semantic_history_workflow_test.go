package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func refreshImplementationHistoryTestOwners(t *testing.T, owner *implementationWorkRealFileFixture, a RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	for len(a.Facts.Prerequisites.Pending) > 0 {
		pending := a.Facts.Prerequisites.Pending[0]
		var observed PrerequisiteResult
		switch pending.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			response := sendObservedManifest(t, context.Background(), owner.hub, a, pending, owner.directory)
			observed = PrerequisiteResult{PrerequisiteID: pending.ID, Manifest: &response.Manifest, Observation: &response.Observation}
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			response := sendObservedShellFileAdmissibility(t, context.Background(), owner.hub, a, pending, owner.directory)
			observed = PrerequisiteResult{PrerequisiteID: pending.ID, OperationAdmissibility: &response.Acquisition.Admissibility, Observation: &response.Acquisition.Observation}
		default:
			t.Fatalf("unsupported test owner prerequisite: %s", pending.Kind)
		}
		result, err := owner.controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventPrerequisiteObserved,
			Scope: RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{PrerequisiteResult: &observed}})
		if err != nil {
			t.Fatal(err)
		}
		a = result.Aggregate
	}
	owner.aggregate = &a
	return a
}

// Real File owner effects and private Runtime transitions, not a public
// provider/fullflow acceptance test. Eight original rounds remain the limit.
func TestImplementationWorkEightRoundsHistoryRolloverAndTerminalPurpose(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Teardown(ctx) })
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "target.txt"), []byte("initial implementation"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := &implementationWorkRealFileFixture{hub: hub, directory: directory, maxRounds: 8}
	var a RuntimeAggregate
	for round := 1; round <= 8; round++ {
		a = implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, owner)
		if len(a.OperationReceipts) != round || len(a.ImplementationWork.Rounds) != round || a.SemanticPlanning.HistoryCount() != round*2 || a.ActiveDispatch != nil || a.Facts.Failure != nil {
			t.Fatalf("round %d did not atomically settle receipt, budget, work and retirement", round)
		}
		if a.ImplementationWork.Rounds[round-1].CompletedReceiptSetHash == "" {
			t.Fatal("last receipt lost collective completion")
		}
		for _, fact := range a.Facts.Evidence.Accepted {
			if fact.Kind == runtimecontract.EvidenceKindValidation || len(fact.CoverageWorkItemIDs) != 0 {
				t.Fatal("work minted ordinary validation/CWI credit")
			}
		}
		a = refreshImplementationHistoryTestOwners(t, owner, a)
		// Reconstruct the controller/repository from wire data between every round.
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var reopened RuntimeAggregate
		if err := json.Unmarshal(data, &reopened); err != nil {
			t.Fatal(err)
		}
		if err := reopened.Validate(); err != nil {
			t.Fatal(err)
		}
		owner.repository.aggregate = &reopened
		owner.controller, err = NewRuntimeController(owner.repository)
		if err != nil {
			t.Fatal(err)
		}
		owner.aggregate = &reopened
	}
	before := a.CloneRaw()
	terminal := implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeTerminal, []string{"target.txt"}, owner)
	if terminal.SemanticPlanning.HistoryCount() != 17 || terminal.SemanticPlanning.HistoryArchive == nil || len(terminal.SemanticPlanning.HistoryArchive.Segments) != 1 || len(terminal.SemanticPlanning.History) != 1 {
		t.Fatal("17th retirement failed to roll over complete history")
	}
	if !reflect.DeepEqual(before.ExecutionBudget, terminal.ExecutionBudget) || !reflect.DeepEqual(before.ImplementationWork.Rounds, terminal.ImplementationWork.Rounds) || !reflect.DeepEqual(before.Definition, terminal.Definition) || !reflect.DeepEqual(before.Facts.Evidence, terminal.Facts.Evidence) {
		t.Fatal("rollover changed prior consumption, original contract or proof")
	}
	if request, err := NextRuntimeImplementationWorkScopeRequest(terminal); err != nil || request != nil {
		t.Fatal("terminal purpose reopened a ninth work round")
	}
	if err := terminal.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt := terminal.OperationReceipts[len(terminal.OperationReceipts)-1].Clone()
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{DispatchID: terminal.ImplementationWork.Rounds[7].DispatchID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
	// Fresh OS processes read the full segmented record and exact duplicate;
	// neither a second CAS nor effect/recovery/provider budget is granted.
	implementationWorkUnknownLateReceiptAcrossProcesses(t, terminal, event, true)

}
