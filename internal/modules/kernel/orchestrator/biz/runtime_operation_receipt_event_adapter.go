package orchestrator

// Operation receipt adapter is internal to the Runtime owner Biz.

import (
	"context"
	"errors"
	"reflect"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeOperationMagicFailure(err error) *cd.Error {
	if err == nil {
		return nil
	}
	return &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}
}

func (m *Biz) handleRuntimeOperationReceipt(event magicevent.Event, result magicevent.Result) {
	if m == nil || event == nil || result == nil {
		if result != nil {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
		}
		return
	}
	// Query/commit receipt events may have been dequeued before Stop removed its
	// subscription. They share the same Runtime owner dependencies as drive
	// and therefore must finish or reject before Stop clears those pointers.
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil {
		result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
		return
	}
	switch event.ID() {
	case orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt:
		command, ok := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if !ok {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
			return
		}
		var receipt runtimecontract.RuntimeOperationReceipt
		var found bool
		err := m.withOrchestratorWorkspaceResourceRead(event.Context(), command.WorkspaceID, func(guarded context.Context) error {
			var queryErr error
			if command.ReadSource != nil {
				source := command.ReadSource
				if source.WorkspaceID != command.WorkspaceID || source.RunID != command.RunID || source.DispatchID != command.DispatchID || source.OperationID != command.OperationID ||
					source.Execution.ExecutionKey != command.ExecutionKey || source.Execution.Fence != command.Fence || !reflect.DeepEqual(source.Execution.GovernanceGrant, command.GovernanceGrant) {
					return common.ErrInvalidArgument
				}
				receipt, found, queryErr = m.runtimeController.QueryOperationReceipt(guarded, command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, event.Source(), command.GovernanceGrant, *source)
				return queryErr
			}
			receipt, found, queryErr = m.runtimeController.QueryOperationReceipt(guarded, command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, event.Source(), command.GovernanceGrant)
			return queryErr
		})
		if err != nil {
			result.Set(nil, runtimeOperationMagicFailure(err))
			return
		}
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			value := receipt.Clone()
			response.Receipt = &value
		}
		result.Set(response, nil)
	case orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt:
		command, ok := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if !ok {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
			return
		}
		if event.Source() == "" || event.Source() != strings.TrimSpace(event.Source()) {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
			return
		}
		// The first read gate hides an already deleted Workspace. It is released
		// before the canonical Runtime dispatcher acquires the same mutation
		// fence: a concurrent deletion either waits for the receipt CAS or wins
		// the write fence and makes the following submission fail closed.
		err := m.withOrchestratorWorkspaceResourceRead(event.Context(), command.WorkspaceID, func(context.Context) error { return nil })
		if err != nil {
			var cas *runtime.RuntimeCASConflictError
			var transient *runtime.RuntimeOwnerTransientError
			result.Set(nil, runtimeOperationMagicFailure(&runtimecontract.RuntimeReceiptDeliveryError{Retryable: errors.As(err, &cas) || errors.As(err, &transient), Cause: err}))
			return
		}
		committed, err := m.submitRuntimeInternalEvent(event.Context(), runtime.RuntimeInternalEvent{
			Kind:  runtime.RuntimeInternalEventOperationReceiptObserved,
			Scope: runtime.RuntimeInternalEventScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID},
			Transaction: &runtime.RuntimeInternalTransactionPayload{
				DispatchID: command.DispatchID, Fence: command.Fence, OperationReceiptOwnerSource: event.Source(), OperationReceipt: &command.Receipt,
			},
		})
		if err != nil {
			var cas *runtime.RuntimeCASConflictError
			var transient *runtime.RuntimeOwnerTransientError
			result.Set(nil, runtimeOperationMagicFailure(&runtimecontract.RuntimeReceiptDeliveryError{Retryable: errors.As(err, &cas) || errors.As(err, &transient), Cause: err}))
			return
		}
		if committed.Receipt == nil {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: *committed.Receipt, StateRevision: committed.Aggregate.StateRevision}, nil)
	default:
		result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
	}
}
