package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func currentRuntimeRecoverableWait(values []runtimecontract.RuntimeRecoverableWaitRecord) (int, runtimecontract.RuntimeRecoverableWaitRecord, bool) {
	for idx, raw := range values {
		wait := raw.Clone()
		if wait.Status == runtimecontract.RuntimeRecoverableWaitWaiting || wait.Status == runtimecontract.RuntimeRecoverableWaitProbeClaimed {
			return idx, wait, true
		}
	}
	return 0, runtimecontract.RuntimeRecoverableWaitRecord{}, false
}

func runtimeRecoverableWaitPendingRetirement(values []runtimecontract.RuntimeRecoverableWaitRecord) (int, runtimecontract.RuntimeRecoverableWaitRecord, bool) {
	for idx, raw := range values {
		wait := raw.Clone()
		if wait.Status == runtimecontract.RuntimeRecoverableWaitConditionSatisfied {
			return idx, wait, true
		}
	}
	return 0, runtimecontract.RuntimeRecoverableWaitRecord{}, false
}

func (c *driveDeliveryComponent) openRecoverableWait(ctx context.Context, record runtimecontract.RuntimeRecoverableWaitRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || record.WorkspaceID == "" || record.RunID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, record.WorkspaceID, record.RunID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	record = record.Clone()
	for _, existing := range aggregate.RecoverableWaits {
		if existing.WaitID == record.WaitID {
			if existing.Clone() != record {
				return RuntimeAggregate{}, fmt.Errorf("%w: runtime recoverable wait identity conflicts", common.ErrConflict)
			}
			return aggregate, nil
		}
	}
	if _, active, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits); found {
		return RuntimeAggregate{}, fmt.Errorf("%w: active runtime recoverable wait %q already exists", common.ErrConflict, active.WaitID)
	}
	if record.StateRevisionOpened != aggregate.StateRevision+1 || record.DefinitionRevision != aggregate.Facts.DefinitionRevision ||
		record.AuthorityTransactionID != runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction()) ||
		record.Status != runtimecontract.RuntimeRecoverableWaitWaiting || record.Attempt != 0 {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime recoverable wait does not match current authority", common.ErrConflict)
	}
	ledger, allocations, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension: record.BudgetDimension, LogicalConsumptionID: record.WaitID, ScopeRef: record.ProbeRequestHash, ReservedAmount: 1,
	}})
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if len(allocations) != 1 || allocations[0].AllocationID != record.BudgetAllocationID {
		return RuntimeAggregate{}, fmt.Errorf("runtime recoverable wait budget allocation identity mismatch")
	}
	aggregate.ExecutionBudget = ledger
	if err := record.Validate(aggregate.ExecutionBudget); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.RecoverableWaits = append(aggregate.RecoverableWaits, record)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *driveDeliveryComponent) claimRecoverableWaitProbe(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, waitID string, leaseGeneration uint64, now string) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(waitID) == "" || leaseGeneration == 0 {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	idx, wait, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits)
	if !found || wait.WaitID != strings.TrimSpace(waitID) {
		return aggregate, false, nil
	}
	if aggregate.DriveLease == nil || aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning || aggregate.DriveLease.RecoveryGeneration != leaseGeneration ||
		aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonRecoverableWait || aggregate.DriveLease.FailureSignature != "recoverable-wait/"+wait.WaitID {
		return aggregate, false, nil
	}
	nowValue, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(now))
	nextValue, nextErr := time.Parse(time.RFC3339Nano, wait.NextEligibleAt)
	deadlineValue, deadlineErr := time.Parse(time.RFC3339Nano, wait.Deadline)
	if parseErr != nil || nextErr != nil || deadlineErr != nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	if nowValue.Before(nextValue) {
		return aggregate, false, nil
	}
	if !nowValue.Before(deadlineValue) {
		return c.expireRecoverableWait(ctx, aggregate, idx, wait, "recoverable_wait_deadline_exceeded")
	}
	if wait.Status == runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		return aggregate, true, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitProbeClaimed
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func (c *driveDeliveryComponent) deferRecoverableWaitProbe(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, waitID, now, probeResultHash string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, wait, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits)
	if !found || wait.WaitID != strings.TrimSpace(waitID) || wait.Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime recoverable wait probe is not claimed", common.ErrConflict)
	}
	if strings.TrimSpace(probeResultHash) != runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := markRecoverableWaitProbeUnknown(&aggregate, wait, probeResultHash); err != nil {
		return RuntimeAggregate{}, err
	}
	nowValue, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(now))
	deadlineValue, deadlineErr := time.Parse(time.RFC3339Nano, wait.Deadline)
	if parseErr != nil || deadlineErr != nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if !nowValue.Before(deadlineValue) {
		updated, _, expireErr := c.expireRecoverableWait(ctx, aggregate, idx, wait, "recoverable_wait_deadline_exceeded")
		return updated, expireErr
	}
	next := nowValue.Add(2 * time.Second)
	if next.After(deadlineValue) {
		next = deadlineValue
	}
	expectedRevision := aggregate.StateRevision
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitWaiting
	aggregate.RecoverableWaits[idx].NextEligibleAt = next.UTC().Format(time.RFC3339Nano)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *driveDeliveryComponent) exhaustRecoverableWait(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, waitID, probeResultHash, code string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, wait, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits)
	if !found || wait.WaitID != strings.TrimSpace(waitID) || wait.Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime recoverable wait probe is not claimed", common.ErrConflict)
	}
	if strings.TrimSpace(probeResultHash) != runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := markRecoverableWaitProbeUnknown(&aggregate, wait, probeResultHash); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := failRecoverableWaitBudgetReconciliation(&aggregate, wait, probeResultHash); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExhausted
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRecoverableWaitExhausted, Code: strings.TrimSpace(code),
		Signature: "recoverable-wait-exhausted/" + wait.WaitID, Message: "recoverable Runtime wait probe failed its sealed contract",
	}
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return RuntimeAggregate{}, err
	}
	facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if reduceErr != nil {
		return RuntimeAggregate{}, reduceErr
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *driveDeliveryComponent) retireRecoverableWait(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, waitID string) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, wait, found := runtimeRecoverableWaitPendingRetirement(aggregate.RecoverableWaits)
	if !found || wait.WaitID != strings.TrimSpace(waitID) {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitRetired
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *driveDeliveryComponent) expireRecoverableWait(ctx context.Context, aggregate RuntimeAggregate, idx int, wait runtimecontract.RuntimeRecoverableWaitRecord, code string) (RuntimeAggregate, bool, error) {
	expectedRevision := aggregate.StateRevision
	if allocation, found := runtimecontract.RuntimeBudgetAllocationForID(aggregate.ExecutionBudget.Allocations, wait.BudgetAllocationID); found && allocation.State == runtimecontract.RuntimeBudgetAllocationReserved {
		ledger, _, releaseErr := runtimecontract.ReleaseRuntimeBudgetAllocation(aggregate.ExecutionBudget, wait.BudgetAllocationID, "recoverable-wait-expired/"+wait.WaitID)
		if releaseErr != nil {
			return RuntimeAggregate{}, false, releaseErr
		}
		aggregate.ExecutionBudget = ledger
	} else if found && allocation.State == runtimecontract.RuntimeBudgetAllocationUnknown {
		if err := failRecoverableWaitBudgetReconciliation(&aggregate, wait, runtimecontract.StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash)); err != nil {
			return RuntimeAggregate{}, false, err
		}
	}
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExpired
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRecoverableWaitExpired, Code: strings.TrimSpace(code),
		Signature: "recoverable-wait-expired/" + wait.WaitID, Message: "recoverable Runtime wait exceeded its sealed deadline",
	}
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return RuntimeAggregate{}, false, err
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, false, nil
}

func markRecoverableWaitProbeUnknown(aggregate *RuntimeAggregate, wait runtimecontract.RuntimeRecoverableWaitRecord, probeResultHash string) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	allocation, found := runtimecontract.RuntimeBudgetAllocationForID(aggregate.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if !found {
		return common.ErrNotFound
	}
	ledger, _, _, err := runtimecontract.JoinRuntimeBudgetConsumption(aggregate.ExecutionBudget, runtimecontract.RuntimeBudgetConsumptionJoin{
		AllocationID: wait.BudgetAllocationID, Owner: wait.ProbeOwner, PhysicalConsumptionID: wait.ProbeRequestHash,
		UnknownAmount: 1, OwnerResultRef: "recoverable-wait-unknown/" + wait.WaitID, OwnerResultHash: strings.TrimSpace(probeResultHash),
	})
	if err != nil {
		return err
	}
	reconciliation, err := runtimeRecoverableWaitBudgetReconciliation(ledger, wait)
	if err != nil {
		return err
	}
	if allocation.State == runtimecontract.RuntimeBudgetAllocationReserved {
		ledger, _, err = runtimecontract.OpenRuntimeBudgetReconciliation(ledger, reconciliation)
	} else {
		ledger, _, err = runtimecontract.MarkRuntimeBudgetReconciliationStillUnknown(ledger, reconciliation.ReconciliationID, probeResultHash)
	}
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

func runtimeRecoverableWaitBudgetReconciliation(ledger runtimecontract.RuntimeExecutionBudgetLedger, wait runtimecontract.RuntimeRecoverableWaitRecord) (runtimecontract.RuntimeBudgetReconciliationRecord, error) {
	return runtimecontract.NewRuntimeBudgetReconciliationRecord(
		ledger,
		wait.BudgetAllocationID,
		string(wait.ProbeKind),
		wait.ProbeOwner,
		wait.ProbeRequestHash,
		wait.RecoveryConditionHash,
	)
}

func failRecoverableWaitBudgetReconciliation(aggregate *RuntimeAggregate, wait runtimecontract.RuntimeRecoverableWaitRecord, ownerResultHash string) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	reconciliation, err := runtimeRecoverableWaitBudgetReconciliation(aggregate.ExecutionBudget, wait)
	if err != nil {
		return err
	}
	ledger, _, err := runtimecontract.FailRuntimeBudgetReconciliation(aggregate.ExecutionBudget, reconciliation.ReconciliationID, ownerResultHash)
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

// confirmRecoverableWaitProbe joins the exact owner result and reports whether
// the result arrived at or after the sealed deadline. A late result still
// confirms that the physical probe occurred, but it cannot satisfy the wait or
// authorize the prerequisite transition.
func confirmRecoverableWaitProbe(aggregate *RuntimeAggregate, observation runtimecontract.RuntimeRecoverableWaitProbeObservation) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return false, err
	}
	idx, wait, found := currentRuntimeRecoverableWait(aggregate.RecoverableWaits)
	if !found || wait.Status != runtimecontract.RuntimeRecoverableWaitProbeClaimed || wait.WaitID != observation.WaitID || wait.ProbeRequestHash != observation.ProbeRequestHash {
		return false, fmt.Errorf("%w: recoverable wait probe observation does not match claimed wait", common.ErrConflict)
	}
	observedAt, observedErr := time.Parse(time.RFC3339Nano, observation.ObservedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, wait.Deadline)
	if observedErr != nil || deadlineErr != nil {
		return false, common.ErrInvalidArgument
	}
	join := runtimecontract.RuntimeBudgetConsumptionJoin{
		AllocationID: wait.BudgetAllocationID, Owner: wait.ProbeOwner, PhysicalConsumptionID: wait.ProbeRequestHash,
		ConfirmedAmount: 1, OwnerResultRef: "recoverable-wait-result/" + wait.WaitID, OwnerResultHash: observation.OwnerResultHash,
	}
	allocation, found := runtimecontract.RuntimeBudgetAllocationForID(aggregate.ExecutionBudget.Allocations, wait.BudgetAllocationID)
	if !found {
		return false, fmt.Errorf("recoverable wait probe allocation is missing")
	}
	var ledger runtimecontract.RuntimeExecutionBudgetLedger
	var changed bool
	var err error
	if allocation.State == runtimecontract.RuntimeBudgetAllocationUnknown {
		reconciliation, reconciliationErr := runtimeRecoverableWaitBudgetReconciliation(aggregate.ExecutionBudget, wait)
		if reconciliationErr != nil {
			return false, reconciliationErr
		}
		ledger, _, changed, err = runtimecontract.ConfirmRuntimeBudgetReconciliation(aggregate.ExecutionBudget, reconciliation.ReconciliationID, join)
	} else {
		ledger, _, changed, err = runtimecontract.JoinRuntimeBudgetConsumption(aggregate.ExecutionBudget, join)
	}
	if err != nil {
		return false, err
	}
	if !changed {
		return false, fmt.Errorf("%w: recoverable wait probe result was already consumed", common.ErrConflict)
	}
	aggregate.ExecutionBudget = ledger
	aggregate.RecoverableWaits[idx].Attempt++
	aggregate.RecoverableWaits[idx].LastProbeResultHash = observation.OwnerResultHash
	if !observedAt.Before(deadline) {
		aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitExpired
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRecoverableWaitExpired, Code: "recoverable_wait_deadline_exceeded",
			Signature: "recoverable-wait-expired/" + wait.WaitID, Message: "recoverable Runtime wait result arrived after its sealed deadline",
		}
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return false, err
		}
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
		if reduceErr != nil {
			return false, reduceErr
		}
		aggregate.Facts, aggregate.Decision = facts, decision
		return true, nil
	}
	aggregate.RecoverableWaits[idx].Status = runtimecontract.RuntimeRecoverableWaitConditionSatisfied
	return false, nil
}
