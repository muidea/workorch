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
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteFileListOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ListFiles(FileListRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory, Path: command.Data.Path,
		Recursive: command.Data.Recursive, MaxEntries: command.Data.MaxEntries,
	})
	entries := make([]orchestratorevents.FileListEntry, 0, len(operationResult.Entries))
	for _, entry := range operationResult.Entries {
		entries = append(entries, orchestratorevents.FileListEntry{
			Path: entry.Path, Kind: entry.Kind, Size: entry.Size, ModifiedAt: entry.ModifiedAt,
			ContentHash: entry.ContentHash, Binary: entry.Binary, Ignored: entry.Ignored,
		})
	}
	response := orchestratorevents.ExecuteFileListOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.FileListOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Path: operationResult.Path, Entries: entries,
			Truncated: operationResult.Truncated, MaxEntries: operationResult.MaxEntries,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage,
		},
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileList, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteFileReadOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ReadFile(event.Context(), FileReadRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory, Path: command.Data.Path, Optional: command.Data.Optional,
		MaxBytes: command.Data.MaxBytes, StartLine: command.Data.StartLine, EndLine: command.Data.EndLine,
	})
	response := orchestratorevents.ExecuteFileReadOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.FileReadOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Path: operationResult.Path, TargetRef: operationResult.TargetRef,
			Exists: operationResult.Exists, Content: operationResult.Content, ContentHash: operationResult.ContentHash,
			Size: operationResult.Size, ModifiedAt: operationResult.ModifiedAt, LineCount: operationResult.LineCount,
			Truncated: operationResult.Truncated, Binary: operationResult.Binary, Redacted: operationResult.Redacted,
			ContentComplete: operationResult.ContentComplete, SafeForPatch: operationResult.SafeForPatch, SafeForFileWrite: operationResult.SafeForFileWrite,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage,
		},
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileRead, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	beforeWorkspaceRevision := m.workspaceTreeRevision(command.WorkspaceDeliverableDirectory)
	operationResult, executeErr := m.runtimeAdapter.ApplyFileEdit(FileEditRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		Path:               command.Data.Path, OldText: command.Data.OldText, NewText: command.Data.NewText,
		BaseHash: command.Data.BaseHash, Occurrence: command.Data.Occurrence,
	})
	response := orchestratorevents.ExecuteFileEditOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID),
		Evidence:  fileOperationEvidence(operationResult),
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileEdit, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, nil, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, response.Status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteFileWriteOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
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
	response := orchestratorevents.ExecuteFileWriteOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID),
		Evidence:  fileOperationEvidence(operationResult),
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindFileWrite, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	var governance *runtimecontract.RuntimeGovernanceRequirement
	if response.Failure != nil && response.Failure.Code == "approval_required" {
		governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: command.Data.TargetRef, Reason: response.Failure.Message, RequestedAction: runtimecontract.StepKindFileWrite, RiskLevel: "high"}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, governance, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, response.Status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
		status, evidence, failure, governance := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		if governance != nil {
			failure = nil
		}
		result.Set(orchestratorevents.ExecutePatchApplyOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
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
	response := orchestratorevents.ExecutePatchApplyOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID),
		Evidence:  fileOperationEvidence(operationResult),
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.RuntimeBlockKindPatchApply, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	var governance *runtimecontract.RuntimeGovernanceRequirement
	if response.Failure != nil && response.Failure.Code == "approval_required" {
		governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: command.Data.TargetRef, Reason: response.Failure.Message, RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high"}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, governance, fileMutationReceiptObservation(command.WorkspaceDeliverableDirectory, operationResult.TargetRefs, response.Status, operationResult, beforeWorkspaceRevision))
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecutePatchPreviewOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
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
	response := orchestratorevents.ExecutePatchPreviewOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimeOperationStatus(executeErr),
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.PatchPreviewOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Operation: operationResult.Operation, PatchType: operationResult.PatchType,
			TargetRef: operationResult.TargetRef, AffectedFiles: operationResult.AffectedFiles,
			BeforeHash: operationResult.BeforeHash, AfterHash: operationResult.AfterHash, ContentHash: operationResult.ContentHash, DiffHash: operationResult.DiffHash,
			AddedLines: operationResult.AddedLines, DeletedLines: operationResult.DeletedLines,
			ApprovalRequired: operationResult.ApprovalRequired, RiskLevel: operationResult.RiskLevel, RiskReasons: operationResult.RiskReasons,
			WillWrite: operationResult.WillWrite, Summary: operationResult.Summary,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage,
		},
	}
	if executeErr != nil {
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.RuntimeBlockKindPatchPreview, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, fileReadOnlyReceiptObservation())
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
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
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindList, StepKindRead, StepKindEdit, StepKindWrite, StepKindPatchApply, StepKindPatchPreview)
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
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.Transaction.AuthoritySnapshotID == "" || command.WorkspaceDeliverableDirectory == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	operation := command.Operation.Normalized()
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || descriptor.OwnerModuleID != ModuleID || command.Admissibility.OwnerModuleID != ModuleID {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	failure, stale, replayProof := m.runtimeAdapter.ProbeRuntimeOperationFeasibilityWithReplayProof(command.WorkspaceDeliverableDirectory, command.Admissibility, operation, command.Observation)
	prerequisiteRequest := fileOperationSnapshotPrerequisiteRequest(operation, failure, command.CurrentTargetSnapshot)
	result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{
		OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID,
		CapabilityStateBinding: command.Admissibility.AcquisitionBinding,
		Feasible:               failure == nil && stale == nil, ReplayProof: replayProof, Failure: failure, Stale: stale, PrerequisiteRequest: prerequisiteRequest,
	}, nil)
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
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRole == "" || command.TargetSelectorRef == "" || command.MaxItems < 1 {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	selection, err := m.runtimeAdapter.AcquireRuntimeTargetSelection(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, command.TargetRole, command.TargetSelectorRef, command.MaxItems)
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
