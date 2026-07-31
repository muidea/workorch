package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerReconciliationGenerationsPreserveHistoryAndRejectStaleRequests(t *testing.T) {
	ctx := context.Background()
	repository, controller, request := reconciliationControllerFixture(t, runtimecontract.RuntimeCWIReconciliationScopeCurrentRun)

	sealed, changed, err := controller.EnsureReconciliationRequest(ctx, request)
	if err != nil || !changed {
		t.Fatalf("seal generation 1 = aggregate=%+v changed=%v err=%v", sealed, changed, err)
	}
	if got := repository.saves; got != 1 {
		t.Fatalf("generation 1 save count = %d, want 1", got)
	}

	batch := reconciliationBatchForRequest(t, request, runtimecontract.RuntimeCWIReconciliationObservationSatisfied)
	committed, changed, err := controller.CommitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
	if err != nil || !changed {
		t.Fatalf("commit generation 1 = aggregate=%+v changed=%v err=%v", committed, changed, err)
	}
	if committed.Reconciliation.Batch == nil || committed.Reconciliation.Batch.Hash != batch.Hash {
		t.Fatalf("generation 1 batch was not persisted: %+v", committed.Reconciliation)
	}

	duplicate, changed, err := controller.EnsureReconciliationRequest(ctx, request)
	if err != nil || changed || duplicate.StateRevision != committed.StateRevision || repository.saves != 2 {
		t.Fatalf("duplicate generation 1 rewrote state: aggregate=%+v changed=%v saves=%d err=%v", duplicate, changed, repository.saves, err)
	}

	next := request
	next.RequestID = "reconcile-generation-2"
	next.Generation = 2
	next.WorkspaceBaselineRevision = "tree-2"
	next.WorkspaceBaselineRef = "workspace-tree/tree-2"
	advanced, changed, err := controller.EnsureReconciliationRequest(ctx, next)
	if err != nil || !changed {
		t.Fatalf("seal generation 2 = aggregate=%+v changed=%v err=%v", advanced, changed, err)
	}
	if len(advanced.Reconciliation.History) != 1 || advanced.Reconciliation.History[0].Request.Generation != 1 || advanced.Reconciliation.History[0].Batch == nil {
		t.Fatalf("generation 1 history was not retained: %+v", advanced.Reconciliation.History)
	}
	if advanced.Reconciliation.Request == nil || advanced.Reconciliation.Request.Generation != 2 || advanced.Reconciliation.Batch != nil {
		t.Fatalf("current generation was not reset: %+v", advanced.Reconciliation)
	}

	historicalDuplicate, changed, err := controller.EnsureReconciliationRequest(ctx, request)
	if err != nil || changed || historicalDuplicate.StateRevision != advanced.StateRevision || repository.saves != 3 {
		t.Fatalf("historical duplicate rewrote current generation: aggregate=%+v changed=%v saves=%d err=%v", historicalDuplicate, changed, repository.saves, err)
	}

	stale := next
	stale.RequestID = "reconcile-stale"
	stale.Generation = 1
	if _, changed, err := controller.EnsureReconciliationRequest(ctx, stale); !errors.Is(err, common.ErrConflict) || changed {
		t.Fatalf("stale generation was accepted: changed=%v err=%v", changed, err)
	}
}

func TestRuntimeControllerReconciliationScopeControlsRetainAndReopenWithoutGrantingCredit(t *testing.T) {
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
		{name: "workspace restart reopens predecessor credit", scope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull, observation: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, initialState: runtimecontract.RuntimeCoverageItemStateSatisfied, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionReopen, wantState: runtimecontract.RuntimeCoverageItemStateOpen},
		{name: "unverifiable observation keeps open coverage at prerequisite", scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, observation: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, initialState: runtimecontract.RuntimeCoverageItemStateOpen, wantDisposition: runtimecontract.RuntimeCWIReconciliationDispositionPrerequisite, wantState: runtimecontract.RuntimeCoverageItemStateOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository, controller, request := reconciliationControllerFixture(t, tc.scope)
			repository.aggregate.Facts.Coverage[0].State = tc.initialState
			if tc.initialState == runtimecontract.RuntimeCoverageItemStateSatisfied {
				repository.aggregate.Facts.Coverage[0].EvidenceRefs = []string{"old-run-receipt"}
			}
			if err := repository.aggregate.Validate(); err != nil {
				t.Fatalf("fixture aggregate: %v", err)
			}
			sealed, changed, err := controller.EnsureReconciliationRequest(ctx, request)
			if err != nil || !changed {
				t.Fatalf("seal request = aggregate=%+v changed=%v err=%v", sealed, changed, err)
			}
			batch := reconciliationBatchForRequest(t, request, tc.observation)
			committed, changed, err := controller.CommitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
			if err != nil || !changed {
				t.Fatalf("commit batch = aggregate=%+v changed=%v err=%v", committed, changed, err)
			}
			if got := committed.Reconciliation.Results[0].Disposition; got != tc.wantDisposition {
				t.Fatalf("disposition = %q, want %q", got, tc.wantDisposition)
			}
			if got := committed.Facts.Coverage[0].State; got != tc.wantState {
				t.Fatalf("coverage state = %q, want %q", got, tc.wantState)
			}
			if tc.wantDisposition == runtimecontract.RuntimeCWIReconciliationDispositionReopen {
				if len(committed.Reconciliation.Reopened) != 1 || len(committed.Reconciliation.Reopened[0].PreviousEvidenceRefs) != 1 || committed.Reconciliation.Reopened[0].PreviousEvidenceRefs[0] != "old-run-receipt" {
					t.Fatalf("reopened lineage lost old receipt: %+v", committed.Reconciliation.Reopened)
				}
			}
			duplicate, changed, err := controller.CommitReconciliationBatch(ctx, request.WorkspaceID, request.RunID, batch)
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
			batch.OwnerReceiptRefs = append(batch.OwnerReceiptRefs, "receipt/"+owner+"/"+binding.BindingID)
		}
	}
	batch.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(batch)
	if err := batch.ValidateForRequest(request); err != nil {
		t.Fatalf("fixture batch: %v", err)
	}
	return batch
}
