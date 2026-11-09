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
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This workflow audits an independently decoded original once between rounds,
// then exercises the current formal-read/CAS contract. The detached audit
// repository remains available to tests of unaccepted/corrupt snapshots; it is
// not the production recovery contract and must not run every historical proof
// twice on every workflow query.
type implementationWorkCommittedTestRepository struct {
	store *implementationWorkDetachedRepository
}

func (r implementationWorkCommittedTestRepository) Load(_ context.Context, w common.WorkspaceID, id common.RunID) (RuntimeAggregate, error) {
	if r.store.aggregate == nil || r.store.aggregate.Facts.WorkspaceID != w || r.store.aggregate.Facts.RunID != id {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return ReadCommittedRuntimeOwnerStateRecord(RuntimeStateRecordFromAggregate(*r.store.aggregate))
}
func (r implementationWorkCommittedTestRepository) Save(ctx context.Context, a RuntimeAggregate, revision uint64) error {
	return r.store.Save(ctx, a, revision)
}

func restoreImplementationHistoryTestController(t *testing.T, owner *implementationWorkRealFileFixture) {
	t.Helper()
	reopened, err := NewRuntimeController(implementationWorkCommittedTestRepository{owner.repository})
	if err != nil {
		t.Fatal(err)
	}
	// The receipt handlers retain this controller's address. Replacing only
	// owner.controller leaves those consumers on the old cold-audit repository,
	// unlike production's formal owner query. Reconstruct all facets together;
	// the independently decoded full audit below still runs every round.
	*owner.controller = *reopened
}

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
// provider/fullflow acceptance test. Continuous effective work has no cumulative round limit.
func TestImplementationWorkThirtyThreeRoundsHistoryRolloverAndTerminalPurpose(t *testing.T) {
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
	owner := &implementationWorkRealFileFixture{hub: hub, directory: directory, maxRepeatedStates: 2}
	var a RuntimeAggregate
	for round := 1; round <= 33; round++ {
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
		restoreImplementationHistoryTestController(t, owner)
		owner.aggregate = &reopened
	}
	before := a.CloneRaw()
	for _, purpose := range []runtimecontract.RuntimeImplementationWorkPurpose{runtimecontract.RuntimeImplementationWorkPurposeInconclusive} {
		restored := before.CloneRaw()
		owner.aggregate, owner.repository.aggregate = &restored, &restored
		restoreImplementationHistoryTestController(t, owner)
		failed := implementationWorkPrivatePipeline(t, purpose, []string{"target.txt"}, owner)
		expectedCode := "implementation_work_repeated_state_exhausted"
		if purpose == runtimecontract.RuntimeImplementationWorkPurposeInconclusive {
			expectedCode = "implementation_work_review_inconclusive"
		}
		if failed.Facts.Failure == nil || failed.Facts.Failure.Code != expectedCode || failed.DefinitionClarification != nil || failed.Decision.State != runtimecontract.ConvergenceStateFailed {
			t.Fatalf("factual observation did not reduce to honest failure: %+v", failed.Facts.Failure)
		}
		if !reflect.DeepEqual(before.ExecutionBudget, failed.ExecutionBudget) || !reflect.DeepEqual(before.ImplementationWork.Rounds, failed.ImplementationWork.Rounds) || !reflect.DeepEqual(before.Facts.Evidence, failed.Facts.Evidence) {
			t.Fatal("factual observation created work, spent effect budget or granted credit")
		}
		if request, err := NextRuntimeImplementationWorkScopeRequest(failed); err != nil || request != nil {
			t.Fatal("failed observation reopened work", err)
		}
		if err := failed.Validate(); err != nil {
			t.Fatal(err)
		}
		tampered := failed.CloneRaw()
		tampered.Facts.Failure.Code = "other"
		if err := tampered.Validate(); err == nil {
			t.Fatal("rehydration lost factual failure correspondence")
		}
	}
	restored := before.CloneRaw()
	owner.aggregate, owner.repository.aggregate = &restored, &restored
	restoreImplementationHistoryTestController(t, owner)
	terminal := implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeTerminal, []string{"target.txt"}, owner)
	if terminal.SemanticPlanning.HistoryCount() != 67 || terminal.SemanticPlanning.HistoryArchive == nil || len(terminal.SemanticPlanning.HistoryArchive.Segments) != 4 || len(terminal.SemanticPlanning.History) != 3 {
		t.Fatal("67th retirement failed to roll over complete history")
	}
	if !reflect.DeepEqual(before.ExecutionBudget, terminal.ExecutionBudget) || !reflect.DeepEqual(before.ImplementationWork.Rounds, terminal.ImplementationWork.Rounds) || !reflect.DeepEqual(before.Definition, terminal.Definition) || !reflect.DeepEqual(before.Facts.Evidence, terminal.Facts.Evidence) {
		t.Fatal("rollover changed prior consumption, original contract or proof")
	}
	if request, err := NextRuntimeImplementationWorkScopeRequest(terminal); err != nil || request != nil {
		t.Fatal("terminal purpose reopened a additional work round")
	}
	if err := terminal.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt := terminal.OperationReceipts[len(terminal.OperationReceipts)-1].Clone()
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: terminal.Facts.WorkspaceID, RunID: terminal.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{DispatchID: terminal.ImplementationWork.Rounds[32].DispatchID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
	// Fresh OS processes read the full segmented record and exact duplicate;
	// neither a second CAS nor effect/recovery/provider budget is granted.
	implementationWorkUnknownLateReceiptAcrossProcesses(t, terminal, event, true)

}
