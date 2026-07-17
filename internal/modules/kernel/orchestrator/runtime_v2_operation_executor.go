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
	case runtimecontract.StepKindFileList:
		return e.executeFileList(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindFileRead:
		return e.executeFileRead(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindFileEdit:
		return e.executeFileEdit(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.RuntimeBlockKindPatchApply:
		return e.executePatchApply(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.RuntimeBlockKindPatchPreview:
		return e.executePatchPreview(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession:
		return e.executeShell(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindGitRead, runtimecontract.StepKindGitWrite:
		return e.executeGit(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
		return e.executeMCP(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindSSHCommand:
		return e.executeSSH(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindWebSearch, runtimecontract.StepKindWebFetch:
		return e.executeWeb(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	}
	return runtimev2.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime operation kind %q has no concrete EventHub executor", request.Operation.Kind)
}

func (e eventRuntimeV2OperationExecutor) executeFileList(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileListOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileListOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileListOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileListOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeFileRead(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileReadOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileReadOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileReadOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileReadOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeSSH(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.SSHOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteSSHOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteSSHOperation, ModuleID, runtimecontract.RuntimeModuleSSH, magicevent.NewHeader(), ctx, command))
	response, err := sshOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeWeb(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.WebOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteWebOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs:          append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteWebOperation, ModuleID, runtimecontract.RuntimeModuleWeb, magicevent.NewHeader(), ctx, command))
	response, err := webOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeMCP(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.MCPOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteMCPOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs:          append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteMCPOperation, ModuleID, runtimecontract.RuntimeModuleMCP, magicevent.NewHeader(), ctx, command))
	response, err := mcpOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeGit(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.GitOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteGitOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs:          append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteGitOperation, ModuleID, runtimecontract.RuntimeModuleGit, magicevent.NewHeader(), ctx, command))
	response, err := gitOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executeShell(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.ShellOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteShellOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteShellOperation, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
	response, err := shellOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
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
	response, err := fileEditOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
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
	response, err := patchApplyOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
}

func (e eventRuntimeV2OperationExecutor) executePatchPreview(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtimev2.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.PatchPreviewOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecutePatchPreviewOperationCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := patchPreviewOperationResponse(result)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.Status, response.SourceRef, response.Failure, response.Evidence), nil
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
