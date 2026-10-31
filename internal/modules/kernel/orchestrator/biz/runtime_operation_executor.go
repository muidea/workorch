package orchestrator

// Concrete operation execution is owned by Biz.

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeOperationExecutor struct {
	hub magicevent.Hub
}

func runtimeOperationExecutionEvent(ctx context.Context, ownerContracts []eventcontract.Contract, topic, destination string, request runtime.RuntimeOperationExecutionRequest, data any) magicevent.Event {
	event := magicevent.NewEventWithContext(topic, ModuleID, destination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(ownerContracts, topic, eventcontract.LaneScope{
		SubjectType: "operation_execution",
		SubjectID:   request.Execution.ExecutionKey,
		WorkspaceID: string(request.WorkspaceID),
		RunID:       string(request.RunID),
		StepID:      request.Execution.ExecutionKey,
	}))
	return event
}

func (e eventRuntimeOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	if e.hub == nil || request.WorkspaceID == "" || request.RunID == "" || request.DispatchID == "" || strings.TrimSpace(request.Execution.ExecutionKey) == "" {
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
	workspace, err := (workspaceSnapshotReader{hub: e.hub}).Read(ctx, request.WorkspaceID)
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
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileListOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecuteFileListOperation, runtimecontract.RuntimeModuleFile, request, command))
	response, err := fileListOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileRead(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileReadOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecuteFileReadOperation, runtimecontract.RuntimeModuleFile, request, command))
	response, err := fileReadOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeSSH(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil || request.OwnerCatalogObservation == nil {
		return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime SSH execution requires persisted owner catalog and observation")
	}
	sealedOperation, err := sshevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog, *request.OwnerCatalogObservation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := sshevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, sshevents.Contracts(), sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleSSH, request, command))
	response, err := sshOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeWeb(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil || request.OwnerCatalogObservation == nil {
		return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime Web execution requires persisted owner catalog and observation")
	}
	sealedOperation, err := webevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog, *request.OwnerCatalogObservation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := webevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, webevents.Contracts(), webevents.TopicExecuteOperation, runtimecontract.RuntimeModuleWeb, request, command))
	response, err := webOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeMCP(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil {
		return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime MCP execution requires persisted owner catalog")
	}
	sealedOperation, err := mcpevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
		TargetRole:                       request.TargetRole,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, mcpevents.Contracts(), mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleMCP, request, command))
	response, err := mcpOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeGit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := gitevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := gitevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext:   runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                    sealedOperation,
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), request.CapabilityStates...),
		WorkspaceTreeObservation:           cloneRuntimePrerequisiteObservation(request.WorkspaceTreeObservation),
		CompletedRequiredOperationReceipts: append([]runtimecontract.RuntimeRequiredOperationReceiptBinding(nil), request.CompletedRequiredOperationReceipts...),
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, gitevents.Contracts(), gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleGit, request, command))
	response, err := gitOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func cloneRuntimePrerequisiteObservation(value *runtimecontract.RuntimePrerequisiteObservation) *runtimecontract.RuntimePrerequisiteObservation {
	if value == nil {
		return nil
	}
	cloned := value.Normalized()
	cloned.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), cloned.Dependencies...)
	return &cloned
}

func (e eventRuntimeOperationExecutor) executeShell(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil || request.OwnerCatalogObservation == nil {
		return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("runtime Shell execution requires persisted owner catalog and observation")
	}
	sealedOperation, err := shellevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation:        *request.OwnerCatalogObservation,
		TargetRole:                request.TargetRole,
		ValidationScopeTargetRefs: append([]string(nil), request.ValidationScopeTargetRefs...),
		EvidencePolicies:          append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		ValidationProofBinding:    request.ValidationProofBinding.Clone(),
	})
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, shellevents.Contracts(), shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleShell, request, command))
	response, err := shellOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileEdit(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileEditOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
		ResultDeliveryProtocolRef:        fileResultDeliveryProtocol(internalDirectory),
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecuteFileEditOperation, runtimecontract.RuntimeModuleFile, request, command))
	response, err := fileEditOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executeFileWrite(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecuteFileWriteOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
		ResultDeliveryProtocolRef:        fileResultDeliveryProtocol(internalDirectory),
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecuteFileWriteOperation, runtimecontract.RuntimeModuleFile, request, command))
	response, err := fileWriteOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchApply(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecutePatchApplyOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
		ResultDeliveryProtocolRef:        fileResultDeliveryProtocol(internalDirectory),
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecutePatchApplyOperation, runtimecontract.RuntimeModuleFile, request, command))
	response, err := patchApplyOperationResponse(result)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtimeOperationExecutionResult(request, response.Receipt), nil
}

func (e eventRuntimeOperationExecutor) executePatchPreview(ctx context.Context, request runtime.RuntimeOperationExecutionRequest, deliverableDirectory string, internalDirectory string) (runtime.RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	command := fileevents.ExecutePatchPreviewOperationCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, deliverableDirectory, internalDirectory),
		SealedOperation:                  sealedOperation,
	}
	result := e.hub.Send(runtimeOperationExecutionEvent(ctx, fileevents.Contracts(), fileevents.TopicExecutePatchPreviewOperation, runtimecontract.RuntimeModuleFile, request, command))
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
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash, ExpectedEffectScope: execution.ExpectedEffectScope,
		IdempotencyClass: execution.IdempotencyClass, AuthorityHash: execution.AuthorityHash,
		AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID, AcquisitionBinding: execution.AcquisitionBinding,
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

func runtimeOperationExecutionContext(request runtime.RuntimeOperationExecutionRequest, deliverableDirectory, internalDirectory string) runtimecontract.RuntimeOperationExecutionContext {
	return runtimecontract.CloneRuntimeOperationExecutionContext(runtimecontract.RuntimeOperationExecutionContext{
		Execution:                        runtimeOperationExecutionEnvelope(request.Execution, request.GovernanceGrant),
		WorkspaceID:                      request.WorkspaceID,
		RunID:                            request.RunID,
		DispatchID:                       request.DispatchID,
		Transaction:                      request.Transaction,
		OperationID:                      request.Operation.ID,
		CoverageWorkItemIDs:              append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
		TargetRefs:                       append([]string(nil), request.Operation.TargetRefs...),
		EvidenceKind:                     request.EvidenceKind,
		HistoricalEffectProofContractRef: request.HistoricalEffectProofContractRef,
		EvidencePolicies:                 append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
		WorkspaceDeliverableDirectory:    deliverableDirectory,
		WorkspaceInternalDirectory:       internalDirectory,
	})
}

func (m *Biz) executeRuntimeActiveDispatch(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	recoveryClaimID := ""
	var operationCallErr error
	maxTransitions := runtimecontract.RuntimeCandidateOperationMaxCount + runtimecontract.RuntimeRecoveryBudgetLimit*3 + 4
	for transition := 0; transition <= maxTransitions; transition++ {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventDispatchExecutionRequested,
			Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
			Transaction: &runtime.RuntimeInternalTransactionPayload{RecoveryClaimID: recoveryClaimID, RecoveryClaimNow: time.Now().UTC().Format(time.RFC3339Nano)},
		})
		recoveryClaimID = ""
		if operationID, authorizationID, claimRequired := runtime.RuntimeOperationRecoveryAuthorizationClaimRequired(err); claimRequired {
			claimed, claimErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:     runtime.RuntimeInternalEventOperationRecoveryClaimed,
				Scope:    runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
				Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryOperationID: operationID, RecoveryAuthorizationID: authorizationID, LeaseNow: time.Now().UTC().Format(time.RFC3339Nano)},
			})
			if claimErr != nil {
				return runtime.RuntimeAggregate{}, claimErr
			}
			if !claimed.RecoveryClaimChanged || claimed.RecoveryClaimID == "" {
				// Another worker or a response-loss replay already claimed this
				// grant. Persisted in-flight state cannot be turned back into a
				// physical-call token by a duplicate delivery.
				return runtime.RuntimeAggregate{}, common.ErrConflict
			}
			aggregate = claimed.Aggregate
			recoveryClaimID = claimed.RecoveryClaimID
			continue
		}
		if operationID, required := runtime.RuntimeOperationOutcomeUnknownTransitionRequired(err); required {
			// Resolve saved owner material before classifying a claimed operation as
			// unknown. This path never obtains or spends a physical-call token.
			if deliveryErr := m.deliverRuntimeOperationResult(ctx, aggregate, operationID); deliveryErr != nil && operationCallErr == nil {
				operationCallErr = deliveryErr
			}
			loaded, loadErr := m.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if loadErr != nil {
				return runtime.RuntimeAggregate{}, loadErr
			}
			aggregate = loaded
			if aggregate.ActiveDispatch == nil {
				return aggregate, nil
			}
			committed := false
			for _, execution := range aggregate.ActiveDispatch.OperationExecutions {
				if execution.OperationID == operationID && execution.OwnerReceiptRef != "" {
					committed = true
					break
				}
			}
			if committed {
				operationCallErr = nil
				continue
			}
			observed, observeErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:        runtime.RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
				Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
				Transaction: &runtime.RuntimeInternalTransactionPayload{OperationOutcomeUnknownID: operationID, OperationOutcomeDiagnostic: operationResultDiagnostic(aggregate, operationID, operationCallErr)},
			})
			if observeErr != nil {
				return observed.Aggregate, observeErr
			}
			aggregate = observed.Aggregate
			if operationCallErr != nil {
				return aggregate, operationCallErr
			}
			continue
		}
		if err == nil && updated.OperationCall != nil {
			call := updated.OperationCall
			if call.Validate() != nil {
				return runtime.RuntimeAggregate{}, common.ErrConflict
			}
			aggregate = updated.Aggregate
			_, operationCallErr = (eventRuntimeOperationExecutor{hub: m.EventHub()}).ExecuteRuntimeOperation(ctx, call.Request)
			// 工作 receipt 已在 owner callback 的唯一 CAS 中结算。重新读取原
			// dispatch/receipt/execution，不能第二次 evidence CAS 或凭 response 判成功。
			if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ImplementationWorkCommitment != nil {
				loaded, loadErr := m.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
				if loadErr != nil {
					return runtime.RuntimeAggregate{}, loadErr
				}
				if receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(loaded.OperationReceipts, call.Request.Execution.ExecutionKey, call.Request.Execution.Fence); found {
					if receipt.OperationID != call.Request.Operation.ID {
						return loaded, common.ErrConflict
					}
					operationCallErr = nil
					aggregate = loaded
					if aggregate.ActiveDispatch == nil {
						return aggregate, nil
					}
					continue
				}
				aggregate = loaded
			}
			if call.RecoveryClaimID != "" {
				returned, returnErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
					Kind:  runtime.RuntimeInternalEventOperationRecoveryCallReturned,
					Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
					Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryOperationID: call.Request.Operation.ID,
						RecoveryAuthorizationID: call.RecoveryAuthorizationID, RecoveryClaimID: call.RecoveryClaimID},
				})
				if returnErr != nil {
					return returned.Aggregate, returnErr
				}
				aggregate = returned.Aggregate
			}
			// Capability owners synchronously commit their exact terminal receipt
			// before returning. Re-enter the preparation event and derive the next
			// owner-private fact only from that persisted receipt; an error/no-receipt
			// path is classified from the durable claim on the next iteration.
			continue
		}
		if err == nil && updated.DispatchExecution != nil {
			handoff := updated.DispatchExecution
			if handoff.Validate() != nil {
				return runtime.RuntimeAggregate{}, common.ErrConflict
			}
			// A persisted, validated receipt is the canonical call outcome. Any
			// synchronous transport error from that call was response loss only.
			operationCallErr = nil
			event := runtime.RuntimeInternalEvent{
				Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
				Transaction: &runtime.RuntimeInternalTransactionPayload{},
			}
			switch {
			case handoff.Evidence != nil:
				event.Kind = runtime.RuntimeInternalEventOperationEvidenceObserved
				event.Transaction.ExecutionEvidence = handoff.Evidence
			case handoff.PrerequisiteStaleReceipt != nil:
				event.Kind = runtime.RuntimeInternalEventOperationPrerequisiteStaleObserved
				event.Transaction.DispatchID = handoff.DispatchID
				event.Transaction.OperationPrerequisiteStaleReceipt = handoff.PrerequisiteStaleReceipt
			case handoff.GovernanceRequired != nil:
				event.Kind = runtime.RuntimeInternalEventOperationGovernanceRequiredObserved
				event.Transaction.OperationGovernanceResult = handoff.GovernanceRequired
			default:
				return runtime.RuntimeAggregate{}, common.ErrConflict
			}
			committed, commitErr := m.submitRuntimeInternalEvent(ctx, event)
			if commitErr != nil {
				return runtime.RuntimeAggregate{}, commitErr
			}
			aggregate = committed.Aggregate
			if aggregate.ActiveDispatch == nil {
				return aggregate, nil
			}
			continue
		}
		operationID, required := runtime.RuntimeOperationRecoveryBudgetRequired(err)
		if !required {
			return updated.Aggregate, err
		}
		if operationID == "" {
			return runtime.RuntimeAggregate{}, common.ErrConflict
		}
		budget, budgetErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:     runtime.RuntimeInternalEventRecoveryBudgetConsumed,
			Scope:    runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
			Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery, RecoveryOperationID: operationID, LeaseNow: time.Now().UTC().Format(time.RFC3339Nano)},
		})
		if budgetErr != nil {
			return runtime.RuntimeAggregate{}, budgetErr
		}
		if budget.Aggregate.Facts.WorkspaceID != aggregate.Facts.WorkspaceID || budget.Aggregate.Facts.RunID != aggregate.Facts.RunID {
			return runtime.RuntimeAggregate{}, common.ErrConflict
		}
		aggregate = budget.Aggregate
	}
	return runtime.RuntimeAggregate{}, common.ErrConflict
}

func fileResultDeliveryProtocol(internalDirectory string) string {
	if internalDirectory != "" {
		return fileevents.MutationResultDeliveryV1
	}
	return ""
}
