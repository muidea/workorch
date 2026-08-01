package file

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindList = modulecommon.StepKindList
const StepKindRead = modulecommon.StepKindRead
const StepKindEdit = modulecommon.StepKindEdit
const StepKindWrite = modulecommon.StepKindWrite
const StepKindPatchApply = modulecommon.StepKindPatchApply
const StepKindPatchPreview = modulecommon.StepKindPatchPreview
const StepKindPatchRevert = modulecommon.StepKindPatchRevert

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{
		Base:           basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine),
		runtimeAdapter: NewRuntimeAdapter(),
	}, nil
}

func NewWithDependencies(deps Dependencies) (*Biz, error) {
	return New(deps.EventHub, deps.BackgroundRoutine)
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, orchestratorevents.TopicOrchestratorAcquireTargetSnapshot, m.handleAcquireTargetSnapshot)
	m.SubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection, m.handleAcquireRuntimeTargetSelection)
	m.SubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, m.handleAcquireWorkspaceManifest)
	m.SubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, orchestratorevents.TopicOrchestratorConfirmRuntimeRepairSurface, m.handleConfirmRuntimeRepairSurface)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileListOperation, m.handleFileListOperation)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileReadOperation, m.handleFileReadOperation)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, m.handleFileEditOperation)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileWriteOperation, m.handleFileWriteOperation)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, m.handlePatchApplyOperation)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, m.handlePatchPreviewOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest)
	m.UnsubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.TopicOrchestratorConfirmRuntimeRepairSurface)
	m.UnsubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.TopicOrchestratorAcquireTargetSnapshot)
	m.UnsubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileEditOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileWriteOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileReadOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteFileListOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation)
	m.runtimeAdapter = nil
}

func (m *Biz) handleFileListOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileListOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecuteFileListOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ListFiles(FileListRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory, Path: command.Data.Path,
		Recursive: command.Data.Recursive, MaxEntries: command.Data.MaxEntries,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileList, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecuteFileListOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func (m *Biz) handleFileReadOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileReadOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecuteFileReadOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ReadFile(event.Context(), FileReadRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory, Path: command.Data.Path, Optional: command.Data.Optional,
		MaxBytes: command.Data.MaxBytes, StartLine: command.Data.StartLine, EndLine: command.Data.EndLine,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileRead, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecuteFileReadOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func (m *Biz) handleFileEditOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	beforeWorkspaceRevision := m.workspaceTreeRevision(command.WorkspaceDeliverableDirectory)
	operationResult, executeErr := m.runtimeAdapter.ApplyFileEdit(FileEditRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		Path:               command.Data.Path, OldText: command.Data.OldText, NewText: command.Data.NewText,
		BaseHash: command.Data.BaseHash, Occurrence: command.Data.Occurrence,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := fileOperationEvidence(operationResult)
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileEdit, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, nil, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecuteFileEditOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func (m *Biz) handleFileWriteOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileWriteOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecuteFileWriteOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	if err := validatePatchApplyReplayProof(command.Execution.ReplayProof, command.OperationID, command.Data.TargetRef); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	beforeWorkspaceRevision := m.workspaceTreeRevision(command.WorkspaceDeliverableDirectory)
	operationResult, executeErr := m.runtimeAdapter.ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		TargetRef:          command.Data.TargetRef, PatchType: "file_write",
		Content: command.Data.Content, ContentPresent: command.Data.ContentPresent,
		ApprovalGranted: command.Execution.GovernanceGrant != nil,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := fileOperationEvidence(operationResult)
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileWrite, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	var governance *runtimecontract.RuntimeGovernanceRequirement
	if failure != nil && failure.Code == "approval_required" {
		governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: command.Data.TargetRef, Reason: failure.Message, RequestedAction: runtimecontract.StepKindFileWrite, RiskLevel: "high"}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, governance, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecuteFileWriteOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func (m *Biz) handlePatchApplyOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecutePatchApplyOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecutePatchApplyOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	beforeWorkspaceRevision := m.workspaceTreeRevision(command.WorkspaceDeliverableDirectory)
	operationResult, executeErr := m.runtimeAdapter.ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		TargetRef:          command.Data.TargetRef, PatchType: command.Data.PatchType,
		Content: command.Data.Content, ContentPresent: command.Data.ContentPresent,
		Diff: command.Data.Diff, OldText: command.Data.OldText, NewText: command.Data.NewText, NewTextPresent: command.Data.NewTextPresent,
		BaseHash: command.Data.BaseHash, Occurrence: command.Data.Occurrence,
		CoverageTargets: command.Data.CoverageTargets, Summary: command.Data.Summary,
		ApprovalGranted: command.Execution.GovernanceGrant != nil,
		ReplayProof:     command.Execution.ReplayProof,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := fileOperationEvidence(operationResult)
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.RuntimeBlockKindPatchApply, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	var governance *runtimecontract.RuntimeGovernanceRequirement
	if failure != nil && failure.Code == "approval_required" {
		governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: command.Data.TargetRef, Reason: failure.Message, RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high"}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, governance, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecutePatchApplyOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func validatePatchApplyReplayProof(proof *runtimecontract.RuntimeOperationReplayProof, operationID, targetRef string) error {
	if proof == nil {
		return nil
	}
	value := proof.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.OwnerModuleID != ModuleID || value.OperationID != strings.TrimSpace(operationID) {
		return fmt.Errorf("runtime patch replay proof operation or owner mismatch")
	}
	targetRef = strings.TrimSpace(targetRef)
	if len(value.TargetRefs) != 1 || value.TargetRefs[0] != targetRef {
		return fmt.Errorf("runtime patch replay proof target mismatch")
	}
	return nil
}

func (m *Biz) handlePatchPreviewOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecutePatchPreviewOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setPrerequisiteError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecutePatchPreviewOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		TargetRef:          command.Data.TargetRef, PatchType: command.Data.PatchType,
		Content: command.Data.Content, ContentPresent: command.Data.ContentPresent,
		Diff: command.Data.Diff, OldText: command.Data.OldText, NewText: command.Data.NewText, NewTextPresent: command.Data.NewTextPresent,
		BaseHash: command.Data.BaseHash, Occurrence: command.Data.Occurrence,
		CoverageTargets: command.Data.CoverageTargets, Summary: command.Data.Summary,
	})
	status := runtimeOperationStatus(executeErr)
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.RuntimeBlockKindPatchPreview, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecutePatchPreviewOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

func runtimeOperationStatus(err error) runtimecontract.RuntimeOperationExecutionStatus {
	if err != nil {
		return runtimecontract.RuntimeOperationExecutionStatusRejected
	}
	return runtimecontract.RuntimeOperationExecutionStatusAccepted
}

func fileOperationEvidence(value RuntimeFileOperationResult) runtimecontract.RuntimeOperationEvidence {
	return runtimecontract.RuntimeOperationEvidence{
		Summary: value.Summary, ContentHash: value.ContentHash, EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
	}
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" ||
		command.PrerequisiteID == "" || command.OwnerModuleID != ModuleID || command.WorkspaceDeliverableDirectory == "" || len(command.RequiredKinds) == 0 {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	inventory, err := m.runtimeAdapter.AcquireRuntimeOperationAdmissibility(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, command.RequiredKinds, command.RequiredTargetRefs, command.MaxItems)
	if setPrerequisiteError(result, err) {
		return
	}
	observation := fileAdmissibilityObservation(command.PrerequisiteID, inventory, command.RequiredTargetRefs, m.admissibilityTargetRevisions(event.Context(), command.WorkspaceDeliverableDirectory, command.RequiredTargetRefs))
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	failure, stale, replayProof := m.runtimeAdapter.ProbeRuntimeOperationFeasibilityWithReplayProof(command.WorkspaceDeliverableDirectory, command.Admissibility, operation, command.Observation)
	prerequisiteRequest := fileOperationSnapshotPrerequisiteRequest(operation, failure, command.CurrentTargetSnapshot)
	category := ownerFeasibilityCategory(failure)
	observation, err := orchestratorevents.NewProbeRuntimeOperationFeasibilityResult(command, failure == nil && stale == nil && prerequisiteRequest == nil, category, failure, stale, prerequisiteRequest, replayProof)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleAcquireWorkspaceManifest(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireWorkspaceManifestCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	manifest, err := m.runtimeAdapter.AcquireWorkspaceManifest(command.WorkspaceDeliverableDirectory)
	if setPrerequisiteError(result, err) {
		return
	}
	observation := fileManifestObservation(command.PrerequisiteID, manifest)
	result.Set(orchestratorevents.AcquireWorkspaceManifestResult{PrerequisiteID: command.PrerequisiteID, Manifest: manifest, Observation: &observation}, nil)
}

// handleRuntimeCWIReconciliationQuery is a read-only File-owner observation.
// It verifies only the activation-sealed target surface and current tree
// revision. It never writes Runtime coverage or interprets Goal/DoD text.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.QueryRuntimeCWIReconciliationCommand)
	if !ok || m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	manifest, err := m.runtimeAdapter.AcquireWorkspaceManifest(command.WorkspaceDeliverableDirectory)
	if err != nil {
		observation := fileReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil,
			"workspace manifest unavailable", "workspace_manifest_unavailable")
		result.Set(orchestratorevents.QueryRuntimeCWIReconciliationResult{Observation: observation, OwnerReceiptRef: fileReconciliationReceiptRef(command, observation)}, nil)
		return
	}
	known := make(map[string]struct{}, len(manifest.TargetRefs))
	for _, target := range manifest.TargetRefs {
		known[target] = struct{}{}
	}
	status := runtimecontract.RuntimeCWIReconciliationObservationSatisfied
	missing := make([]string, 0)
	targetRefs := append([]string(nil), command.Binding.TargetRefs...)
	artifactOnly := len(targetRefs) == 0 && len(command.Binding.ArtifactRefs) != 0
	observedRevision := manifest.Revision
	if len(targetRefs) == 0 && len(command.Binding.ArtifactRefs) != 0 {
		// Artifact refs are only admissible when File can resolve them directly
		// against its own manifest.  URI/semantic references are not guessed.
		targetRefs = append([]string(nil), command.Binding.ArtifactRefs...)
	}
	if len(targetRefs) == 0 && command.Binding.TargetSelectorRef != "" {
		selection, selectionErr := m.runtimeAdapter.AcquireRuntimeTargetSelection(
			command.WorkspaceDeliverableDirectory,
			"reconciliation/"+command.RequestID+"/"+command.Binding.BindingID,
			"reconciliation",
			command.Binding.TargetSelectorRef,
			command.Binding.ProspectiveTargetRef,
			runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
			runtimeTargetSelectionBinding{ownerRef: command.Binding.TargetSelectorOwnerRef, subjectRef: command.Binding.TargetSelectorSubjectRef},
		)
		if selectionErr == nil {
			selection = selection.Normalized()
			observedRevision = selection.ObservedRevision
			targetRefs = append(targetRefs, selection.TargetRefs...)
			switch selection.Disposition {
			case runtimecontract.RuntimeTargetSelectionSelected, runtimecontract.RuntimeTargetSelectionScope:
				if len(selection.TargetRefs) == 0 {
					status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
				}
			case runtimecontract.RuntimeTargetSelectionCreate, runtimecontract.RuntimeTargetSelectionAbsent:
				status = runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied
			default:
				status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
			}
		} else {
			status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
		}
	}
	if len(targetRefs) == 0 && status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
	} else {
		for _, target := range targetRefs {
			if _, found := known[target]; !found {
				missing = append(missing, target)
			}
		}
		if len(missing) != 0 {
			status = runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied
		}
	}
	message := ""
	code := ""
	if len(missing) != 0 && artifactOnly {
		status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
		message = "binding artifact reference is not a directly verifiable File target"
		code = "artifact_scope_unverifiable"
	} else if len(missing) != 0 {
		message = "sealed file target is absent from the current workspace"
		code = "target_missing"
	} else if status == runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		message = "binding target selector or artifact cannot be independently verified by File owner"
		code = "target_scope_unverifiable"
	} else if status == runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied {
		message = "sealed File target selection is absent from the current workspace"
		code = "target_selection_unsatisfied"
	}
	observation := fileReconciliationObservation(command, status, observedRevision, targetRefs, message, code)
	result.Set(orchestratorevents.QueryRuntimeCWIReconciliationResult{Observation: observation, OwnerReceiptRef: fileReconciliationReceiptRef(command, observation)}, nil)
}

func fileReconciliationObservation(command orchestratorevents.QueryRuntimeCWIReconciliationCommand, status runtimecontract.RuntimeCWIReconciliationObservationStatus, revision string, targetRefs []string, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	if strings.TrimSpace(revision) == "" {
		revision = command.WorkspaceBaselineRevision
	}
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: ModuleID, ObservedRevision: revision, Status: status,
		TargetRefs: targetRefs, ArtifactRefs: append([]string(nil), command.Binding.ArtifactRefs...),
	}
	if status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		value.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "file-reconciliation/" + code, TargetRefs: append([]string(nil), targetRefs...), ObservedRevision: revision,
		}
	}
	return value.Normalized()
}

func fileReconciliationReceiptRef(command orchestratorevents.QueryRuntimeCWIReconciliationCommand, observation runtimecontract.RuntimeCWIReconciliationObservation) string {
	return "file-reconciliation/" + runtimecontract.StableRuntimeCWIReconciliationObservationRef(observation)
}

// handleConfirmRuntimeRepairSurface is a File-owner read observation.  File
// verifies only the current regular workspace surface; it never derives a
// Runtime action from the diagnostic or mutates the Runtime aggregate.
func (m *Biz) handleConfirmRuntimeRepairSurface(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ConfirmRuntimeRepairSurfaceCommand)
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	observation := command.Observation.Normalized()
	if observation.WorkspaceID != command.WorkspaceID || observation.RunID != command.RunID || observation.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	manifest, err := m.runtimeAdapter.AcquireWorkspaceManifest(command.WorkspaceDeliverableDirectory)
	if setPrerequisiteError(result, err) {
		return
	}
	known := make(map[string]struct{}, len(manifest.TargetRefs))
	for _, target := range manifest.TargetRefs {
		known[target] = struct{}{}
	}
	confirmed := true
	for _, target := range observation.CandidateTargetRefs {
		if _, exists := known[target]; !exists {
			confirmed = false
			break
		}
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", ObservedRevision: manifest.Revision,
		Disposition: runtimecontract.RuntimeRepairSurfaceRejected, Reason: "candidate target is not a current regular workspace file",
	}
	if confirmed {
		confirmation.Disposition = runtimecontract.RuntimeRepairSurfaceConfirmed
		confirmation.ConfirmedTargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
		confirmation.Reason = ""
	}
	if err := confirmation.Validate(); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	result.Set(orchestratorevents.ConfirmRuntimeRepairSurfaceResult{Confirmation: confirmation}, nil)
}

func (m *Biz) handleAcquireTargetSnapshot(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireTargetSnapshotCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRef == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	snapshot, err := m.runtimeAdapter.AcquireTargetSnapshot(event.Context(), command.WorkspaceDeliverableDirectory, command.TargetRef, command.MaxBytes)
	if setPrerequisiteError(result, err) {
		return
	}
	observation := fileTargetSnapshotObservation(command.PrerequisiteID, snapshot)
	result.Set(orchestratorevents.AcquireTargetSnapshotResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot, Observation: &observation}, nil)
}

func (m *Biz) handleAcquireRuntimeTargetSelection(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeTargetSelectionCommand)
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRole == "" || command.TargetSelectorRef == "" || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	selection, err := m.runtimeAdapter.AcquireRuntimeTargetSelection(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, command.TargetRole, command.TargetSelectorRef, command.ProspectiveTargetRef, command.MaxItems, runtimeTargetSelectionBinding{ownerRef: command.TargetSelectorOwnerRef, subjectRef: command.TargetSelectorSubjectRef, candidateRefs: command.TargetSelectorCandidateRefs})
	if setPrerequisiteError(result, err) {
		return
	}
	observation := fileTargetSelectionObservation(command.PrerequisiteID, selection)
	result.Set(orchestratorevents.AcquireRuntimeTargetSelectionResult{PrerequisiteID: command.PrerequisiteID, Selection: selection, Observation: &observation}, nil)
}

func setPrerequisiteError(result magicevent.Result, err error) bool {
	if result == nil {
		return true
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return true
	}
	return false
}

func fileReadOnlyReceiptObservation() orchestratorevents.RuntimeOperationReceiptObservation {
	return orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
}

func (m *Biz) workspaceTreeRevision(workspaceDirectory string) string {
	if m == nil || m.runtimeAdapter == nil {
		return ""
	}
	manifest, err := m.runtimeAdapter.AcquireWorkspaceManifest(workspaceDirectory)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Revision)
}

// admissibilityTargetRevisions keeps File owner freshness exact for the
// bounded targets that an inventory actually authorizes. A workspace-tree
// revision alone is insufficient: a delayed inventory result must be rejected
// when an admitted target changes even if an owner-specific tree revision is
// unchanged or unavailable.
func (m *Biz) admissibilityTargetRevisions(ctx context.Context, workspaceDirectory string, targetRefs []string) map[string]string {
	if m == nil || m.runtimeAdapter == nil {
		return nil
	}
	values := make(map[string]string, len(targetRefs))
	for _, targetRef := range targetRefs {
		targetRef = strings.TrimSpace(targetRef)
		if targetRef == "" {
			continue
		}
		result, err := m.runtimeAdapter.ReadFile(ctx, FileReadRequest{
			WorkspaceDirectory: workspaceDirectory, Path: targetRef, MaxBytes: 1,
		})
		if err == nil && result.Exists && strings.TrimSpace(result.ContentHash) != "" {
			values[targetRef] = strings.TrimSpace(result.ContentHash)
		}
	}
	return values
}

func fileMutationReceiptObservation(workspaceDirectory string, targetRefs []string, status runtimecontract.RuntimeOperationExecutionStatus, result RuntimeFileOperationResult, beforeWorkspaceRevisions ...string) orchestratorevents.RuntimeOperationReceiptObservation {
	observation := orchestratorevents.RuntimeOperationReceiptObservation{
		ObservedEffectScope: runtimecontract.RuntimeEffectUnknown,
		BeforeHash:          result.BeforeHash,
		AfterHash:           result.AfterHash,
	}
	if status != runtimecontract.RuntimeOperationExecutionStatusAccepted {
		return observation
	}
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
	observation.Changed = result.BeforeHash != result.AfterHash
	observation.NoOp = !observation.Changed
	if observation.NoOp {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectNone
		return observation
	}
	for _, targetRef := range targetRefs {
		targetRef = strings.TrimSpace(targetRef)
		if targetRef == "" {
			continue
		}
		observation.ResourceChanges = append(observation.ResourceChanges, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: ModuleID, SubjectRef: targetRef,
			BeforeRevision: result.BeforeHash, AfterRevision: result.AfterHash,
		})
	}
	beforeWorkspaceRevision := ""
	if len(beforeWorkspaceRevisions) > 0 {
		beforeWorkspaceRevision = strings.TrimSpace(beforeWorkspaceRevisions[0])
	}
	if manifest, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspaceDirectory); err == nil && manifest.Revision != "" &&
		(beforeWorkspaceRevision == "" || beforeWorkspaceRevision != manifest.Revision) {
		observation.ResourceChanges = append(observation.ResourceChanges, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable",
			BeforeRevision: beforeWorkspaceRevision, AfterRevision: manifest.Revision,
		})
	}
	return observation
}

func fileManifestObservation(prerequisiteID string, manifest runtimecontract.WorkspaceManifest) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: ModuleID, ContentHash: manifest.Revision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", Revision: manifest.Revision}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func fileTargetSnapshotObservation(prerequisiteID string, snapshot runtimecontract.TargetSnapshot) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: ModuleID, ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: ModuleID, SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func fileTargetSelectionObservation(prerequisiteID string, selection runtimecontract.RuntimeTargetSelection) runtimecontract.RuntimePrerequisiteObservation {
	selection = selection.Normalized()
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: ModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", Revision: selection.ObservedRevision}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func fileAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility, requiredTargetRefs []string, targetRevisions map[string]string) runtimecontract.RuntimePrerequisiteObservation {
	dependencies := []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision}}
	for _, targetRef := range requiredTargetRefs {
		targetRef = strings.TrimSpace(targetRef)
		if targetRef == "" {
			continue
		}
		if revision := strings.TrimSpace(targetRevisions[targetRef]); revision != "" {
			dependencies = append(dependencies, runtimecontract.RuntimeFreshnessDependency{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: ModuleID, SubjectRef: targetRef, Revision: revision})
		}
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID, Dependencies: dependencies,
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}
