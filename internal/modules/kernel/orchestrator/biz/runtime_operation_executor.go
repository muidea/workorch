package orchestrator

// Concrete operation execution is owned by Biz.

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeOperationExecutor struct {
	hub magicevent.Hub
}

func (e eventRuntimeOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	if e.hub == nil || request.WorkspaceID == "" || request.RunID == "" || request.DispatchID == "" {
		return runtime.RuntimeOperationExecutionResult{}, common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, request.WorkspaceID)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	destination := runtimecontract.RuntimeModuleForOperationKind(request.Operation.Kind)
	if destination == "" {
		return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime operation kind %q has no owner module", request.Operation.Kind)
	}
	workspace, err := (workspaceEventRepository{hub: e.hub}).Get(ctx, request.WorkspaceID)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	switch runtimecontract.CanonicalRuntimeBlockKind(request.Operation.Kind) {
	case runtimecontract.StepKindFileList:
		return e.executeFileList(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindFileRead:
		return e.executeFileRead(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindFileEdit:
		return e.executeFileEdit(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
	case runtimecontract.StepKindFileWrite:
		return e.executeFileWrite(ctx, request, workspace.DeliverableDirectory, workspace.InternalDirectory)
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
	return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime operation kind %q has no concrete EventHub executor", request.Operation.Kind)
}

func (e eventRuntimeOperationExecutor) executeFileList(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileListOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileListOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileListOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileListOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileRead(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileReadOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileReadOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileReadOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileReadOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeSSH(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.SSHOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteSSHOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteSSHOperation, ModuleID, runtimecontract.RuntimeModuleSSH, magicevent.NewHeader(), ctx, command))
	response, err := sshOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeWeb(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.WebOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteWebOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteWebOperation, ModuleID, runtimecontract.RuntimeModuleWeb, magicevent.NewHeader(), ctx, command))
	response, err := webOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeMCP(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.MCPOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteMCPOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteMCPOperation, ModuleID, runtimecontract.RuntimeModuleMCP, magicevent.NewHeader(), ctx, command))
	response, err := mcpOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeGit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.GitOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteGitOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind, RequiredOperationRef: request.Operation.RequiredOperationRef,
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), request.CapabilityStates...),
		CompletedRequiredOperationReceipts: append([]runtimecontract.RuntimeRequiredOperationReceiptBinding(nil), request.CompletedRequiredOperationReceipts...),
		TargetRefs:                         append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteGitOperation, ModuleID, runtimecontract.RuntimeModuleGit, magicevent.NewHeader(), ctx, command))
	response, err := gitOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeShell(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.ShellOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteShellOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		ValidationRecipes:             append([]runtimecontract.RuntimeValidationRecipe(nil), request.ValidationRecipes...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteShellOperation, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
	response, err := shellOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileEdit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileEditOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileEditOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileEditOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileWrite(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.FileWriteOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecuteFileWriteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteFileWriteOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileWriteOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchApply(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.PatchApplyOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecutePatchApplyOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := patchApplyOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchPreview(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.PatchPreviewOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := orchestratorevents.ExecutePatchPreviewOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := patchPreviewOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(response.EventID, response.OperationID, response.EvidenceKind, response.SourceRef, response.Receipt), nil
}

func runtimeOperationExecutionResult(eventID string, operationID string, evidenceKind string, sourceRef string, receipt runtimecontract.RuntimeOperationReceipt) runtime.RuntimeOperationExecutionResult {
	status, evidence, failure, governance := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
	ret := runtime.RuntimeOperationExecutionResult{
		EventID: eventID, OperationID: operationID, EvidenceKind: evidenceKind,
		Status: status, SourceRef: sourceRef, Failure: failure, Evidence: evidence, Receipt: receipt,
	}
	if governance != nil {
		governance := governance.Clone()
		ret.Governance = &governance
	}
	return ret
}

func runtimeOperationExecutionEnvelope(execution runtimecontract.RuntimeOperationExecutionRecord, grant *runtimecontract.RuntimeOperationGovernanceGrant) orchestratorevents.RuntimeOperationExecutionEnvelope {
	execution = execution.Normalized()
	ret := orchestratorevents.RuntimeOperationExecutionEnvelope{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, ExpectedEffectScope: execution.ExpectedEffectScope,
	}
	if execution.ReplayProof != nil {
		proof := execution.ReplayProof.Clone()
		ret.ReplayProof = &proof
	}
	if grant != nil {
		value := grant.Normalized()
		ret.GovernanceGrant = &value
	}
	return ret
}

func (m *Biz) executeRuntimeActiveDispatch(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDispatchExecutionRequested, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Executor: eventRuntimeOperationExecutor{hub: m.EventHub()},
	})
	return updated.Aggregate, err
}
