package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerReconciliationGenerationsPreserveHistoryAndRejectStaleRequests(t *testing.T) {
	ctx := context.Background()
	repository, controller, request := reconciliationControllerFixture(t, runtimecontract.RuntimeCWIReconciliationScopeCurrentRun)

	sealed, changed, err := controller.ensureReconciliationRequest(ctx, request)
	if err != nil || !changed {
		t.Fatalf("seal generation 1 = aggregate=%+v changed=%v err=%v", sealed, changed, err)
	}
	if got := repository.saves; got != 1 {
		t.Fatalf("generation 1 save count = %d, want 1", got)
	}

	batch := reconciliationBatchForRequest(t, request, runtimecontract.RuntimeCWIReconciliationObservationSatisfied)
	committed, changed, err := controller.commitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
	if err != nil || !changed {
		t.Fatalf("commit generation 1 = aggregate=%+v changed=%v err=%v", committed, changed, err)
	}
	if committed.Reconciliation.Batch == nil || committed.Reconciliation.Batch.Hash != batch.Hash {
		t.Fatalf("generation 1 batch was not persisted: %+v", committed.Reconciliation)
	}

	duplicate, changed, err := controller.ensureReconciliationRequest(ctx, request)
	if err != nil || changed || duplicate.StateRevision != committed.StateRevision || repository.saves != 2 {
		t.Fatalf("duplicate generation 1 rewrote state: aggregate=%+v changed=%v saves=%d err=%v", duplicate, changed, repository.saves, err)
	}

	next := request
	next.RequestID = "reconcile-generation-2"
	next.Generation = 2
	next.WorkspaceBaselineRevision = "tree-2"
	next.WorkspaceBaselineRef = "workspace-tree/tree-2"
	advanced, changed, err := controller.ensureReconciliationRequest(ctx, next)
	if err != nil || !changed {
		t.Fatalf("seal generation 2 = aggregate=%+v changed=%v err=%v", advanced, changed, err)
	}
	if len(advanced.Reconciliation.History) != 1 || advanced.Reconciliation.History[0].Request.Generation != 1 || advanced.Reconciliation.History[0].Batch == nil {
		t.Fatalf("generation 1 history was not retained: %+v", advanced.Reconciliation.History)
	}
	if advanced.Reconciliation.Request == nil || advanced.Reconciliation.Request.Generation != 2 || advanced.Reconciliation.Batch != nil {
		t.Fatalf("current generation was not reset: %+v", advanced.Reconciliation)
	}

	historicalDuplicate, changed, err := controller.ensureReconciliationRequest(ctx, request)
	if err != nil || changed || historicalDuplicate.StateRevision != advanced.StateRevision || repository.saves != 3 {
		t.Fatalf("historical duplicate rewrote current generation: aggregate=%+v changed=%v saves=%d err=%v", historicalDuplicate, changed, repository.saves, err)
	}

	stale := next
	stale.RequestID = "reconcile-stale"
	stale.Generation = 1
	if _, changed, err := controller.ensureReconciliationRequest(ctx, stale); !errors.Is(err, common.ErrConflict) || changed {
		t.Fatalf("stale generation was accepted: changed=%v err=%v", changed, err)
	}
}

func TestRuntimeControllerReconciliationHistoryLimitRejectsNewGenerationWithoutCAS(t *testing.T) {
	ctx := context.Background()
	repository, controller, template := reconciliationControllerFixture(t, runtimecontract.RuntimeCWIReconciliationScopeCurrentRun)

	for generation := uint64(1); generation <= runtimecontract.RuntimeCWIReconciliationHistoryLimit+1; generation++ {
		request := template.Clone()
		request.RequestID = fmt.Sprintf("reconcile-history-%d", generation)
		request.Generation = generation
		if _, changed, err := controller.ensureReconciliationRequest(ctx, request); err != nil || !changed {
			t.Fatalf("seal generation %d = changed=%v err=%v", generation, changed, err)
		}
	}

	beforeRevision := repository.aggregate.StateRevision
	beforeSaves := repository.saves
	beforeHistory := len(repository.aggregate.Reconciliation.History)
	beforeRequest := repository.aggregate.Reconciliation.Request.Clone()
	overflow := template.Clone()
	overflow.RequestID = "reconcile-history-overflow"
	overflow.Generation = runtimecontract.RuntimeCWIReconciliationHistoryLimit + 2
	if _, changed, err := controller.ensureReconciliationRequest(ctx, overflow); !errors.Is(err, common.ErrConflict) || changed {
		t.Fatalf("history overflow was accepted: changed=%v err=%v", changed, err)
	}
	if repository.saves != beforeSaves || repository.aggregate.StateRevision != beforeRevision ||
		len(repository.aggregate.Reconciliation.History) != beforeHistory ||
		repository.aggregate.Reconciliation.Request == nil ||
		repository.aggregate.Reconciliation.Request.RequestID != beforeRequest.RequestID {
		t.Fatalf("history overflow performed a partial CAS: saves=%d/%d aggregate=%+v", repository.saves, beforeSaves, repository.aggregate.Reconciliation)
	}
}

func TestRuntimeControllerStaleReconciliationBatchIsZeroMutation(t *testing.T) {
	ctx := context.Background()
	repository, controller, firstRequest := reconciliationControllerFixture(t, runtimecontract.RuntimeCWIReconciliationScopeCurrentRun)
	repository.aggregate.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
	repository.aggregate.Facts.Coverage[0].EvidenceRefs = []string{"current-run-receipt"}
	runtimeTestSynchronizeCanonicalCWILedger(t, repository.aggregate)
	if err := repository.aggregate.Validate(); err != nil {
		t.Fatalf("fixture aggregate: %v", err)
	}

	if _, changed, err := controller.ensureReconciliationRequest(ctx, firstRequest); err != nil || !changed {
		t.Fatalf("seal generation 1 = changed=%v err=%v", changed, err)
	}
	firstBatch := reconciliationBatchForRequest(t, firstRequest, runtimecontract.RuntimeCWIReconciliationObservationSatisfied)
	if _, changed, err := controller.commitReconciliationBatch(ctx, firstRequest.WorkspaceID, firstRequest.RunID, firstBatch); err != nil || !changed {
		t.Fatalf("commit generation 1 = changed=%v err=%v", changed, err)
	}

	secondRequest := firstRequest.Clone()
	secondRequest.RequestID = "reconcile-generation-2"
	secondRequest.Generation = 2
	secondRequest.WorkspaceBaselineRevision = "tree-2"
	secondRequest.WorkspaceBaselineRef = "workspace-tree/tree-2"
	if _, changed, err := controller.ensureReconciliationRequest(ctx, secondRequest); err != nil || !changed {
		t.Fatalf("seal generation 2 = changed=%v err=%v", changed, err)
	}

	before := repository.aggregate
	beforeFacts := before.Facts.Normalized()
	beforeRequest := before.Reconciliation.Request.Clone()
	beforeContract := before.Facts.ReconciliationContract.Clone()
	beforeRevision := before.StateRevision
	beforeSaves := repository.saves
	beforeSessions := len(before.CandidateRequestSessions)
	beforeReceipts := len(before.OperationReceipts)

	delayed, changed, err := controller.commitReconciliationBatch(ctx, firstRequest.WorkspaceID, firstRequest.RunID, firstBatch)
	if err != nil || changed {
		t.Fatalf("delayed historical batch = changed=%v err=%v", changed, err)
	}
	if delayed.StateRevision != beforeRevision || repository.saves != beforeSaves ||
		!reflect.DeepEqual(delayed.Facts.Normalized(), beforeFacts) ||
		delayed.Reconciliation.Request == nil || !reflect.DeepEqual(delayed.Reconciliation.Request.Clone(), beforeRequest) ||
		!reflect.DeepEqual(delayed.Facts.ReconciliationContract.Clone(), beforeContract) ||
		len(delayed.CandidateRequestSessions) != beforeSessions || len(delayed.OperationReceipts) != beforeReceipts || delayed.ActiveDispatch != nil {
		t.Fatalf("stale reconciliation batch mutated Runtime authority or execution ledger: aggregate=%+v saves=%d/%d", delayed, repository.saves, beforeSaves)
	}
}

func TestRuntimeControllerReconciliationObservationsControlRetainAndReopenWithoutGrantingCredit(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name            string
		scope           runtimecontract.RuntimeCWIReconciliationScope
		observation     runtimecontract.RuntimeCWIReconciliationObservationStatus
		wantDisposition runtimecontract.RuntimeCWIReconciliationDisposition
		initialState    runtimecontract.RuntimeCoverageItemState
		wantState       runtimecontract.RuntimeCoverageItemState
	}{
		{name: "current run retains matching existing credit", scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, observation: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, initialState: runtimecontract.RuntimeCoverageItemStateSatisfied, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionRetain, wantState: runtimecontract.RuntimeCoverageItemStateSatisfied},
		{name: "workspace full retains matching current-run credit", scope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull, observation: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, initialState: runtimecontract.RuntimeCoverageItemStateSatisfied, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionRetain, wantState: runtimecontract.RuntimeCoverageItemStateSatisfied},
		{name: "workspace full stale observation reopens exact current-run credit", scope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull, observation: runtimecontract.RuntimeCWIReconciliationObservationStale, initialState: runtimecontract.RuntimeCoverageItemStateSatisfied, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionReopen, wantState: runtimecontract.RuntimeCoverageItemStateOpen},
		{name: "unverifiable observation keeps open coverage at prerequisite", scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, observation: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, initialState: runtimecontract.RuntimeCoverageItemStateOpen, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionPrerequisite, wantState: runtimecontract.RuntimeCoverageItemStateOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository, controller, request := reconciliationControllerFixture(t, tc.scope)
			repository.aggregate.Facts.Coverage[0].State = tc.initialState
			if tc.initialState == runtimecontract.RuntimeCoverageItemStateSatisfied {
				repository.aggregate.Facts.Coverage[0].EvidenceRefs = []string{"old-run-receipt"}
			}
			runtimeTestSynchronizeCanonicalCWILedger(t, repository.aggregate)
			if err := repository.aggregate.Validate(); err != nil {
				t.Fatalf("fixture aggregate: %v", err)
			}
			sealed, changed, err := controller.ensureReconciliationRequest(ctx, request)
			if err != nil || !changed {
				t.Fatalf("seal request = aggregate=%+v changed=%v err=%v", sealed, changed, err)
			}
			batch := reconciliationBatchForRequest(t, request, tc.observation)
			committed, changed, err := controller.commitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
			if err != nil || !changed {
				t.Fatalf("commit batch = aggregate=%+v changed=%v err=%v", committed, changed, err)
			}
			results := committed.Reconciliation.Results
			reopened := committed.Reconciliation.Reopened
			if tc.wantDisposition == runtimecontract.RuntimeCWIReconciliationDispositionReopen {
				if len(committed.Reconciliation.History) == 0 {
					t.Fatalf("reopen did not archive its superseded reconciliation generation: %+v", committed.Reconciliation)
				}
				history := committed.Reconciliation.History[len(committed.Reconciliation.History)-1]
				results = history.Results
				reopened = history.Reopened
			}
			if len(results) != 1 || results[0].Disposition != tc.wantDisposition {
				t.Fatalf("reconciliation results = %+v, want disposition %q", results, tc.wantDisposition)
			}
			if got := committed.Facts.Coverage[0].State; got != tc.wantState {
				t.Fatalf("coverage state = %q, want %q", got, tc.wantState)
			}
			if tc.wantDisposition == runtimecontract.RuntimeCWIReconciliationDispositionReopen {
				if len(reopened) != 1 || len(reopened[0].PreviousEvidenceRefs) != 1 || reopened[0].PreviousEvidenceRefs[0] != "old-run-receipt" {
					t.Fatalf("reopened lineage lost old receipt: %+v", reopened)
				}
			}
			duplicate, changed, err := controller.commitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
			if err != nil || changed || duplicate.StateRevision != committed.StateRevision || repository.saves != 2 {
				t.Fatalf("duplicate batch rewrote state: aggregate=%+v changed=%v saves=%d err=%v", duplicate, changed, repository.saves, err)
			}
		})
	}
}

func reconciliationControllerFixture(t *testing.T, scope runtimecontract.RuntimeCWIReconciliationScope) (*controllerRepository, *RuntimeController, runtimecontract.RuntimeCWIReconciliationRequest) {
	t.Helper()
	aggregate := stateChangeAggregate(t)
	contract, err := runtimecontract.CompileRuntimeCWIReconciliationContract(aggregate.Facts.DefinitionRevision, []runtimecontract.CoverageFocus{aggregate.Facts.Focus})
	if err != nil {
		t.Fatalf("compile reconciliation contract: %v", err)
	}
	aggregate.Facts.ReconciliationContract = contract
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("reconciliation fixture aggregate: %v", err)
	}
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatalf("NewRuntimeController: %v", err)
	}
	request := runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID:                 "reconcile-generation-1",
		WorkspaceID:               aggregate.Facts.WorkspaceID,
		RunID:                     aggregate.Facts.RunID,
		Scope:                     scope,
		DefinitionRevision:        contract.DefinitionRevision,
		BindingSetHash:            contract.BindingSetHash,
		Bindings:                  contract.Bindings,
		WorkspaceBaselineRevision: "tree-1",
		WorkspaceBaselineRef:      "workspace-tree/tree-1",
		Generation:                1,
	}
	if err := request.ValidateAgainstContract(contract); err != nil {
		t.Fatalf("fixture request: %v", err)
	}
	return repository, controller, request
}

func reconciliationBatchForRequest(t *testing.T, request runtimecontract.RuntimeCWIReconciliationRequest, status runtimecontract.RuntimeCWIReconciliationObservationStatus) runtimecontract.RuntimeCWIReconciliationBatch {
	t.Helper()
	batch := runtimecontract.RuntimeCWIReconciliationBatch{
		RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
		Generation: request.Generation, BindingSetHash: request.BindingSetHash,
	}
	for _, binding := range request.Bindings {
		for _, owner := range binding.OwnerModuleIDs {
			observation := runtimecontract.RuntimeCWIReconciliationObservation{
				RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
				Generation: request.Generation, CoverageWorkItemID: binding.CoverageWorkItemID,
				BindingID: binding.BindingID, BindingHash: binding.BindingHash, OwnerModuleID: owner,
				ObservedRevision: request.WorkspaceBaselineRevision, Status: status,
				TargetRefs: append([]string(nil), binding.TargetRefs...), CheckRefs: append([]string(nil), binding.RequiredCheckRefs...),
			}
			batch.Observations = append(batch.Observations, observation)
		}
	}
	batch.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(batch)
	if err := batch.ValidateForRequest(request); err != nil {
		t.Fatalf("fixture batch: %v", err)
	}
	return batch
}
