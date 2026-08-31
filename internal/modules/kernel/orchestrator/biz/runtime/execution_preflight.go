package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type activeDispatchExecutionFlow uint8

const (
	activeDispatchExecutionReady activeDispatchExecutionFlow = iota + 1
	activeDispatchExecutionSkip
	activeDispatchExecutionStop
)

type activeDispatchExecution struct {
	component             *candidateDispatchComponent
	ctx                   context.Context
	workspaceID           common.WorkspaceID
	runID                 common.RunID
	recoveryAuthorization *runtimecontract.RuntimeOperationRecoveryAuthorization
	recoveryCallToken     string
	recoveryNow           string
	aggregate             RuntimeAggregate
	dispatch              SealedDispatch
	operations            []runtimecontract.RuntimeOperation
	completed             map[string]struct{}
}

// RuntimeOperationExecutionCallHandoff is the immutable output of the one
// SM-3 preparation transition. Biz may execute this request through the typed
// EventHub only after the claim/fence (and, for replay, the exact SM-4 claim)
// is durable. It carries no repository or callback authority.
type RuntimeOperationExecutionCallHandoff struct {
	Request                 RuntimeOperationExecutionRequest
	RecoveryAuthorizationID string
	RecoveryClaimID         string
}

func (h RuntimeOperationExecutionCallHandoff) Validate() error {
	request := h.Request
	if request.WorkspaceID == "" || request.RunID == "" || request.DispatchID == "" || request.Operation.ID == "" ||
		request.Execution.OperationID != request.Operation.ID || request.Execution.ExecutionKey == "" || request.Execution.Fence == 0 {
		return common.ErrInvalidArgument
	}
	if h.RecoveryClaimID != strings.TrimSpace(h.RecoveryClaimID) || h.RecoveryAuthorizationID != strings.TrimSpace(h.RecoveryAuthorizationID) ||
		(h.RecoveryClaimID == "") != (h.RecoveryAuthorizationID == "") {
		return common.ErrInvalidArgument
	}
	return nil
}

// RuntimeDispatchExecutionHandoff is the bounded, read-only result of one
// sealed-operation call. The execution event itself owns only SM-3 claim/call
// coordination; the exact persisted receipt is promoted by one subsequent
// owner-private fact event with its complete coordinated write-set.
type RuntimeDispatchExecutionHandoff struct {
	DispatchID               string
	OperationID              string
	Evidence                 *ExecutionEvidence
	PrerequisiteStaleReceipt *runtimecontract.RuntimeOperationReceipt
	GovernanceRequired       *RuntimeOperationExecutionResult
}

func (h RuntimeDispatchExecutionHandoff) Validate() error {
	if h.DispatchID == "" || h.OperationID == "" {
		return common.ErrInvalidArgument
	}
	count := 0
	if h.Evidence != nil {
		count++
		if h.Evidence.DispatchID != h.DispatchID || h.Evidence.OperationID != h.OperationID || h.Evidence.Validate() != nil {
			return common.ErrInvalidArgument
		}
	}
	if h.PrerequisiteStaleReceipt != nil {
		count++
		receipt := h.PrerequisiteStaleReceipt.Normalized()
		if receipt.OperationID != h.OperationID || receipt.PrerequisiteStale == nil || receipt.Validate() != nil {
			return common.ErrInvalidArgument
		}
	}
	if h.GovernanceRequired != nil {
		count++
		result := h.GovernanceRequired
		if result.OperationID != h.OperationID || result.Governance == nil || result.Receipt.OperationID != h.OperationID ||
			result.Governance.Validate() != nil || result.Receipt.Validate() != nil {
			return common.ErrInvalidArgument
		}
	}
	if count != 1 {
		return common.ErrInvalidArgument
	}
	return nil
}

type preparedActiveDispatchOperation struct {
	execution       runtimecontract.RuntimeOperationExecutionRecord
	governanceGrant *runtimecontract.RuntimeOperationGovernanceGrant
	handoff         *RuntimeDispatchExecutionHandoff
	flow            activeDispatchExecutionFlow
}

func prepareActiveDispatchExecution(ctx context.Context, component *candidateDispatchComponent, workspaceID common.WorkspaceID, runID common.RunID, recoveryClaimID, recoveryNow string) (*activeDispatchExecution, error) {
	if component == nil || component.repository == nil || workspaceID == "" || runID == "" {
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
		component: component, ctx: ctx, workspaceID: workspaceID, runID: runID,
		recoveryCallToken: recoveryClaimID, recoveryNow: strings.TrimSpace(recoveryNow),
		aggregate: aggregate, dispatch: dispatch, operations: operations,
		completed: stringSet(dispatch.CompletedOperationIDs),
	}, nil
}

func (x *activeDispatchExecution) run() (RuntimeAggregate, *RuntimeOperationExecutionCallHandoff, *RuntimeDispatchExecutionHandoff, error) {
	for _, operation := range x.operations {
		if _, ok := x.completed[operation.ID]; ok {
			continue
		}
		if err := requireCompletedOperationDependencies(operation, x.completed); err != nil {
			return RuntimeAggregate{}, nil, nil, err
		}
		prepared, err := x.prepareOperation(operation)
		if err != nil {
			return RuntimeAggregate{}, nil, nil, err
		}
		if prepared.handoff != nil {
			return x.aggregate, nil, prepared.handoff, nil
		}
		switch prepared.flow {
		case activeDispatchExecutionSkip:
			continue
		case activeDispatchExecutionStop:
			return x.aggregate, nil, nil, nil
		}
		call, err := x.operationExecutionCallHandoff(operation, prepared)
		if err != nil {
			return RuntimeAggregate{}, nil, nil, err
		}
		return x.aggregate, call, nil, nil
	}
	return x.aggregate, nil, nil, nil
}

func requireCompletedOperationDependencies(operation runtimecontract.RuntimeOperation, completed map[string]struct{}) error {
	for _, dependency := range operation.DependsOn {
		if _, ok := completed[dependency]; !ok {
			return fmt.Errorf("operation %q dependency %q is not committed", operation.ID, dependency)
		}
	}
	return nil
}
