package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCWIBindingFenceRejectsCandidateStagesWithoutMutation(t *testing.T) {
	ctx := context.Background()

	newController := func(t *testing.T) (*RuntimeController, *controllerRepository) {
		t.Helper()
		aggregate := stateChangeAggregate(t)
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		return controller, repository
	}
	assertUnchanged := func(t *testing.T, repository *controllerRepository, before RuntimeAggregate, saves int) {
		t.Helper()
		if repository.saves != saves || !reflect.DeepEqual(*repository.aggregate, before) {
			t.Fatalf("stale CWI binding mutated aggregate: saves=%d want=%d aggregate=%+v", repository.saves, saves, repository.aggregate)
		}
	}

	t.Run("request", func(t *testing.T) {
		controller, repository := newController(t)
		manifest := runtimeCWIManifestForBindingFenceTest(t, *repository.aggregate)
		runtimeTestRemoveCurrentCWIBinding(t, repository.aggregate)
		before := *repository.aggregate
		saves := repository.saves

		_, err := controller.beginCandidateAttempt(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, "cwi-binding-request", manifest, "cwi-binding-request")
		if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "candidate request binding fence") {
			t.Fatalf("candidate request stale binding error = %v", err)
		}
		assertUnchanged(t, repository, before, saves)
	})

	t.Run("proposal", func(t *testing.T) {
		controller, repository := newController(t)
		pending, err := controller.BeginCandidateAttemptForTest(ctx, repository.aggregate.Facts.WorkspaceID, repository.aggregate.Facts.RunID, 0, "cwi-binding-proposal")
		if err != nil {
			t.Fatal(err)
		}
		manifest, logicalRequestID := runtimeCWIPendingCandidateBindingForFenceTest(t, controller, pending)
		candidate := runtimeTestBindCandidateToCurrentFocus(t, pending, stateChangeCandidate())
		candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			t.Fatal(err)
		}
		runtimeTestRemoveCurrentCWIBinding(t, repository.aggregate)
		before := *repository.aggregate
		saves := repository.saves

		_, err = controller.admitCandidateProposal(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, manifest, logicalRequestID, candidate)
		if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "candidate proposal binding fence") {
			t.Fatalf("candidate proposal stale binding error = %v", err)
		}
		assertUnchanged(t, repository, before, saves)
	})

	t.Run("seal", func(t *testing.T) {
		controller, repository := newController(t)
		pending, err := controller.BeginCandidateAttemptForTest(ctx, repository.aggregate.Facts.WorkspaceID, repository.aggregate.Facts.RunID, 0, "cwi-binding-seal")
		if err != nil {
			t.Fatal(err)
		}
		manifest, logicalRequestID := runtimeCWIPendingCandidateBindingForFenceTest(t, controller, pending)
		candidate := runtimeTestBindCandidateToCurrentFocus(t, pending, stateChangeCandidate())
		candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.admitCandidateProposal(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, 0, manifest, logicalRequestID, candidate); err != nil {
			t.Fatal(err)
		}
		if err := RecordFixtureCandidateFeasibilities(ctx, controller, pending.Facts.WorkspaceID, pending.Facts.RunID, 0, candidate); err != nil {
			t.Fatal(err)
		}
		runtimeTestRemoveCurrentCWIBinding(t, repository.aggregate)
		before := *repository.aggregate
		saves := repository.saves

		result, err := controller.admitAndSealCandidateAttempt(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, manifest, logicalRequestID, candidate)
		if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "candidate dispatch binding fence") {
			t.Fatalf("candidate seal stale binding result=%+v err=%v", result, err)
		}
		if repository.aggregate.ActiveDispatch != nil {
			t.Fatalf("stale CWI binding sealed a dispatch: %+v", repository.aggregate.ActiveDispatch)
		}
		assertUnchanged(t, repository, before, saves)
	})
}

func TestRuntimeCWIBindingFenceRejectsStaleBindingSetBeforeCandidateRequest(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := runtimeCWIManifestForBindingFenceTest(t, aggregate)
	repository.aggregate.Facts.ReconciliationContract.BindingSetHash = strings.Repeat("0", 64)
	before := *repository.aggregate
	saves := repository.saves

	_, err = controller.beginCandidateAttempt(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, "stale-binding-set", manifest, "stale-binding-set")
	if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "candidate request binding fence") {
		t.Fatalf("candidate request stale binding-set error = %v", err)
	}
	if repository.saves != saves || !reflect.DeepEqual(*repository.aggregate, before) {
		t.Fatalf("stale binding-set mutated aggregate: saves=%d want=%d aggregate=%+v", repository.saves, saves, repository.aggregate)
	}
}

func TestRuntimeCWIBindingFenceRejectsReceiptAndBlocksOwnerExecution(t *testing.T) {
	ctx := context.Background()

	t.Run("receipt", func(t *testing.T) {
		controller, repository, sealed := runtimeCWISealedDispatchForBindingFenceTest(t)
		operationID := runtimeTestActiveOperationID(t, sealed, 0)
		claimed, err := controller.claimOperationExecution(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, operationID)
		if err != nil {
			t.Fatal(err)
		}
		execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
		if !found {
			t.Fatal("claimed operation is missing")
		}
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
			Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: execution.ExpectedEffectScope,
			Changed: true, TargetRefs: []string{"main.go"}, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner result"},
		}
		runtimeTestRemoveCurrentCWIBinding(t, repository.aggregate)
		before := *repository.aggregate
		saves := repository.saves

		_, _, err = controller.commitOperationReceipt(ctx, before.Facts.WorkspaceID, before.Facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt)
		if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "operation receipt binding fence") {
			t.Fatalf("operation receipt stale binding error = %v", err)
		}
		if repository.saves != saves || !reflect.DeepEqual(*repository.aggregate, before) {
			t.Fatalf("stale CWI receipt mutated aggregate: saves=%d want=%d aggregate=%+v", repository.saves, saves, repository.aggregate)
		}
	})

	t.Run("execution", func(t *testing.T) {
		controller, repository, sealed := runtimeCWISealedDispatchForBindingFenceTest(t)
		runtimeTestRemoveCurrentCWIBinding(t, repository.aggregate)
		before := *repository.aggregate
		saves := repository.saves
		executor := &failIfExecutedOperationExecutor{}

		if _, err := controller.executeActiveDispatch(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, executor); err == nil {
			t.Fatal("stale CWI binding allowed active dispatch execution")
		}
		if executor.called {
			t.Fatal("stale CWI binding invoked the capability owner")
		}
		if repository.saves != saves || !reflect.DeepEqual(*repository.aggregate, before) {
			t.Fatalf("stale CWI execution mutated aggregate: saves=%d want=%d aggregate=%+v", repository.saves, saves, repository.aggregate)
		}
	})
}

func TestRuntimeCWIBindingFenceRejectsFocusSettlementWithoutMutation(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	runtimeTestRemoveCurrentCWIBinding(t, &aggregate)
	before := aggregate.Facts

	if _, err := settleRuntimeCWIsForCurrentFocus(aggregate.Facts); err == nil {
		t.Fatal("focus settlement accepted a missing current CWI binding")
	}
	if !reflect.DeepEqual(aggregate.Facts, before) {
		t.Fatalf("focus settlement mutated stale CWI facts: before=%+v after=%+v", before, aggregate.Facts)
	}
}

func runtimeCWISealedDispatchForBindingFenceTest(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	t.Helper()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, stateChangeCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil {
		t.Fatal("binding-fence fixture did not seal a dispatch")
	}
	return controller, repository, sealed
}

func runtimeCWIManifestForBindingFenceTest(t *testing.T, aggregate RuntimeAggregate) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func runtimeCWIPendingCandidateBindingForFenceTest(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate) (runtimecontract.RuntimeCandidateContractManifest, string) {
	t.Helper()
	manifest := runtimeCWIManifestForBindingFenceTest(t, aggregate)
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Attempt == 0 && attempt.Transaction == aggregate.Decision.AuthorityTransaction() {
			return manifest, attempt.LogicalRequestID
		}
	}
	t.Fatal("binding-fence fixture has no pending candidate attempt")
	return runtimecontract.RuntimeCandidateContractManifest{}, ""
}

func runtimeTestRemoveCurrentCWIBinding(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil || len(aggregate.Facts.Focus.CoverageWorkItemIDs) == 0 {
		t.Fatal("binding-fence fixture has no current focus CWI")
	}
	coverageID := aggregate.Facts.Focus.CoverageWorkItemIDs[0]
	for index := range aggregate.Facts.Coverage {
		item := &aggregate.Facts.Coverage[index]
		if item.ID != coverageID {
			continue
		}
		item.CurrentBindingRef = nil
		item.BindingLineage = nil
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
		item.State = runtimecontract.RuntimeCoverageItemStateOpen
		return
	}
	t.Fatalf("binding-fence fixture has no coverage row for %q", coverageID)
}
