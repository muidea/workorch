package orchestrator

// Operation receipt adapter is internal to the Runtime V2 owner Biz.

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimeOperationMagicFailure(err error) *cd.Error {
	if err == nil {
		return nil
	}
	return &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}
}

func (m *Biz) handleRuntimeV2OperationReceipt(event magicevent.Event, result magicevent.Result) {
	if m == nil || m.bizPtr == nil || result == nil {
		if result != nil {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
		}
		return
	}
	switch event.ID() {
	case orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt:
		command, ok := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if !ok {
			result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
			return
		}
		receipt, found, err := m.bizPtr.QueryRuntimeV2OperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, command.GovernanceGrant)
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
		aggregate, receipt, err := m.bizPtr.CommitRuntimeV2OperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, command.Receipt)
		if err != nil {
			result.Set(nil, runtimeOperationMagicFailure(err))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: aggregate.StateRevision}, nil)
	default:
		result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
	}
}
