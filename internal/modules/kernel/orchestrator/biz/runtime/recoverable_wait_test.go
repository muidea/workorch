package runtime

import (
	"context"
	"testing"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeRecoverableWaitClaimDeferRestartConfirmAndRetire(t *testing.T) {
	ctx := context.Background()
	controller, repository, initialized, prerequisite := runtimeRecoverableWaitController(t)
	base := time.Date(2026, time.August, 21, 10, 0, 0, 0, time.UTC)
	wait := runtimeRecoverableWaitRecord(t, initialized, prerequisite, base, base.Add(time.Second), base.Add(time.Minute))
	opened, err := controller.driveDelivery.openRecoverableWait(ctx, wait)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	duplicate, err := controller.driveDelivery.openRecoverableWait(ctx, wait)
	if err != nil || duplicate.StateRevision != opened.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate open aggregate=%+v saves=%d/%d err=%v", duplicate, repository.saves, saves, err)
	}

	requested := runtimecontract.RuntimeDriveLease{
		WorkspaceID: opened.Facts.WorkspaceID, RunID: opened.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonRecoverableWait, FailureSignature: "recoverable-wait/" + wait.WaitID,
		DueAt: wait.NextEligibleAt,
	}
	_, lease, _, err := controller.scheduleDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, requested)
	if err != nil {
		t.Fatal(err)
	}
	_, running, claimed, err := controller.claimDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, lease, wait.NextEligibleAt)
	if err != nil || !claimed {
		t.Fatalf("claim drive lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	_, accepted, err := controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, base.Format(time.RFC3339Nano))
	if err != nil || accepted {
		t.Fatalf("early probe claim accepted=%v err=%v", accepted, err)
	}
	_, accepted, err = controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration+1, wait.NextEligibleAt)
	if err != nil || accepted {
		t.Fatalf("stale generation probe claim accepted=%v err=%v", accepted, err)
	}
	claimedAggregate, accepted, err := controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.NextEligibleAt)
	if err != nil || !accepted || claimedAggregate.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		t.Fatalf("probe claim aggregate=%+v accepted=%v err=%v", claimedAggregate.RecoverableWaits, accepted, err)
	}
	saves = repository.saves
	_, accepted, err = controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.NextEligibleAt)
	if err != nil || !accepted || repository.saves != saves {
		t.Fatalf("duplicate probe claim accepted=%v saves=%d/%d err=%v", accepted, repository.saves, saves, err)
	}

	unknownHash := runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash)
	deferred, err := controller.driveDelivery.deferRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, wait.NextEligibleAt, unknownHash)
	if err != nil || deferred.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitWaiting || deferred.RecoverableWaits[0].Attempt != 0 {
		t.Fatalf("deferred wait=%+v err=%v", deferred.RecoverableWaits, err)
	}
	allocation, _ := runtimecontract.RuntimeBudgetAllocationForID(deferred.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if allocation.State != runtimecontract.RuntimeBudgetAllocationUnknown {
		t.Fatalf("deferred probe allocation=%+v", allocation)
	}
	if len(deferred.ExecutionBudget.Reconciliations) != 1 || deferred.ExecutionBudget.Reconciliations[0].Status != runtimecontract.RuntimeBudgetReconciliationPending {
		t.Fatalf("deferred probe reconciliation=%+v", deferred.ExecutionBudget.Reconciliations)
	}
	stillUnknown := deferred
	if err := markRecoverableWaitProbeUnknown(&stillUnknown, deferred.RecoverableWaits[0], unknownHash); err != nil {
		t.Fatal(err)
	}
	if len(stillUnknown.ExecutionBudget.Reconciliations) != 1 || stillUnknown.ExecutionBudget.Reconciliations[0].Status != runtimecontract.RuntimeBudgetReconciliationStillUnknown {
		t.Fatalf("repeated unknown probe reconciliation=%+v", stillUnknown.ExecutionBudget.Reconciliations)
	}
	if err := markRecoverableWaitProbeUnknown(&stillUnknown, deferred.RecoverableWaits[0], unknownHash); err != nil {
		t.Fatalf("duplicate still-unknown probe result: %v", err)
	}

	recovered, recoveredLease, changed, err := controller.recoverDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, deferred.RecoverableWaits[0].NextEligibleAt)
	if err != nil || !changed || recoveredLease.RecoveryGeneration == running.RecoveryGeneration {
		t.Fatalf("recovered aggregate=%+v lease=%+v changed=%v err=%v", recovered, recoveredLease, changed, err)
	}
	_, rerun, claimed, err := controller.claimDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, recoveredLease, recoveredLease.DueAt)
	if err != nil || !claimed {
		t.Fatalf("recovered lease claim=%+v claimed=%v err=%v", rerun, claimed, err)
	}
	reclaimed, accepted, err := controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, rerun.RecoveryGeneration, recoveredLease.DueAt)
	if err != nil || !accepted || reclaimed.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		t.Fatalf("reclaimed wait=%+v accepted=%v err=%v", reclaimed.RecoverableWaits, accepted, err)
	}

	result, probe := runtimeProviderCeilingProbeResult(t, reclaimed, prerequisite, wait)
	satisfied, err := controller.authority.commitPrerequisite(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, result, &probe)
	if err != nil {
		t.Fatal(err)
	}
	if satisfied.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitConditionSatisfied || satisfied.RecoverableWaits[0].Attempt != 1 {
		t.Fatalf("satisfied wait=%+v", satisfied.RecoverableWaits)
	}
	allocation, _ = runtimecontract.RuntimeBudgetAllocationForID(satisfied.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if allocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed {
		t.Fatalf("confirmed probe allocation=%+v", allocation)
	}
	if len(satisfied.ExecutionBudget.Reconciliations) != 1 || satisfied.ExecutionBudget.Reconciliations[0].Status != runtimecontract.RuntimeBudgetReconciliationConfirmed ||
		satisfied.ExecutionBudget.Reconciliations[0].OwnerResultHash != probe.OwnerResultHash {
		t.Fatalf("confirmed probe reconciliation=%+v", satisfied.ExecutionBudget.Reconciliations)
	}
	saves = repository.saves
	if _, err := controller.authority.commitPrerequisite(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, result, &probe); err == nil || repository.saves != saves {
		t.Fatalf("duplicate probe result err=%v saves=%d/%d", err, repository.saves, saves)
	}
	retired, err := controller.driveDelivery.retireRecoverableWait(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID)
	if err != nil || retired.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitRetired {
		t.Fatalf("retired wait=%+v err=%v", retired.RecoverableWaits, err)
	}
}

func TestRuntimeAdministrativeTerminationAtomicallyRetiresActiveWaitLeaseAndAllocation(t *testing.T) {
	for _, test := range []struct {
		name           string
		claimProbe     bool
		wantWait       runtimecontract.RuntimeRecoverableWaitStatus
		wantAllocation runtimecontract.RuntimeBudgetAllocationState
	}{
		{name: "waiting", wantWait: runtimecontract.RuntimeRecoverableWaitExpired, wantAllocation: runtimecontract.RuntimeBudgetAllocationReleased},
		{name: "probe claimed", claimProbe: true, wantWait: runtimecontract.RuntimeRecoverableWaitExhausted, wantAllocation: runtimecontract.RuntimeBudgetAllocationExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			controller, repository, initialized, prerequisite := runtimeRecoverableWaitController(t)
			base := time.Date(2026, time.August, 21, 10, 0, 0, 0, time.UTC)
			wait := runtimeRecoverableWaitRecord(t, initialized, prerequisite, base, base.Add(time.Second), base.Add(time.Minute))
			opened, err := controller.driveDelivery.openRecoverableWait(ctx, wait)
			if err != nil {
				t.Fatal(err)
			}
			requested := runtimecontract.RuntimeDriveLease{
				WorkspaceID: opened.Facts.WorkspaceID, RunID: opened.Facts.RunID,
				Reason: runtimecontract.RuntimeDriveLeaseReasonRecoverableWait, FailureSignature: "recoverable-wait/" + wait.WaitID,
				DueAt: wait.NextEligibleAt,
			}
			leased, lease, changed, err := controller.scheduleDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, requested)
			if err != nil || !changed || leased.DriveLease == nil {
				t.Fatalf("schedule wait lease aggregate=%+v lease=%+v changed=%v err=%v", leased, lease, changed, err)
			}
			beforeTermination := leased
			if test.claimProbe {
				_, running, claimed, claimErr := controller.claimDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, lease, wait.NextEligibleAt)
				if claimErr != nil || !claimed {
					t.Fatalf("claim wait lease=%+v claimed=%v err=%v", running, claimed, claimErr)
				}
				beforeTermination, claimed, err = controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.NextEligibleAt)
				if err != nil || !claimed {
					t.Fatalf("claim wait probe aggregate=%+v claimed=%v err=%v", beforeTermination, claimed, err)
				}
			}
			saves := repository.saves
			terminated, err := controller.requestTermination(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancel")
			if err != nil {
				t.Fatal(err)
			}
			allocation, found := runtimecontract.RuntimeBudgetAllocationForID(terminated.ExecutionBudget.Allocations, wait.BudgetAllocationID)
			if !found || terminated.Termination == nil || terminated.Termination.Revision != terminated.StateRevision ||
				terminated.StateRevision != beforeTermination.StateRevision+1 || terminated.DriveLease != nil ||
				terminated.RecoverableWaits[0].Status != test.wantWait || allocation.State != test.wantAllocation || repository.saves != saves+1 {
				t.Fatalf("termination did not atomically retire SM-4 work: termination=%+v wait=%+v allocation=%+v saves=%d/%d", terminated.Termination, terminated.RecoverableWaits, allocation, repository.saves, saves)
			}
			if test.claimProbe && (len(terminated.ExecutionBudget.Reconciliations) != 1 || terminated.ExecutionBudget.Reconciliations[0].Status != runtimecontract.RuntimeBudgetReconciliationFailed) {
				t.Fatalf("claimed probe termination did not close conservative reconciliation: %+v", terminated.ExecutionBudget.Reconciliations)
			}
		})
	}
}

func TestRuntimeRecoverableWaitDeadlineAndContractFailureAreTerminal(t *testing.T) {
	for _, test := range []struct {
		name       string
		exhausted  bool
		wantStatus runtimecontract.RuntimeRecoverableWaitStatus
		wantCode   string
		wantBudget runtimecontract.RuntimeBudgetAllocationState
	}{
		{name: "deadline before probe", wantStatus: runtimecontract.RuntimeRecoverableWaitExpired, wantCode: "recoverable_wait_expired", wantBudget: runtimecontract.RuntimeBudgetAllocationReleased},
		{name: "probe contract failure", exhausted: true, wantStatus: runtimecontract.RuntimeRecoverableWaitExhausted, wantCode: "recoverable_wait_exhausted", wantBudget: runtimecontract.RuntimeBudgetAllocationExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			controller, _, initialized, prerequisite := runtimeRecoverableWaitController(t)
			base := time.Date(2026, time.August, 21, 11, 0, 0, 0, time.UTC)
			wait := runtimeRecoverableWaitRecord(t, initialized, prerequisite, base, base.Add(time.Second), base.Add(2*time.Second))
			opened, err := controller.driveDelivery.openRecoverableWait(ctx, wait)
			if err != nil {
				t.Fatal(err)
			}
			_, lease, _, err := controller.scheduleDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, runtimecontract.RuntimeDriveLease{
				WorkspaceID: opened.Facts.WorkspaceID, RunID: opened.Facts.RunID,
				Reason: runtimecontract.RuntimeDriveLeaseReasonRecoverableWait, FailureSignature: "recoverable-wait/" + wait.WaitID,
				DueAt: wait.NextEligibleAt,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, running, claimed, err := controller.claimDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, lease, wait.NextEligibleAt)
			if err != nil || !claimed {
				t.Fatalf("claim lease=%+v claimed=%v err=%v", running, claimed, err)
			}
			var terminal RuntimeAggregate
			if test.exhausted {
				claimedAggregate, accepted, claimErr := controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.NextEligibleAt)
				if claimErr != nil || !accepted {
					t.Fatalf("claim probe=%v err=%v", accepted, claimErr)
				}
				terminal, err = controller.driveDelivery.exhaustRecoverableWait(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash), "probe_contract_failed")
				_ = claimedAggregate
			} else {
				terminal, _, err = controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.Deadline)
			}
			if err != nil || terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != test.wantCode || terminal.RecoverableWaits[0].Status != test.wantStatus {
				t.Fatalf("terminal aggregate=%+v wait=%+v err=%v", terminal.Decision, terminal.RecoverableWaits, err)
			}
			allocation, _ := runtimecontract.RuntimeBudgetAllocationForID(terminal.ExecutionBudget.Allocations, wait.BudgetAllocationID)
			if allocation.State != test.wantBudget {
				t.Fatalf("terminal allocation=%+v", allocation)
			}
			if test.exhausted && (len(terminal.ExecutionBudget.Reconciliations) != 1 || terminal.ExecutionBudget.Reconciliations[0].Status != runtimecontract.RuntimeBudgetReconciliationFailed) {
				t.Fatalf("exhausted probe reconciliation=%+v", terminal.ExecutionBudget.Reconciliations)
			}
		})
	}
}

func TestRuntimeRecoverableWaitResultAtDeadlineConfirmsProbeButCannotSatisfyCondition(t *testing.T) {
	ctx := context.Background()
	controller, repository, initialized, prerequisite := runtimeRecoverableWaitController(t)
	base := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	wait := runtimeRecoverableWaitRecord(t, initialized, prerequisite, base, base.Add(time.Second), base.Add(2*time.Second))
	opened, err := controller.driveDelivery.openRecoverableWait(ctx, wait)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, _, err := controller.scheduleDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: opened.Facts.WorkspaceID, RunID: opened.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonRecoverableWait, FailureSignature: "recoverable-wait/" + wait.WaitID,
		DueAt: wait.NextEligibleAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, running, claimed, err := controller.claimDriveLease(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, lease, wait.NextEligibleAt)
	if err != nil || !claimed {
		t.Fatalf("claim lease=%+v claimed=%v err=%v", running, claimed, err)
	}
	claimedAggregate, accepted, err := controller.driveDelivery.claimRecoverableWaitProbe(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, wait.WaitID, running.RecoveryGeneration, wait.NextEligibleAt)
	if err != nil || !accepted {
		t.Fatalf("claim probe accepted=%v err=%v", accepted, err)
	}
	result, probe := runtimeProviderCeilingProbeResult(t, claimedAggregate, prerequisite, wait)
	probe.ObservedAt = wait.Deadline
	badProbe := probe
	badProbe.OwnerResultHash = "mismatched-owner-result"
	saves := repository.saves
	if _, err := controller.authority.commitPrerequisite(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, result, &badProbe); err == nil || repository.saves != saves {
		t.Fatalf("mismatched late result err=%v saves=%d/%d", err, repository.saves, saves)
	}
	terminal, err := controller.authority.commitPrerequisite(ctx, opened.Facts.WorkspaceID, opened.Facts.RunID, result, &probe)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "recoverable_wait_expired" || terminal.RecoverableWaits[0].Status != runtimecontract.RuntimeRecoverableWaitExpired ||
		terminal.RecoverableWaits[0].Attempt != 1 || terminal.Facts.Resources.ProviderCeiling != nil {
		t.Fatalf("late probe terminal=%+v wait=%+v ceiling=%+v", terminal.Decision, terminal.RecoverableWaits, terminal.Facts.Resources.ProviderCeiling)
	}
	allocation, _ := runtimecontract.RuntimeBudgetAllocationForID(terminal.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if allocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed {
		t.Fatalf("late probe allocation=%+v", allocation)
	}
}

func runtimeRecoverableWaitController(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate, runtimecontract.RuntimePrerequisite) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), []runtimecontract.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 64 * 1024, Reason: "provider ceiling required",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(initialized.Facts.Prerequisites.Pending) != 1 || initialized.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindProviderCeiling {
		t.Fatalf("provider prerequisite=%+v", initialized.Facts.Prerequisites.Pending)
	}
	return controller, repository, initialized, initialized.Facts.Prerequisites.Pending[0]
}

func runtimeRecoverableWaitRecord(t *testing.T, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, opened, next, deadline time.Time) runtimecontract.RuntimeRecoverableWaitRecord {
	t.Helper()
	authorityID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	probeHash := runtimecontract.StableRuntimeProviderCeilingProbeRequestHash(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, prerequisite.ID,
		aggregate.Facts.Activation.ProviderSelection, prerequisite.MinimumBytes,
	)
	record := runtimecontract.RuntimeRecoverableWaitRecord{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, AuthorityTransactionID: authorityID,
		StateRevisionOpened: aggregate.StateRevision + 1,
		ReasonCode:          runtimecontract.RuntimeRecoverableWaitReasonProviderCeilingUnavailable,
		ProbeKind:           runtimecontract.RuntimeRecoverableWaitProbeProviderCeiling, ProbeOwner: runtimecontract.RuntimeModuleLLM,
		ProbeRequestHash:      probeHash,
		RecoveryConditionHash: runtimecontract.StableRuntimeProviderCeilingRecoveryConditionHash(aggregate.Facts.DefinitionRevision, authorityID, prerequisite.ID),
		OpenedAt:              opened.Format(time.RFC3339Nano), NextEligibleAt: next.Format(time.RFC3339Nano), Deadline: deadline.Format(time.RFC3339Nano),
		MaxAttempts: 1, BudgetDimension: runtimecontract.RuntimeBudgetDimensionRecoveryProbe,
		BudgetPolicyRevision: aggregate.ExecutionBudget.Policy.PolicyRevision, Status: runtimecontract.RuntimeRecoverableWaitWaiting,
	}
	record.WaitID = runtimecontract.StableRuntimeRecoverableWaitID(record)
	preview, allocations, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension: record.BudgetDimension, LogicalConsumptionID: record.WaitID, ScopeRef: record.ProbeRequestHash, ReservedAmount: 1,
	}})
	if err != nil || len(allocations) != 1 {
		t.Fatalf("reserve wait budget allocations=%+v err=%v", allocations, err)
	}
	record.BudgetAllocationID = allocations[0].AllocationID
	if err := record.Validate(preview); err != nil {
		t.Fatal(err)
	}
	return record
}

func runtimeProviderCeilingProbeResult(t *testing.T, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, wait runtimecontract.RuntimeRecoverableWaitRecord) (PrerequisiteResult, runtimecontract.RuntimeRecoverableWaitProbeObservation) {
	t.Helper()
	ceiling := runtimecontract.ProviderCeiling{
		Selection:           aggregate.Facts.Activation.ProviderSelection,
		ContextWindowTokens: 128000, ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 8192,
		MaxInputTokens: 119808, MaxBytes: 128 * 1024, CapabilityRevision: "capability-1", ConfigurationRevision: "config-1",
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		ContentHash: "provider-ceiling-content-1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			SubjectRef: "ai-proxy/gpt-4o", Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	probe := runtimecontract.RuntimeRecoverableWaitProbeObservation{
		WaitID: wait.WaitID, ProbeRequestHash: wait.ProbeRequestHash, OwnerResultHash: observation.ObservationID,
		ObservedAt: wait.NextEligibleAt,
	}
	return PrerequisiteResult{PrerequisiteID: prerequisite.ID, ProviderCeiling: &ceiling, Observation: &observation}, probe
}
