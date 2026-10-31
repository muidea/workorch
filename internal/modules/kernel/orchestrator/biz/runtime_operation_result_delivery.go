package orchestrator

import (
	"context"
	"errors"
	"fmt"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Adapter routing stays outside Runtime core. This command cannot execute or
// regenerate an operation: the File owner may deliver only saved original bytes.
func (m *Biz) deliverRuntimeOperationResult(ctx context.Context, a runtime.RuntimeAggregate, id string) error {
	if a.ActiveDispatch == nil || a.ActiveDispatch.Candidate.StateChange == nil {
		return nil
	}
	var supported bool
	for _, entry := range a.ActiveDispatch.Candidate.StateChange.Operations {
		if entry.Operation.ID == id {
			switch entry.Operation.Kind {
			case contract.StepKindFileWrite, contract.StepKindFileEdit, contract.RuntimeBlockKindPatchApply:
				supported = true
			}
		}
	}
	if !supported {
		return nil
	}
	request, found, err := runtime.CompileOperationResultDeliveryRequest(a, id)
	if err != nil || !found {
		return err
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, a.Facts.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace.InternalDirectory == "" {
		return nil
	}
	sealed, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return err
	}
	cmd := fileevents.DeliverMutationResultCommand{RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, workspace.DeliverableDirectory, workspace.InternalDirectory), SealedOperation: sealed}
	event := runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicDeliverMutationResult, contract.RuntimeModuleFile, request, cmd)
	result := m.EventHub().Send(event)
	if result == nil {
		return fmt.Errorf("File result delivery returned no result")
	}
	data, magicErr := result.Get()
	if err = runtimeOperationMagicError(magicErr); err != nil {
		return err
	}
	if _, ok := data.(fileevents.DeliverMutationResultResult); !ok {
		return fmt.Errorf("File result delivery returned invalid response")
	}
	return nil
}
func operationResultDiagnostic(a runtime.RuntimeAggregate, id string, err error) *contract.RuntimeOriginalDiagnostic {
	var failure *contract.RuntimeReceiptDeliveryError
	if !errors.As(err, &failure) || failure.Diagnostic == nil || a.ActiveDispatch == nil {
		return nil
	}
	for _, entry := range a.ActiveDispatch.OperationExecutions {
		if entry.OperationID == id && entry.ExecutionKey == failure.ExecutionKey && entry.Fence == failure.Fence && failure.Diagnostic.OperationID == id && failure.Diagnostic.Validate() == nil {
			return failure.Diagnostic.CloneOriginal()
		}
	}
	return nil
}
