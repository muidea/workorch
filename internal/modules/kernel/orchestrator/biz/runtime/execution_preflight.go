package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type activeDispatchExecutionFlow uint8

const (
	activeDispatchExecutionReady activeDispatchExecutionFlow = iota + 1
	activeDispatchExecutionSkip
	activeDispatchExecutionStop
	activeDispatchExecutionRetry
)

type activeDispatchExecution struct {
	component             *candidateDispatchComponent
	ctx                   context.Context
	workspaceID           common.WorkspaceID
	runID                 common.RunID
	executor              RuntimeOperationExecutor
	recoveryOperationID   string
	recoveryBudgetAttempt int
	aggregate             RuntimeAggregate
	dispatch              SealedDispatch
	operations            []runtimecontract.RuntimeOperation
	completed             map[string]struct{}
}

type preparedActiveDispatchOperation struct {
	execution       runtimecontract.RuntimeOperationExecutionRecord
	governanceGrant *runtimecontract.RuntimeOperationGovernanceGrant
	flow            activeDispatchExecutionFlow
}

func prepareActiveDispatchExecution(ctx context.Context, component *candidateDispatchComponent, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor, recoveryOperationID string, recoveryBudgetAttempt int) (*activeDispatchExecution, error) {
	if component == nil || component.repository == nil || executor == nil || workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	aggregate, err := component.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return nil, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.State != DispatchStateSealed {
		return nil, fmt.Errorf("%w: runtime has no active sealed dispatch", common.ErrConflict)
	}
	// Owner execution is downstream of the same canonical CWI fence used by
	// candidate, dispatch, receipt, and evidence admission.
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
		return nil, fmt.Errorf("%w: operation execution binding fence: %v", common.ErrConflict, err)
	}
	dispatch := *aggregate.ActiveDispatch
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		return nil, err
	}
	return &activeDispatchExecution{
		component: component, ctx: ctx, workspaceID: workspaceID, runID: runID, executor: executor,
		recoveryOperationID: recoveryOperationID, recoveryBudgetAttempt: recoveryBudgetAttempt,
		aggregate: aggregate, dispatch: dispatch, operations: operations,
		completed: stringSet(dispatch.CompletedOperationIDs),
	}, nil
}

func (x *activeDispatchExecution) run() (RuntimeAggregate, error) {
	for _, operation := range x.operations {
		if _, ok := x.completed[operation.ID]; ok {
			continue
		}
		if err := requireCompletedOperationDependencies(operation, x.completed); err != nil {
			return RuntimeAggregate{}, err
		}
		prepared, err := x.prepareOperation(operation)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		switch prepared.flow {
		case activeDispatchExecutionSkip:
			continue
		case activeDispatchExecutionStop:
			return x.aggregate, nil
		}
		ownerCall, err := x.callOperationOwner(operation, prepared)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		switch ownerCall.flow {
		case activeDispatchExecutionStop:
			return x.aggregate, nil
		case activeDispatchExecutionRetry:
			return x.component.executeActiveDispatch(x.ctx, x.workspaceID, x.runID, x.executor, "", 0)
		}
		flow, err := x.projectOperationReceipt(operation, prepared.execution, ownerCall.result)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		if flow == activeDispatchExecutionStop {
			return x.aggregate, nil
		}
	}
	return x.aggregate, nil
}

func requireCompletedOperationDependencies(operation runtimecontract.RuntimeOperation, completed map[string]struct{}) error {
	for _, dependency := range operation.DependsOn {
		if _, ok := completed[dependency]; !ok {
			return fmt.Errorf("operation %q dependency %q is not committed", operation.ID, dependency)
		}
	}
	return nil
}
