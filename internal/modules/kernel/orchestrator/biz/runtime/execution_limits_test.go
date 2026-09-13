package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func executionLimitedAggregate(t *testing.T, calls, operations int, prepare ...func(*RuntimeAggregate)) RuntimeAggregate {
	t.Helper()
	return stateChangeAggregateWithPreparation(t, func(a *RuntimeAggregate) {
		a.Facts.Activation.ExecutionContract.ExecutionLimits = &runtimecontract.RuntimeExecutionLimits{
			ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: calls, MaxCapabilityOperations: operations,
		}
		for _, apply := range prepare {
			apply(a)
		}
	})
}

type executionLimitsCASRepository struct {
	*providerTerminalCASRepository
	t *testing.T
}

func (r *executionLimitsCASRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	value, err := r.providerTerminalCASRepository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return reloadExecutionLimitedAggregate(r.t, value), nil
}

func reloadExecutionLimitedAggregate(t *testing.T, a RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeAggregate
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	return restored
}

func TestExecutionLimitsProviderReservationCASAndRestart(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			repository := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: executionLimitedAggregate(t, 1, 2)}, t: t}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			a := repository.aggregate
			_, session, err := controller.ensureCandidateRequestSession(ctx, a.Facts.WorkspaceID, a.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "limits-prefix", "limits-manifest", "")
			if err != nil {
				t.Fatal(err)
			}
			repository.mode, repository.fired = mode, false
			before := reloadExecutionLimitedAggregate(t, repository.aggregate)
			physicalID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
			_, err = controller.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physicalID)
			if (mode == providerTerminalCASNormal) != (err == nil) {
				t.Fatalf("CAS result: %v", err)
			}
			if mode == providerTerminalCASFailBeforeWrite && !reflect.DeepEqual(before, reloadExecutionLimitedAggregate(t, repository.aggregate)) {
				t.Fatal("failed CAS leaked reservation/submission")
			}
			repository.aggregate = reloadExecutionLimitedAggregate(t, repository.aggregate)
			repository.mode = providerTerminalCASNormal
			fresh, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			submitted, err := fresh.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physicalID)
			if err != nil {
				t.Fatal(err)
			}
			saves := repository.saves
			if _, err := fresh.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physicalID); err != nil || saves != repository.saves {
				t.Fatal("duplicate reservation wrote again")
			}
			if _, err := fresh.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, "other-physical"); err == nil || saves != repository.saves {
				t.Fatal("stale physical identity accepted")
			}
			view, err := runtimecontract.BuildRuntimeExecutionBudgetView(a.Facts.WorkspaceID, a.Facts.RunID, submitted.StateRevision, submitted.ExecutionBudget, "", "")
			if err != nil {
				t.Fatal(err)
			}
			assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderCall, 0, 1, 0)
			// A changed request, generation or purpose cannot provide a new Run
			// balance. Both document paths use this same pure allocation compiler.
			for _, kind := range []runtimecontract.RuntimeProviderDocumentKind{runtimecontract.RuntimeProviderDocumentOperations, runtimecontract.RuntimeProviderDocumentSemanticPlan} {
				for _, sequence := range []int{1, 2} {
					specs, err := runtimecontract.RuntimeProviderBudgetAllocationSpecs(submitted.ExecutionBudget, kind, 0, sequence, "new-logical", "new-physical")
					if err != nil {
						t.Fatal(err)
					}
					_, _, _, err = runtimecontract.ReserveRuntimeBudgetAllocations(submitted.ExecutionBudget, specs)
					var exhausted *runtimecontract.RuntimeBudgetExhaustedError
					if !errors.As(err, &exhausted) || exhausted.Dimension != runtimecontract.RuntimeBudgetDimensionProviderCall {
						t.Fatalf("path refreshed budget: %v", err)
					}
				}
			}
			// Even coordinated policy/hash changes must fail against the sealed
			// startup declaration on a same-version aggregate load.
			tampered := reloadExecutionLimitedAggregate(t, submitted)
			for i := range tampered.Facts.ExecutionBudgetPolicy.Dimensions {
				if tampered.Facts.ExecutionBudgetPolicy.Dimensions[i].Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall {
					tampered.Facts.ExecutionBudgetPolicy.Dimensions[i].Limit++
				}
			}
			tampered.Facts.ExecutionBudgetPolicy.PolicyHash = runtimecontract.StableRuntimeExecutionBudgetPolicyHash(tampered.Facts.ExecutionBudgetPolicy)
			tampered.ExecutionBudget.Policy = tampered.Facts.ExecutionBudgetPolicy.Clone()
			if tampered.Validate() == nil {
				t.Fatal("rehashed budget extension accepted")
			}
		})
	}
}

func TestExecutionLimitsOperationClaimCASAndRestart(t *testing.T) {
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			a := executionLimitedAggregate(t, 4, 1, func(a *RuntimeAggregate) {
				a.Facts.Focus.TargetRefs = []string{"main.go", "other.go"}
				catalog := runtimecontract.RuntimeOperationAdmissibility{
					PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
					OperationKinds:    []string{runtimecontract.RuntimeBlockKindPatchApply},
					OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget}}},
					TargetRefs:        []string{"main.go", "other.go"}, ObservedRevision: "tree-v1",
				}
				catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
				a.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
			})
			_, _ = addFreshFileAdmissibilityObservation(t, &a)
			fixture := &durableRuntimeStateRepository{aggregate: a}
			sealed := sealCandidateForTest(t, fixture, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
				{Operation: patchApplyTestOperation("first", "main.go")}, {Operation: patchApplyTestOperation("second", "other.go")},
			}}})
			first, second := runtimeTestActiveOperationIDForTarget(t, sealed, "main.go"), runtimeTestActiveOperationIDForTarget(t, sealed, "other.go")
			repository := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: reloadExecutionLimitedAggregate(t, sealed), mode: mode}, t: t}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			_, err = controller.claimOperationExecution(ctx, a.Facts.WorkspaceID, a.Facts.RunID, first)
			if (mode == providerTerminalCASNormal) != (err == nil) {
				t.Fatalf("claim CAS: %v", err)
			}
			if mode == providerTerminalCASFailBeforeWrite && !reflect.DeepEqual(reloadExecutionLimitedAggregate(t, sealed), reloadExecutionLimitedAggregate(t, repository.aggregate)) {
				t.Fatal("claim leaked before CAS")
			}
			repository.aggregate = reloadExecutionLimitedAggregate(t, repository.aggregate)
			repository.mode = providerTerminalCASNormal
			fresh, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if mode == providerTerminalCASFailBeforeWrite {
				if _, err := fresh.claimOperationExecution(ctx, a.Facts.WorkspaceID, a.Facts.RunID, first); err != nil {
					t.Fatal(err)
				}
			}
			before, saves := reloadExecutionLimitedAggregate(t, repository.aggregate), repository.saves
			if _, err := fresh.claimOperationExecution(ctx, a.Facts.WorkspaceID, a.Facts.RunID, first); err == nil || repository.saves != saves {
				t.Fatal("unresolved operation reclaimed after restart")
			}
			_, err = fresh.claimOperationExecution(ctx, a.Facts.WorkspaceID, a.Facts.RunID, second)
			var exhausted *runtimecontract.RuntimeBudgetExhaustedError
			if !errors.As(err, &exhausted) || exhausted.Dimension != runtimecontract.RuntimeBudgetDimensionCapabilityOperation || saves != repository.saves {
				t.Fatalf("operation budget escaped: %v", err)
			}
			if !reflect.DeepEqual(before, reloadExecutionLimitedAggregate(t, repository.aggregate)) {
				t.Fatal("exhaustion changed dispatch or budget")
			}
			view, err := runtimecontract.BuildRuntimeExecutionBudgetView(a.Facts.WorkspaceID, a.Facts.RunID, before.StateRevision, before.ExecutionBudget, "", "")
			if err != nil {
				t.Fatal(err)
			}
			assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionCapabilityOperation, 0, 1, 0)
		})
	}
}

func TestExecutionLimitsUnknownProviderStillConsumesSharedBudget(t *testing.T) {
	a := executionLimitedAggregate(t, 1, 2)
	policy := a.ExecutionBudget.Policy
	ledger, _, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(a.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{Dimension: runtimecontract.RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "logical", ScopeRef: "physical", ReservedAmount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	a.ExecutionBudget = ledger
	settlement, err := NewRuntimeProviderBudgetUnknownSettlement("logical", "physical")
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := applyCandidateRequestBudgetSettlement(&a, &settlement); err != nil || !changed {
		t.Fatalf("unknown settlement: %v", err)
	}
	view, err := runtimecontract.BuildRuntimeExecutionBudgetView(a.Facts.WorkspaceID, a.Facts.RunID, a.StateRevision, a.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderCall, 0, 0, 1)
	encoded, err := json.Marshal(a.ExecutionBudget)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimecontract.RuntimeExecutionBudgetLedger
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Validate() != nil || !runtimecontract.SameRuntimeExecutionBudgetPolicy(policy, restored.Policy) {
		t.Fatal("budget reload drift")
	}
	_, _, _, err = runtimecontract.ReserveRuntimeBudgetAllocations(restored, []runtimecontract.RuntimeBudgetAllocationSpec{{Dimension: runtimecontract.RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "renamed", ScopeRef: "another", ReservedAmount: 1}})
	var exhausted *runtimecontract.RuntimeBudgetExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("unknown released balance: %v", err)
	}
	if changed, err := applyCandidateRequestBudgetSettlement(&a, &settlement); err != nil || changed {
		t.Fatalf("duplicate unknown changed balance: %v", err)
	}
}
