package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeV2OperationExecutor struct {
	hub magicevent.Hub
}

func (e eventRuntimeV2OperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	if e.hub == nil || request.WorkspaceID == "" || request.RunID == "" || request.DispatchID == "" {
		return runtimev2.RuntimeOperationExecutionResult{}, common.ErrInvalidArgument
	}
	destination := runtimecontract.RuntimeModuleForOperationKind(request.Operation.Kind)
	if destination == "" {
		return runtimev2.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime operation kind %q has no owner module", request.Operation.Kind)
	}
	workspace, err := (workspaceEventRepository{hub: e.hub}).Get(ctx, request.WorkspaceID)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	switch runtimecontract.CanonicalRuntimeBlockKind(request.Operation.Kind) {
	case runtimecontract.StepKindFileEdit:
		return e.executeFileEdit(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.RuntimeBlockKindPatchApply:
		return e.executePatchApply(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteRuntimeOperation,
		ModuleID,
		destination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.ExecuteRuntimeOperationCommand{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID,
			Transaction: request.Transaction, Operation: request.Operation,
			EvidenceKind:                  request.EvidenceKind,
			EvidencePolicies:              request.EvidencePolicies,
			WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
			WorkspaceInternalDirectory:    workspace.InternalDirectory,
		},
	)
	result := e.hub.Send(event)
	if result == nil {
		return runtimev2.RuntimeOperationExecutionResult{}, common.ErrInvalidArgument
	}
	if magicErr := result.Error(); magicErr != nil {
		if magicErr.Cause != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, magicErr.Cause
		}
		return runtimev2.RuntimeOperationExecutionResult{}, magicErr
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		if magicErr.Cause != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, magicErr.Cause
		}
		return runtimev2.RuntimeOperationExecutionResult{}, magicErr
	}
	response, ok := data.(orchestratorevents.ExecuteRuntimeOperationResult)
	if !ok {
		return runtimev2.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime operation owner returned invalid result type %T", data)
	}
	return runtimev2.RuntimeOperationExecutionResult{
		EventID: response.EventID, OperationID: response.OperationID, EvidenceKind: response.EvidenceKind,
		Status: response.Status, SourceRef: response.SourceRef, Failure: response.Failure, Evidence: response.Evidence,
	}, nil
}

func (e eventRuntimeV2OperationExecutor) executeFileEdit(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileEditOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileEditOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	responseData, err := runtimeOperationEventResult(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	response, ok := responseData.(orchestratorevents.ExecuteFileEditOperationResult)
	if !ok {
		return runtimev2.RuntimeOperationExecutionResult{}, fmt.Errorf("file block returned invalid file/edit result type %T", responseData)
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executePatchApply(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.PatchApplyOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecutePatchApplyOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	responseData, err := runtimeOperationEventResult(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	response, ok := responseData.(orchestratorevents.ExecutePatchApplyOperationResult)
	if !ok {
		return runtimev2.RuntimeOperationExecutionResult{}, fmt.Errorf("file block returned invalid patch/apply result type %T", responseData)
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func runtimeOperationEventResult(result magicevent.Result) (any, error) {
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if magicErr := result.Error(); magicErr != nil {
		if magicErr.Cause != nil {
			return nil, magicErr.Cause
		}
		return nil, magicErr
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		if magicErr.Cause != nil {
			return nil, magicErr.Cause
		}
		return nil, magicErr
	}
	return data, nil
}

func runtimeOperationExecutionResult(eventID string, operationID string, evidenceKind string, status runtimecontract.RuntimeOperationExecutionStatus, sourceRef string, failure *runtimecontract.FailureFact, evidence runtimecontract.RuntimeOperationEvidence) runtimev2.RuntimeOperationExecutionResult {
	return runtimev2.RuntimeOperationExecutionResult{
		EventID: eventID, OperationID: operationID, EvidenceKind: evidenceKind,
		Status: status, SourceRef: sourceRef, Failure: failure, Evidence: evidence,
	}
}

func (m *Module) executeRuntimeV2ActiveDispatch(ctx context.Context, aggregate runtimev2.RuntimeAggregate) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.rawEventHub == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return m.bizPtr.ExecuteRuntimeV2Dispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, eventRuntimeV2OperationExecutor{hub: m.rawEventHub})
}
