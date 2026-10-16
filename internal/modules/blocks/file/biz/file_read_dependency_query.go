package file

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeFileReadDependenciesQuery(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setPrerequisiteError(result, fmt.Errorf("File read dependency query requires exact Orchestrator source"))
		return
	}
	command, ok := event.Data().(fileevents.QueryRuntimeFileReadDependenciesCommand)
	if !ok || m.runtimeAdapter == nil || command.RequestID == "" || command.RequestID != strings.TrimSpace(command.RequestID) ||
		command.IssuedAtStateRevision == 0 || command.SourceReceiptHash == "" || command.SourceReceiptHash != strings.TrimSpace(command.SourceReceiptHash) ||
		command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision) {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if command.ApplicabilitySource != nil {
		m.handleRuntimeFileMutationApplicabilityQuery(event, result, command)
		return
	}
	operation, err := validateSealedRuntimeFileOperation(event, command.RuntimeOperationExecutionContext, command.SealedOperation, runtimecontract.StepKindFileRead)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	data, err := fileevents.FileReadOperationDataFromRuntimeOperation(operation)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	original := command.OriginalDependencies
	if !reflect.DeepEqual(original, original.Normalized()) {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if err := original.Validate(); err != nil || original.DefinitionRevision != command.Transaction.DefinitionRevision || len(original.Entries) != 1 {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	entry := original.Entries[0]
	if entry.OwnerKind != "capability" || entry.OwnerInstanceID != ModuleID+"/"+string(command.WorkspaceID) ||
		entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) || entry.ResourceID != data.Path {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	// Receipt miss 不授予此 query 执行权限；完整 source 在 Orchestrator 原 aggregate 反查。
	receipt, found, err := m.queryRuntimeOperationReceiptWithSource(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution, &command.RuntimeOperationExecutionContext)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash ||
		receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone ||
		!reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &original) || !reflect.DeepEqual(receipt.TargetRefs, operation.TargetRefs) {
		setPrerequisiteError(result, fmt.Errorf("%w: File read dependency source receipt mismatch", common.ErrConflict))
		return
	}
	response := fileevents.QueryRuntimeFileReadDependenciesResult{
		RequestID: command.RequestID, IssuedAtStateRevision: command.IssuedAtStateRevision, SourceReceiptHash: command.SourceReceiptHash,
	}
	// 工作区缺失不是 target absence；非普通对象也不进入流式读取。
	root, err := os.Stat(command.WorkspaceDeliverableDirectory)
	if err != nil || !root.IsDir() {
		if err == nil {
			err = common.ErrInvalidArgument
		}
		setPrerequisiteError(result, err)
		return
	}
	_, target, _, err := guardedWorkspacePath(command.WorkspaceDeliverableDirectory, data.Path, false)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	info, err := os.Stat(target)
	if err != nil && !os.IsNotExist(err) {
		setPrerequisiteError(result, err)
		return
	}
	if err == nil && !info.Mode().IsRegular() {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	// 重新运行原有界 selector，不把旧 DependencyHash echo 成 current observation。
	observed, readErr := m.runtimeAdapter.ReadFile(event.Context(), FileReadRequest{
		WorkspaceDirectory: command.WorkspaceDeliverableDirectory, Path: data.Path, Optional: data.Optional,
		MaxBytes: data.MaxBytes, StartLine: data.StartLine, EndLine: data.EndLine,
	})
	if os.IsNotExist(readErr) || (readErr == nil && !observed.Exists) {
		response.Status = fileevents.RuntimeFileReadDependencyMissing
		result.Set(response, nil)
		return
	}
	if readErr != nil {
		setPrerequisiteError(result, readErr)
		return
	}
	current, err := fileReadOwnerDependencies(fileevents.ExecuteFileReadOperationCommand{
		RuntimeOperationExecutionContext: command.RuntimeOperationExecutionContext, SealedOperation: command.SealedOperation,
	}, observed, data.StartLine, data.EndLine, data.MaxBytes)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Status = fileevents.RuntimeFileReadDependencyObserved
	response.CurrentDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(&current)
	result.Set(response, nil)
}

// This branch is opt-in through the original versioned receipt proof. An old
// file/read query still uses its exact range/bound selector above; no archived
// mutation is silently upgraded to the new observation protocol.
func (m *Biz) handleRuntimeFileMutationApplicabilityQuery(event magicevent.Event, result magicevent.Result, command fileevents.QueryRuntimeFileReadDependenciesCommand) {
	kind := command.SealedOperation.Operation.Kind
	if kind != runtimecontract.StepKindFileEdit && kind != runtimecontract.StepKindFileWrite && kind != runtimecontract.RuntimeBlockKindPatchApply {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	operation, err := validateSealedRuntimeFileOperation(event, command.RuntimeOperationExecutionContext, command.SealedOperation, kind)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if err := runtimecontract.ValidateRuntimeEvidenceDependencyPair(&command.OriginalDependencies, command.ApplicabilitySource, command.Transaction.DefinitionRevision); err != nil {
		setPrerequisiteError(result, err)
		return
	}
	original := command.ApplicabilitySource.OwnerRevisionSet
	if len(operation.TargetRefs) != 1 || len(original.Entries) != 1 {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	entry := original.Entries[0]
	if entry.OwnerKind != "capability" || entry.OwnerInstanceID != ModuleID+"/"+string(command.WorkspaceID) || entry.ResourceKind != "workspace_target_content" || entry.ResourceID != operation.TargetRefs[0] {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	receipt, found, err := m.queryRuntimeOperationReceiptWithSource(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution, &command.RuntimeOperationExecutionContext)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash ||
		(receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted && receipt.Outcome != runtimecontract.RuntimeOperationOutcomeNoOp) ||
		!reflect.DeepEqual(receipt.TargetRefs, operation.TargetRefs) || !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &command.OriginalDependencies) ||
		!reflect.DeepEqual(receipt.Evidence.ApplicabilityDependencies, command.ApplicabilitySource) {
		setPrerequisiteError(result, fmt.Errorf("%w: File applicability source receipt mismatch", common.ErrConflict))
		return
	}
	response := fileevents.QueryRuntimeFileReadDependenciesResult{RequestID: command.RequestID, IssuedAtStateRevision: command.IssuedAtStateRevision, SourceReceiptHash: command.SourceReceiptHash}
	root, err := os.Stat(command.WorkspaceDeliverableDirectory)
	if err != nil || !root.IsDir() {
		if err == nil {
			err = common.ErrInvalidArgument
		}
		setPrerequisiteError(result, err)
		return
	}
	unlock := m.runtimeAdapter.lockWorkspaceEffect(command.WorkspaceDeliverableDirectory)
	defer unlock()
	_, target, _, err := guardedWorkspacePath(command.WorkspaceDeliverableDirectory, operation.TargetRefs[0], false)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		response.Status = fileevents.RuntimeFileReadDependencyMissing
		result.Set(response, nil)
		return
	}
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	if !info.Mode().IsRegular() {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	// Hash the entire actual stream while retaining at most one byte locally.
	// Size comes from that stream, not a potentially stale stat or old receipt.
	scan, err := scanBoundedFile(event.Context(), target, 0, 0, 1)
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	proof, err := filePatchApplicabilityDependencies(command.RuntimeOperationExecutionContext, operation, &filePatchReadObservation{
		TargetRef: operation.TargetRefs[0], ContentHash: scan.ContentHash, Size: scan.Size, Purpose: "mutation_postcondition",
	})
	if err != nil {
		setPrerequisiteError(result, err)
		return
	}
	response.Status = fileevents.RuntimeFileReadDependencyObserved
	response.CurrentDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(&proof.OwnerRevisionSet)
	result.Set(response, nil)
}
