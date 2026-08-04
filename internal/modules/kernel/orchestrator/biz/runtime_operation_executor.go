package orchestrator

// Concrete operation execution is owned by Biz.

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
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
	data, err := fileevents.FileListOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileListOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileListOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileListOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileRead(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := fileevents.FileReadOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileReadOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileReadOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileReadOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeSSH(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := sshevents.OperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := sshevents.ExecuteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, ModuleID, runtimecontract.RuntimeModuleSSH, magicevent.NewHeader(), ctx, command))
	response, err := sshOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeWeb(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := webevents.OperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := webevents.ExecuteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(webevents.TopicExecuteOperation, ModuleID, runtimecontract.RuntimeModuleWeb, magicevent.NewHeader(), ctx, command))
	response, err := webOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeMCP(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := mcpevents.OperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := mcpevents.ExecuteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(mcpevents.TopicExecuteOperation, ModuleID, runtimecontract.RuntimeModuleMCP, magicevent.NewHeader(), ctx, command))
	response, err := mcpOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeGit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := gitevents.OperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := gitevents.ExecuteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind, RequiredOperationRef: request.Operation.RequiredOperationRef,
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), request.CapabilityStates...),
		CompletedRequiredOperationReceipts: append([]runtimecontract.RuntimeRequiredOperationReceiptBinding(nil), request.CompletedRequiredOperationReceipts...),
		TargetRefs:                         append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(gitevents.TopicExecuteOperation, ModuleID, runtimecontract.RuntimeModuleGit, magicevent.NewHeader(), ctx, command))
	response, err := gitOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeShell(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := shellevents.OperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := shellevents.ExecuteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		ValidationRecipes:             append([]runtimecontract.RuntimeValidationRecipe(nil), request.ValidationRecipes...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
	response, err := shellOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileEdit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := fileevents.FileEditOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileEditOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileEditOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileEditOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileWrite(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := fileevents.FileWriteOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileWriteOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecuteFileWriteOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := fileWriteOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchApply(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := fileevents.PatchApplyOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecutePatchApplyOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecutePatchApplyOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := patchApplyOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchPreview(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	data, err := fileevents.PatchPreviewOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecutePatchPreviewOperationCommand{
		Execution:   runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
		OperationID: request.Operation.ID,
		TargetRefs:  append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
		EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory, Data: data,
	}
	result := e.hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecutePatchPreviewOperation, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command))
	response, err := patchPreviewOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func runtimeOperationExecutionResult(request runtime.RuntimeOperationExecutionRequest, receipt runtimecontract.RuntimeOperationReceipt) runtime.RuntimeOperationExecutionResult {
	status, evidence, governance := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
	failure := runtimeOperationFailureFromReceiptFeedback(receipt)
	ret := runtime.RuntimeOperationExecutionResult{
		EventID:     orchestratorevents.RuntimeOperationEventID(request.DispatchID, request.Operation.ID),
		OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(request.DispatchID, request.Operation.ID),
		Failure: failure, Evidence: evidence, Receipt: receipt,
	}
	if governance != nil {
		governance := governance.Clone()
		ret.Governance = &governance
	}
	return ret
}

func runtimeOperationFailureFromReceiptFeedback(receipt runtimecontract.RuntimeOperationReceipt) *runtimecontract.FailureFact {
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
		return nil
	}
	diagnostic := receipt.Feedback.OriginalDiagnostic.CloneOriginal()
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClass(diagnostic.OriginalClass), Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage,
		OriginalDiagnostic: diagnostic.CloneOriginal(), TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if diagnostic.TargetObservation != nil {
		value := diagnostic.TargetObservation.Clone()
		failure.TargetObservation = &value
	}
	if diagnostic.ValidationDiagnostic != nil {
		value := diagnostic.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &value
	}
	if diagnostic.ValidationFailureObservation != nil {
		value := diagnostic.ValidationFailureObservation.Clone()
		failure.ValidationFailureObservation = &value
	}
	if diagnostic.GovernanceDependency != nil {
		value := diagnostic.GovernanceDependency.Clone()
		failure.GovernanceDependency = &value
	}
	return failure
}

func runtimeOperationExecutionEnvelope(execution runtimecontract.RuntimeOperationExecutionRecord, grant *runtimecontract.RuntimeOperationGovernanceGrant) runtimecontract.RuntimeOperationExecutionEnvelope {
	execution = execution.Normalized()
	ret := runtimecontract.RuntimeOperationExecutionEnvelope{
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
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDispatchExecutionRequested, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Executor: eventRuntimeOperationExecutor{hub: m.EventHub()},
	})
	return updated.Aggregate, err
}
