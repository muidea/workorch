package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// reserveCandidateRequestExecutionBudget is part of the same SM-3 CAS as the
// provider-submitted marker.  A successful return authorizes exactly one
// physical provider call; an error therefore leaves the caller with zero
// external-call authority.
func reserveCandidateRequestExecutionBudget(aggregate *RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, sequence int, physicalCallID string) error {
	if aggregate == nil || session.ExpectedDocument == nil {
		return fmt.Errorf("runtime candidate request has no expected-document budget identity")
	}
	specs, err := runtimecontract.RuntimeProviderBudgetAllocationSpecs(
		aggregate.ExecutionBudget,
		session.DocumentKind,
		session.ExpectedDocument.CorrectionAttempt,
		sequence,
		session.LogicalRequestID,
		physicalCallID,
	)
	if err != nil {
		return err
	}
	ledger, _, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, specs)
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

func applyCandidateRequestBudgetObservation(aggregate *RuntimeAggregate, observation *runtimecontract.RuntimeProviderBudgetObservation) (bool, error) {
	if aggregate == nil || observation == nil {
		return false, nil
	}
	ledger, changed, err := runtimecontract.ApplyRuntimeProviderBudgetObservation(aggregate.ExecutionBudget, observation.Clone())
	if err != nil {
		return false, err
	}
	aggregate.ExecutionBudget = ledger
	return changed, nil
}

func runtimeOperationBudgetScopeRef(executionKey string, fence uint64) string {
	return fmt.Sprintf("%s/%d", strings.TrimSpace(executionKey), fence)
}

func reserveRuntimeOperationExecutionBudget(aggregate *RuntimeAggregate, execution runtimecontract.RuntimeOperationExecutionRecord, nextFence uint64) error {
	if aggregate == nil || strings.TrimSpace(execution.ExecutionKey) == "" || nextFence == 0 {
		return fmt.Errorf("runtime operation budget identity is incomplete")
	}
	ledger, _, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(aggregate.ExecutionBudget, []runtimecontract.RuntimeBudgetAllocationSpec{{
		Dimension:            runtimecontract.RuntimeBudgetDimensionCapabilityOperation,
		LogicalConsumptionID: execution.ExecutionKey,
		ScopeRef:             runtimeOperationBudgetScopeRef(execution.ExecutionKey, nextFence),
		ReservedAmount:       1,
	}})
	if err != nil {
		return err
	}
	aggregate.ExecutionBudget = ledger
	return nil
}

func joinRuntimeOperationExecutionBudget(aggregate *RuntimeAggregate, operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, receipt *runtimecontract.RuntimeOperationReceipt, outcomeUnknown bool) (bool, error) {
	if aggregate == nil || strings.TrimSpace(execution.ExecutionKey) == "" || execution.Fence == 0 {
		return false, fmt.Errorf("runtime operation budget identity is incomplete")
	}
	scopeRef := runtimeOperationBudgetScopeRef(execution.ExecutionKey, execution.Fence)
	var allocation runtimecontract.RuntimeBudgetAllocation
	for _, candidate := range aggregate.ExecutionBudget.Allocations {
		candidate = candidate.Normalized()
		if candidate.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation && candidate.LogicalConsumptionID == execution.ExecutionKey && candidate.ScopeRef == scopeRef {
			allocation = candidate
			break
		}
	}
	if allocation.AllocationID == "" {
		return false, fmt.Errorf("runtime operation execution has no matching budget allocation")
	}
	owner := strings.TrimSpace(runtimecontract.RuntimeModuleForOperationKind(operation.Kind))
	if owner == "" {
		return false, fmt.Errorf("runtime operation budget owner is unknown")
	}
	resultRef := "runtime-receipt/" + execution.ExecutionKey
	resultHash := "runtime-operation-outcome-unknown/" + scopeRef
	confirmed, unknown := int64(1), int64(0)
	if outcomeUnknown {
		confirmed, unknown = 0, 1
	} else {
		if receipt == nil {
			return false, fmt.Errorf("runtime operation budget confirmation requires receipt")
		}
		resultHash = runtimecontract.StableRuntimeOperationReceiptHash(*receipt)
	}
	ledger, _, changed, err := runtimecontract.JoinRuntimeBudgetConsumption(aggregate.ExecutionBudget, runtimecontract.RuntimeBudgetConsumptionJoin{
		AllocationID: allocation.AllocationID, Owner: owner, PhysicalConsumptionID: scopeRef,
		ConfirmedAmount: confirmed, UnknownAmount: unknown, OwnerResultRef: resultRef, OwnerResultHash: resultHash,
	})
	if err != nil {
		return false, err
	}
	aggregate.ExecutionBudget = ledger
	return changed, nil
}
