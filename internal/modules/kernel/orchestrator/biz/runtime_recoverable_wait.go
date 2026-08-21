package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeProviderCeilingWaitInitialDelay = time.Second
	runtimeProviderCeilingWaitDeadline     = 30 * time.Second
	runtimeProviderCeilingWaitMaxAttempts  = 1
)

// runtimeRecoverableWaitPendingError is private SM-4 scheduling state. The
// persisted wait remains the only recovery authority; this error merely asks
// the owner scheduler to install a timer for its sealed NextEligibleAt.
type runtimeRecoverableWaitPendingError struct {
	Wait runtimecontract.RuntimeRecoverableWaitView
}

func (e *runtimeRecoverableWaitPendingError) Error() string {
	if e == nil {
		return "Runtime has a recoverable wait"
	}
	return fmt.Sprintf("Runtime recoverable wait %s is pending until %s", e.Wait.WaitID, e.Wait.NextEligibleAt)
}

func runtimeProviderCeilingPrerequisiteCommand(aggregate runtime.RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite) llmevents.AcquireProviderCeilingCommand {
	return llmevents.AcquireProviderCeilingCommand{
		WorkspaceID:         aggregate.Facts.WorkspaceID,
		RunID:               aggregate.Facts.RunID,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
		PrerequisiteID:      prerequisite.ID,
		Selection:           aggregate.Facts.Activation.ProviderSelection,
		MinimumBytes:        prerequisite.MinimumBytes,
	}
}

func runtimeProviderCeilingProbeRequestHash(command llmevents.AcquireProviderCeilingCommand) string {
	return runtimecontract.StableRuntimeProviderCeilingProbeRequestHash(
		command.WorkspaceID,
		command.RunID,
		command.AuthoritySnapshotID,
		command.PrerequisiteID,
		command.Selection,
		command.MinimumBytes,
	)
}

func (m *Biz) acquireRuntimeProviderCeilingPrerequisite(ctx context.Context, aggregate runtime.RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite) (runtime.PrerequisiteResult, error) {
	command := runtimeProviderCeilingPrerequisiteCommand(aggregate, prerequisite)
	facts, err := AcquireAndCommitProviderCeilingPrerequisite(ctx, m.EventHub(), aggregate.Facts, command)
	if err != nil {
		return runtime.PrerequisiteResult{}, err
	}
	observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
	if !ok {
		return runtime.PrerequisiteResult{}, fmt.Errorf("provider ceiling prerequisite %q did not return an observation", prerequisite.ID)
	}
	return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ProviderCeiling: facts.Resources.ProviderCeiling, Observation: &observation}, nil
}

func runtimeRecoverableWaitEligibleError(err error) bool {
	if err == nil {
		return false
	}
	var timeout interface{ Timeout() bool }
	var temporary interface{ Temporary() bool }
	return errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &timeout) && timeout.Timeout()) ||
		(errors.As(err, &temporary) && temporary.Temporary())
}

func (m *Biz) openRuntimeProviderCeilingRecoverableWait(ctx context.Context, aggregate runtime.RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite) (runtime.RuntimeAggregate, error) {
	if prerequisite.Kind != runtimecontract.PrerequisiteKindProviderCeiling {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	command := runtimeProviderCeilingPrerequisiteCommand(aggregate, prerequisite)
	now := time.Now().UTC()
	record := runtimecontract.RuntimeRecoverableWaitRecord{
		WorkspaceID:            aggregate.Facts.WorkspaceID,
		RunID:                  aggregate.Facts.RunID,
		DefinitionRevision:     aggregate.Facts.DefinitionRevision,
		AuthorityTransactionID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction()),
		StateRevisionOpened:    aggregate.StateRevision + 1,
		ReasonCode:             runtimecontract.RuntimeRecoverableWaitReasonProviderCeilingUnavailable,
		ProbeKind:              runtimecontract.RuntimeRecoverableWaitProbeProviderCeiling,
		ProbeOwner:             runtimecontract.RuntimeModuleLLM,
		ProbeRequestHash:       runtimeProviderCeilingProbeRequestHash(command),
		RecoveryConditionHash:  runtimecontract.StableRuntimeProviderCeilingRecoveryConditionHash(aggregate.Facts.DefinitionRevision, runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction()), prerequisite.ID),
		OpenedAt:               now.Format(time.RFC3339Nano),
		NextEligibleAt:         now.Add(runtimeProviderCeilingWaitInitialDelay).Format(time.RFC3339Nano),
		Deadline:               now.Add(runtimeProviderCeilingWaitDeadline).Format(time.RFC3339Nano),
		MaxAttempts:            runtimeProviderCeilingWaitMaxAttempts,
		BudgetDimension:        runtimecontract.RuntimeBudgetDimensionRecoveryProbe,
		BudgetPolicyRevision:   aggregate.ExecutionBudget.Policy.PolicyRevision,
		Status:                 runtimecontract.RuntimeRecoverableWaitWaiting,
	}
	record.WaitID = runtimecontract.StableRuntimeRecoverableWaitID(record)
	preview, allocations, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension:            record.BudgetDimension,
		LogicalConsumptionID: record.WaitID,
		ScopeRef:             record.ProbeRequestHash,
		ReservedAmount:       1,
	}})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if len(allocations) != 1 {
		return runtime.RuntimeAggregate{}, fmt.Errorf("provider ceiling recoverable wait did not reserve one probe allocation")
	}
	record.BudgetAllocationID = allocations[0].AllocationID
	if err := record.Validate(preview); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return m.runtimeOpenRecoverableWait(ctx, record)
}

func runtimeCurrentRecoverableWait(aggregate runtime.RuntimeAggregate) (runtimecontract.RuntimeRecoverableWaitRecord, bool) {
	for _, raw := range aggregate.RecoverableWaits {
		wait := raw.Clone()
		if wait.Active() {
			return wait, true
		}
	}
	return runtimecontract.RuntimeRecoverableWaitRecord{}, false
}

func runtimeRecoverableWaitPendingRetirement(aggregate runtime.RuntimeAggregate) (runtimecontract.RuntimeRecoverableWaitRecord, bool) {
	for _, raw := range aggregate.RecoverableWaits {
		wait := raw.Clone()
		if wait.Status == runtimecontract.RuntimeRecoverableWaitConditionSatisfied {
			return wait, true
		}
	}
	return runtimecontract.RuntimeRecoverableWaitRecord{}, false
}

func runtimeRecoverableWaitDriveResult(aggregate runtime.RuntimeAggregate) (orchestratorevents.DriveRuntimeResult, error) {
	wait, found := runtimeCurrentRecoverableWait(aggregate)
	if !found {
		return orchestratorevents.DriveRuntimeResult{}, fmt.Errorf("Runtime has no active recoverable wait")
	}
	view, err := runtimecontract.RuntimeRecoverableWaitProjection(wait, aggregate.StateRevision)
	if err != nil {
		return orchestratorevents.DriveRuntimeResult{}, err
	}
	return orchestratorevents.DriveRuntimeResult{
		WorkspaceID:         aggregate.Facts.WorkspaceID,
		RunID:               aggregate.Facts.RunID,
		StateRevision:       aggregate.StateRevision,
		EvidenceRevision:    aggregate.Facts.EvidenceRevision,
		DefinitionRevision:  aggregate.Facts.DefinitionRevision,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
		State:               aggregate.Decision.State,
		RecoverableWait:     &view,
	}, nil
}

func runtimePendingProviderCeilingProbe(aggregate runtime.RuntimeAggregate, wait runtimecontract.RuntimeRecoverableWaitRecord) (runtimecontract.RuntimePrerequisite, llmevents.AcquireProviderCeilingCommand, error) {
	for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindProviderCeiling {
			continue
		}
		command := runtimeProviderCeilingPrerequisiteCommand(aggregate, prerequisite)
		conditionHash := runtimecontract.StableRuntimeProviderCeilingRecoveryConditionHash(aggregate.Facts.DefinitionRevision, wait.AuthorityTransactionID, prerequisite.ID)
		if runtimeProviderCeilingProbeRequestHash(command) == wait.ProbeRequestHash && conditionHash == wait.RecoveryConditionHash {
			return prerequisite, command, nil
		}
	}
	return runtimecontract.RuntimePrerequisite{}, llmevents.AcquireProviderCeilingCommand{}, fmt.Errorf("recoverable provider ceiling wait %q has no matching pending prerequisite", wait.WaitID)
}

func (m *Biz) driveRuntimeRecoverableWait(ctx context.Context, aggregate runtime.RuntimeAggregate, leaseGeneration uint64) (runtimeDriveTransitionOutcome, error) {
	wait, found := runtimeCurrentRecoverableWait(aggregate)
	if !found || leaseGeneration == 0 {
		result, err := runtimeRecoverableWaitDriveResult(aggregate)
		return runtimeDriveTransitionOutcome{aggregate: aggregate, result: &result}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	claimed, accepted, err := m.runtimeClaimRecoverableWaitProbe(ctx, aggregate, wait.WaitID, leaseGeneration, now)
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	if runtimeConvergenceTerminal(claimed.Decision.State) {
		return terminalRuntimeDriveTransition(claimed), nil
	}
	if !accepted {
		result, resultErr := runtimeRecoverableWaitDriveResult(claimed)
		return runtimeDriveTransitionOutcome{aggregate: claimed, result: &result}, resultErr
	}
	aggregate = claimed
	wait, found = runtimeCurrentRecoverableWait(aggregate)
	if !found || wait.Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		return runtimeDriveTransitionOutcome{}, fmt.Errorf("recoverable wait probe claim was not persisted")
	}
	prerequisite, command, err := runtimePendingProviderCeilingProbe(aggregate, wait)
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	if runtimeProviderCeilingProbeRequestHash(command) != wait.ProbeRequestHash {
		return runtimeDriveTransitionOutcome{}, fmt.Errorf("recoverable wait probe command identity drifted")
	}
	result, probeErr := m.acquireRuntimeProviderCeilingPrerequisite(ctx, aggregate, prerequisite)
	if probeErr != nil {
		if errors.Is(probeErr, context.Canceled) {
			return runtimeDriveTransitionOutcome{}, probeErr
		}
		if !runtimeRecoverableWaitEligibleError(probeErr) {
			exhausted, exhaustErr := m.runtimeExhaustRecoverableWait(ctx, aggregate, wait.WaitID, "recoverable_wait_probe_contract_failed")
			if exhaustErr != nil {
				return runtimeDriveTransitionOutcome{}, exhaustErr
			}
			return terminalRuntimeDriveTransition(exhausted), nil
		}
		deferred, deferErr := m.runtimeDeferRecoverableWaitProbe(ctx, aggregate, wait.WaitID, now)
		if deferErr != nil {
			return runtimeDriveTransitionOutcome{}, deferErr
		}
		if runtimeConvergenceTerminal(deferred.Decision.State) {
			return terminalRuntimeDriveTransition(deferred), nil
		}
		waitResult, resultErr := runtimeRecoverableWaitDriveResult(deferred)
		return runtimeDriveTransitionOutcome{aggregate: deferred, result: &waitResult}, resultErr
	}
	if result.Observation == nil {
		return runtimeDriveTransitionOutcome{}, fmt.Errorf("recoverable provider ceiling probe returned no observation")
	}
	probeObservation := runtimecontract.RuntimeRecoverableWaitProbeObservation{
		WaitID:           wait.WaitID,
		ProbeRequestHash: wait.ProbeRequestHash,
		OwnerResultHash:  result.Observation.ObservationID,
		ObservedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	committed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventPrerequisiteObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{PrerequisiteResult: &result, RecoverableWaitProbeObservation: &probeObservation},
	})
	if err != nil {
		return runtimeDriveTransitionOutcome{}, err
	}
	return runtimeDriveTransitionOutcome{aggregate: committed.Aggregate}, nil
}
