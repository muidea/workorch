package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"strings"
)

// Frozen pre-change traversal, independent of the call-local lookup.
func referenceBudgetBeforeLookupValidate(l RuntimeExecutionBudgetLedger) error {
	l = l.Clone()
	if err := l.Policy.Validate(); err != nil {
		return err
	}
	allocations := map[string]RuntimeBudgetAllocation{}
	for _, allocation := range l.Allocations {
		if runtimeBudgetDimensionUsesSourceLedger(allocation.Dimension) {
			return fmt.Errorf("runtime budget dimension %q must be derived from its authoritative source ledger", allocation.Dimension)
		}
		if err := allocation.Validate(l.Policy); err != nil {
			return err
		}
		if _, duplicate := allocations[allocation.AllocationID]; duplicate {
			return fmt.Errorf("runtime budget allocation %q is duplicated", allocation.AllocationID)
		}
		allocations[allocation.AllocationID] = allocation
	}
	consumptionsByAllocation := map[string]int{}
	physicalOwners := map[string]string{}
	seenConsumptions := map[string]struct{}{}
	for _, consumption := range l.Consumptions {
		allocation, found := allocations[consumption.AllocationID]
		if !found {
			return fmt.Errorf("runtime budget consumption has no allocation")
		}
		if err := consumption.Validate(allocation); err != nil {
			return err
		}
		if _, duplicate := seenConsumptions[consumption.ConsumptionID]; duplicate {
			return fmt.Errorf("runtime budget consumption %q is duplicated", consumption.ConsumptionID)
		}
		seenConsumptions[consumption.ConsumptionID] = struct{}{}
		consumptionsByAllocation[consumption.AllocationID]++
		physicalKey := consumption.Owner + "\x00" + consumption.PhysicalConsumptionID + "\x00" + string(allocation.Dimension)
		if other, duplicate := physicalOwners[physicalKey]; duplicate && other != consumption.AllocationID {
			return fmt.Errorf("runtime physical consumption belongs to multiple allocations")
		}
		physicalOwners[physicalKey] = consumption.AllocationID
	}
	for _, allocation := range l.Allocations {
		count := consumptionsByAllocation[allocation.AllocationID]
		switch allocation.State {
		case RuntimeBudgetAllocationReserved, RuntimeBudgetAllocationReleased:
			if count != 0 {
				return fmt.Errorf("runtime budget allocation state %q cannot carry consumption", allocation.State)
			}
		case RuntimeBudgetAllocationConfirmed:
			if count != 1 {
				return fmt.Errorf("confirmed runtime budget allocation requires one consumption")
			}
			fact := runtimeBudgetConsumptionForAllocation(l.Consumptions, allocation.AllocationID)
			if fact.UnknownAmount != 0 || allocation.OwnerFactRef != fact.OwnerResultRef {
				return fmt.Errorf("confirmed runtime budget allocation carries unknown consumption")
			}
		case RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
			if count != 1 {
				return fmt.Errorf("unknown or exhausted runtime budget allocation requires one consumption")
			}
			fact := runtimeBudgetConsumptionForAllocation(l.Consumptions, allocation.AllocationID)
			if fact.UnknownAmount <= 0 || fact.ConfirmedAmount != 0 || allocation.OwnerFactRef != fact.OwnerResultRef {
				return fmt.Errorf("unknown or exhausted runtime budget allocation has invalid consumption")
			}
		}
	}
	seenReconciliations := map[string]struct{}{}
	for _, record := range l.Reconciliations {
		if err := record.Validate(l.Allocations); err != nil {
			return err
		}
		if _, duplicate := seenReconciliations[record.ReconciliationID]; duplicate {
			return fmt.Errorf("runtime budget reconciliation %q is duplicated", record.ReconciliationID)
		}
		seenReconciliations[record.ReconciliationID] = struct{}{}
	}
	_, err := referenceBudgetBeforeLookupView("workspace-validation", "run-validation", 1, l, "", "")
	return err
}

func referenceBudgetBeforeLookupView(workspaceID common.WorkspaceID, runID common.RunID, stateRevision uint64, ledger RuntimeExecutionBudgetLedger, activeWaitID, terminalReasonCode string, sourceUsage ...RuntimeExecutionBudgetSourceUsage) (RuntimeExecutionBudgetView, error) {
	ledger = ledger.Clone()
	if workspaceID == "" || runID == "" || stateRevision == 0 || ledger.Policy.PolicyRevision == "" {
		return RuntimeExecutionBudgetView{}, common.ErrInvalidArgument
	}
	if len(sourceUsage) > 1 {
		return RuntimeExecutionBudgetView{}, common.ErrInvalidArgument
	}
	usage := RuntimeExecutionBudgetSourceUsage{}
	if len(sourceUsage) == 1 {
		usage = sourceUsage[0]
		if err := usage.Validate(ledger.Policy); err != nil {
			return RuntimeExecutionBudgetView{}, err
		}
	}
	view := RuntimeExecutionBudgetView{WorkspaceID: workspaceID, RunID: runID, PolicyRevision: ledger.Policy.PolicyRevision, StateRevision: stateRevision, ActiveWaitID: strings.TrimSpace(activeWaitID), TerminalReasonCode: strings.TrimSpace(terminalReasonCode)}
	for _, dimension := range ledger.Policy.Dimensions {
		item := RuntimeExecutionBudgetDimensionView{Dimension: dimension.Dimension, Unit: dimension.Unit, Limit: dimension.Limit, Confirmed: usage.confirmed(dimension.Dimension)}
		for _, allocation := range ledger.Allocations {
			if allocation.Dimension != dimension.Dimension {
				continue
			}
			switch allocation.State {
			case RuntimeBudgetAllocationReserved:
				item.Reserved += allocation.ReservedAmount
			case RuntimeBudgetAllocationConfirmed:
				item.Confirmed += runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID).ConfirmedAmount
			case RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
				item.Unknown += runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID).UnknownAmount
			}
		}
		used := item.Confirmed + item.Reserved + item.Unknown
		if used < 0 || used > item.Limit {
			return RuntimeExecutionBudgetView{}, fmt.Errorf("runtime execution budget view exceeds dimension %q", item.Dimension)
		}
		item.Remaining = item.Limit - used
		item.Exhausted = item.Remaining == 0
		view.Dimensions = append(view.Dimensions, item)
	}
	view.ViewHash = StableRuntimeExecutionBudgetViewHash(view)
	return view, nil
}

func referenceBudgetBeforeLookupRemaining(ledger RuntimeExecutionBudgetLedger, dimension RuntimeBudgetDimension) (int64, error) {
	dimensionPolicy, found := RuntimeBudgetDimensionPolicyFor(ledger.Policy, dimension)
	if !found {
		return 0, common.ErrInvalidArgument
	}
	var unavailable int64
	for _, allocation := range ledger.Allocations {
		allocation = allocation.Normalized()
		if allocation.Dimension != dimension {
			continue
		}
		switch allocation.State {
		case RuntimeBudgetAllocationReserved:
			unavailable += allocation.ReservedAmount
		case RuntimeBudgetAllocationConfirmed, RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
			fact := runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID)
			unavailable += fact.ConfirmedAmount + fact.UnknownAmount
		}
		if unavailable > dimensionPolicy.Limit {
			return 0, fmt.Errorf("runtime budget dimension %q exceeds its limit", dimension)
		}
	}
	return dimensionPolicy.Limit - unavailable, nil
}
