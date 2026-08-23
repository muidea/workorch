package readmodelprojection

import (
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeReadModelsProjectRecoverableWaitDistinctFromGovernanceAndTerminal(t *testing.T) {
	state := runtimeRecoverableWaitProjectionState(t)
	progress := mapRuntimeProgress(state)
	if progress.RecoverableWait == nil || progress.RecoverableWait.WaitID != state.RecoverableWaits[0].WaitID ||
		progress.ExecutionBudget == nil || progress.ExecutionBudget.ActiveWaitID != state.RecoverableWaits[0].WaitID ||
		progress.TerminalCode != "" {
		t.Fatalf("Runtime progress recoverable wait=%+v budget=%+v", progress.RecoverableWait, progress.ExecutionBudget)
	}
	assertProjectedRuntimeBudgetDimension(t, progress.ExecutionBudget, runtimecontract.RuntimeBudgetDimensionOperationRecovery, 1)
	assertProjectedRuntimeBudgetDimension(t, progress.ExecutionBudget, runtimecontract.RuntimeBudgetDimensionSameFocusRepair, 2)
	acceptance := mapAcceptanceProjection(&runmodel.Run{ID: "run-wait", RuntimeState: &state})
	if acceptance.RecoverableWait == nil || acceptance.RecoverableWait.WaitID != state.RecoverableWaits[0].WaitID ||
		acceptance.ExecutionBudget == nil || acceptance.ExecutionBudget.ActiveWaitID != state.RecoverableWaits[0].WaitID ||
		len(acceptance.Governance) != 0 {
		t.Fatalf("acceptance recoverable wait=%+v budget=%+v governance=%+v", acceptance.RecoverableWait, acceptance.ExecutionBudget, acceptance.Governance)
	}
}

func runtimeRecoverableWaitProjectionState(t *testing.T) runtimecontract.RuntimeStateRecord {
	t.Helper()
	ledger, err := runtimecontract.NewRuntimeExecutionBudgetLedger(runtimecontract.DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	wait := runtimecontract.RuntimeRecoverableWaitRecord{
		WorkspaceID: "ws-wait", RunID: "run-wait", DefinitionRevision: "definition-wait", AuthorityTransactionID: "authority-wait",
		StateRevisionOpened: 2, ReasonCode: runtimecontract.RuntimeRecoverableWaitReasonProviderCeilingUnavailable,
		ProbeKind: runtimecontract.RuntimeRecoverableWaitProbeProviderCeiling, ProbeOwner: runtimecontract.RuntimeModuleLLM,
		ProbeRequestHash: "provider-ceiling-probe", RecoveryConditionHash: "provider-ceiling-condition",
		OpenedAt: base.Format(time.RFC3339Nano), NextEligibleAt: base.Add(time.Second).Format(time.RFC3339Nano), Deadline: base.Add(time.Minute).Format(time.RFC3339Nano),
		MaxAttempts: 1, BudgetDimension: runtimecontract.RuntimeBudgetDimensionRecoveryProbe,
		BudgetPolicyRevision: ledger.Policy.PolicyRevision, Status: runtimecontract.RuntimeRecoverableWaitWaiting,
	}
	wait.WaitID = runtimecontract.StableRuntimeRecoverableWaitID(wait)
	ledger, allocations, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(ledger, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension: wait.BudgetDimension, LogicalConsumptionID: wait.WaitID, ScopeRef: wait.ProbeRequestHash, ReservedAmount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	wait.BudgetAllocationID = allocations[0].AllocationID
	return runtimecontract.RuntimeStateRecord{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, StateRevision: 3,
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-wait", RunID: "run-wait", DefinitionRevision: "definition-wait", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
			ExecutionBudgetPolicy: ledger.Policy,
			Recovery:              runtimecontract.RuntimeRecoveryLedger{OperationRecoveryAttempts: 1},
			Repair:                runtimecontract.RuntimeRepairLedger{FocusID: "focus-wait", RepairAttempts: 2},
		},
		Decision:        runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedPrerequisite},
		ExecutionBudget: ledger, RecoverableWaits: []runtimecontract.RuntimeRecoverableWaitRecord{wait},
	}
}

func assertProjectedRuntimeBudgetDimension(t *testing.T, view *runtimecontract.RuntimeExecutionBudgetView, dimension runtimecontract.RuntimeBudgetDimension, confirmed int64) {
	t.Helper()
	if view == nil {
		t.Fatal("runtime execution budget projection is missing")
	}
	for _, item := range view.Dimensions {
		if item.Dimension == dimension {
			if item.Confirmed != confirmed {
				t.Fatalf("projected budget dimension %q = %+v", dimension, item)
			}
			return
		}
	}
	t.Fatalf("projected budget dimension %q is missing", dimension)
}
