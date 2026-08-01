package orchestrator

// Operation receipt adapter is internal to the Runtime owner Biz.

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimeOperationMagicFailure(err error) *cd.Error {
	if err == nil {
		return nil
	}
	return &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}
}

func (m *Biz) handleRuntimeOperationReceipt(event magicevent.Event, result magicevent.Result) {
	if m == nil || result == nil {
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
	if m.bizPtr == nil {
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
		receipt, found, err := m.bizPtr.QueryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, command.GovernanceGrant)
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
		committed, err := m.submitRuntimeInternalEvent(event.Context(), runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventOperationReceiptObserved, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			DispatchID: command.DispatchID, Fence: command.Fence, OperationReceipt: &command.Receipt,
		})
		if err != nil {
			result.Set(nil, runtimeOperationMagicFailure(err))
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
