package file

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindList = modulecommon.StepKindList
const StepKindRead = modulecommon.StepKindRead
const StepKindEdit = modulecommon.StepKindEdit
const StepKindPatchApply = modulecommon.StepKindPatchApply
const StepKindPatchPreview = modulecommon.StepKindPatchPreview
const StepKindPatchRevert = modulecommon.StepKindPatchRevert

type Module struct {
	runtimeAdapter       *filebiz.RuntimeAdapter
	prerequisiteObserver magicevent.SimpleObserver
	capabilityObserver   magicevent.SimpleObserver
	eventHub             magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub magicevent.Hub
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
	}
	m.runtimeAdapter = filebiz.NewRuntimeAdapter()
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.capabilityObserver != nil {
		return nil
	}
	m.prerequisiteObserver = magicevent.NewSimpleObserverWithMatchID(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, m.eventHub)
	m.prerequisiteObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireTargetSnapshot, m.handleAcquireTargetSnapshot)
	m.prerequisiteObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, m.handleAcquireWorkspaceManifest)
	m.capabilityObserver = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileListOperation, m.handleFileListOperation)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileReadOperation, m.handleFileReadOperation)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, m.handleFileEditOperation)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, m.handlePatchApplyOperation)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation, m.handlePatchPreviewOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.prerequisiteObserver != nil {
		m.prerequisiteObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest)
		m.prerequisiteObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireTargetSnapshot)
	}
	m.prerequisiteObserver = nil
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecutePatchPreviewOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileReadOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileListOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleFileListOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileListOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ListFiles(filebiz.FileListRequest{
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
	result.Set(response, nil)
}

func (m *Module) handleFileReadOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileReadOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ReadFile(event.Context(), filebiz.FileReadRequest{
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
	result.Set(response, nil)
}

func (m *Module) handleFileEditOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ApplyFileEdit(filebiz.FileEditRequest{
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
	result.Set(response, nil)
}

func (m *Module) handlePatchApplyOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecutePatchApplyOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ApplyPatch(filebiz.PatchApplyRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory,
		TargetRef:          command.Data.TargetRef, PatchType: command.Data.PatchType,
		Content: command.Data.Content, ContentPresent: command.Data.ContentPresent,
		Diff: command.Data.Diff, OldText: command.Data.OldText, NewText: command.Data.NewText, NewTextPresent: command.Data.NewTextPresent,
		BaseHash: command.Data.BaseHash, Occurrence: command.Data.Occurrence,
		CoverageTargets: command.Data.CoverageTargets, Summary: command.Data.Summary,
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
	result.Set(response, nil)
}

func (m *Module) handlePatchPreviewOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecutePatchPreviewOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.PreviewPatch(filebiz.PatchApplyRequest{
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
	result.Set(response, nil)
}

func runtimeOperationStatus(err error) runtimecontract.RuntimeOperationExecutionStatus {
	if err != nil {
		return runtimecontract.RuntimeOperationExecutionStatusRejected
	}
	return runtimecontract.RuntimeOperationExecutionStatusAccepted
}

func fileOperationEvidence(value filebiz.RuntimeFileOperationResult) runtimecontract.RuntimeOperationEvidence {
	return runtimecontract.RuntimeOperationEvidence{
		Summary: value.Summary, ContentHash: value.ContentHash, EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
	}
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindList, StepKindRead, StepKindEdit, StepKindPatchApply, StepKindPatchPreview)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(response, nil)
}

func (m *Module) handleAcquireWorkspaceManifest(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireWorkspaceManifestCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	manifest, err := m.runtimeAdapter.AcquireWorkspaceManifest(command.WorkspaceDeliverableDirectory)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(orchestratorevents.AcquireWorkspaceManifestResult{PrerequisiteID: command.PrerequisiteID, Manifest: manifest}, nil)
}

func (m *Module) handleAcquireTargetSnapshot(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireTargetSnapshotCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRef == "" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	snapshot, err := m.runtimeAdapter.AcquireTargetSnapshot(event.Context(), command.WorkspaceDeliverableDirectory, command.TargetRef, command.MaxBytes)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(orchestratorevents.AcquireTargetSnapshotResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot}, nil)
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
