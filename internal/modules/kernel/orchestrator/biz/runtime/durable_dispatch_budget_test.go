package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDurableDispatchBudgetSealCASRestartDuplicateAndStale(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			a := executionLimitedAggregate(t, 16, 16, func(a *RuntimeAggregate) {
				a.Facts.Activation.ExecutionContract.ExecutionLimits.MaxDurableDispatchesPerCWI = 1
			})
			repo := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: a}, t: t}
			controller, err := NewRuntimeController(repo)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := controller.BeginCandidateAttemptForTest(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, "durable-budget-seal")
			if err != nil {
				t.Fatal(err)
			}
			manifest, logical := runtimeCWIPendingCandidateBindingForFenceTest(t, controller, pending)
			candidate := runtimeTestBindCandidateToCurrentFocus(t, pending, stateChangeCandidate())
			candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.admitCandidateProposal(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, logical, candidate); err != nil {
				t.Fatal(err)
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, controller, a.Facts.WorkspaceID, a.Facts.RunID, 0, candidate); err != nil {
				t.Fatal(err)
			}
			before := reloadExecutionLimitedAggregate(t, repo.aggregate)
			repo.mode, repo.fired = mode, false
			_, err = controller.admitAndSealCandidateAttempt(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, logical, candidate)
			if (mode == providerTerminalCASNormal) != (err == nil) {
				t.Fatalf("CAS result: %v", err)
			}
			if mode == providerTerminalCASFailBeforeWrite && !reflect.DeepEqual(before, reloadExecutionLimitedAggregate(t, repo.aggregate)) {
				t.Fatal("failed CAS leaked seal reservation")
			}
			repo.aggregate = reloadExecutionLimitedAggregate(t, repo.aggregate)
			repo.mode = providerTerminalCASNormal
			fresh, err := NewRuntimeController(repo)
			if err != nil {
				t.Fatal(err)
			}
			result, err := fresh.admitAndSealCandidateAttempt(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, logical, candidate)
			if err != nil {
				t.Fatal(err)
			}
			usage, err := runtimecontract.RuntimeDurableDispatchUsage(sealedDispatchRecord(result.Aggregate.ActiveDispatch), sealedDispatchesToRuntimeRecords(result.Aggregate.CompletedDispatches), result.Aggregate.CandidateAttempts)
			if err != nil || len(usage) == 0 {
				t.Fatalf("missing seal reservation: %v %v", usage, err)
			}
			for _, used := range usage {
				if used != 1 {
					t.Fatal("duplicate seal reservation")
				}
			}
			writes := repo.saves
			if _, err := fresh.admitAndSealCandidateAttempt(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, logical, candidate); err != nil || repo.saves != writes {
				t.Fatalf("duplicate wrote: %v", err)
			}
			if _, err := fresh.admitAndSealCandidateAttempt(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, "stale-logical", candidate); err == nil || repo.saves != writes {
				t.Fatal("stale seal accepted")
			}
			if !reflect.DeepEqual(before.ExecutionBudget, repo.aggregate.ExecutionBudget) || len(repo.aggregate.OperationReceipts) != 0 {
				t.Fatal("logical seal allocated physical budget or effect")
			}
		})
	}
}

func TestTerminalFailureRetainsPendingFeasibilityCatalogInSameCAS(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			a := executionLimitedAggregate(t, 16, 16)
			repo := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: a}, t: t}
			controller, err := NewRuntimeController(repo)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := controller.BeginCandidateAttemptForTest(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, "terminal-catalog")
			if err != nil {
				t.Fatal(err)
			}
			manifest, logical := runtimeCWIPendingCandidateBindingForFenceTest(t, controller, pending)
			candidate := runtimeTestBindCandidateToCurrentFocus(t, pending, stateChangeCandidate())
			candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.admitCandidateProposal(ctx, a.Facts.WorkspaceID, a.Facts.RunID, 0, manifest, logical, candidate); err != nil {
				t.Fatal(err)
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, controller, a.Facts.WorkspaceID, a.Facts.RunID, 0, candidate); err != nil {
				t.Fatal(err)
			}
			before := reloadExecutionLimitedAggregate(t, repo.aggregate)
			if len(before.CandidateAttempts[0].Feasibilities) == 0 {
				t.Fatal("fixture has no feasibility fact")
			}
			fact := runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionBudgetExhausted, Code: "execution_budget_exhausted", Signature: "durable-dispatch-test", Message: "durable dispatch limit exhausted"}
			repo.mode, repo.fired = mode, false
			_, err = controller.recordDriveFailureFact(ctx, a.Facts.WorkspaceID, a.Facts.RunID, fact)
			if (mode == providerTerminalCASNormal) != (err == nil) {
				t.Fatalf("CAS result: %v", err)
			}
			if mode == providerTerminalCASFailBeforeWrite && !reflect.DeepEqual(before, reloadExecutionLimitedAggregate(t, repo.aggregate)) {
				t.Fatal("failed CAS leaked terminal cleanup")
			}
			repo.mode = providerTerminalCASNormal
			fresh, err := NewRuntimeController(repo)
			if err != nil {
				t.Fatal(err)
			}
			result, err := fresh.recordDriveFailureFact(ctx, a.Facts.WorkspaceID, a.Facts.RunID, fact)
			if err != nil {
				t.Fatal(err)
			}
			result = reloadExecutionLimitedAggregate(t, result)
			attempt := result.CandidateAttempts[0]
			if result.Decision.State != runtimecontract.ConvergenceStateFailed || attempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed || len(attempt.FeasibilityCatalogs) == 0 {
				t.Fatal("terminal lost feasibility preimage")
			}
			if !reflect.DeepEqual(before.ExecutionBudget, result.ExecutionBudget) || result.ActiveDispatch != nil || len(result.OperationReceipts) != 0 {
				t.Fatal("terminal allocated work or refunded physical calls")
			}
			writes := repo.saves
			if _, err := fresh.recordDriveFailureFact(ctx, a.Facts.WorkspaceID, a.Facts.RunID, fact); err != nil || repo.saves != writes {
				t.Fatalf("terminal replay wrote: %v", err)
			}
			for idx := range result.CandidateAttempts {
				result.CandidateAttempts[idx].FeasibilityCatalogs = nil
			}
			if result.Validate() == nil {
				t.Fatal("missing historical catalog accepted on reload")
			}
		})
	}
}
