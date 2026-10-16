package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeValidationRawOutput(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.QueryRuntimeValidationRawOutputCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || command.Validate() != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	// 不提供 found=false 执行旁路，也不接受 caller 提供的 envelope、目录或原文引用。
	query := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), event.Context(), orchestratorevents.QueryRuntimeOperationReceiptCommand{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, DispatchID: command.DispatchID,
		OperationID: command.OperationID, ExecutionKey: command.ExecutionKey, Fence: command.Fence,
	})
	query.BindLaneKey(eventcontract.QueryLaneKey(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID), SubjectID: command.ExecutionKey}))
	response := m.SendEvent(query)
	if response == nil {
		setCapabilityError(result, fmt.Errorf("raw output source query returned no result"))
		return
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		setCapabilityError(result, resultErr)
		return
	}
	source, ok := data.(orchestratorevents.QueryRuntimeOperationReceiptResult)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	receipt, found, err := orchestratorevents.ValidateQueryRuntimeOperationReceiptResult(source, command.ExecutionKey, command.Fence, command.OperationID)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	// rejected managed validation 的 effect 仍可能是 unknown；仅允许读取原诊断，绝不借此清除 unknown 或授权重执行。
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash || receipt.Evidence.Validation == nil ||
		(receipt.ObservedEffectScope != runtimecontract.RuntimeEffectEphemeralSession &&
			!(receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected && receipt.ObservedEffectScope == runtimecontract.RuntimeEffectUnknown)) ||
		(receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted && receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected) ||
		!shellProjectObservationProtocol(receipt.Evidence.Validation.ObservationProtocolRef) || receipt.Evidence.Validation.ExecutionConjunction == nil {
		setCapabilityError(result, fmt.Errorf("%w: raw output requires exact persisted project validation receipt", common.ErrConflict))
		return
	}
	var identity *runtimecontract.RuntimeValidationOutputIdentity
	for _, stage := range receipt.Evidence.Validation.ExecutionConjunction.Stages {
		if stage.Identity.ID == command.StageID {
			value := stage.Stdout.Clone()
			if command.Stream == "stderr" {
				value = stage.Stderr.Clone()
			}
			identity = &value
			break
		}
	}
	if identity == nil || identity.Storage == nil || identity.Storage.Reference == "" {
		setCapabilityError(result, fmt.Errorf("%w: original receipt has no saved output for selected stream", common.ErrConflict))
		return
	}
	workspaceCommand := workspaceevents.GetWorkspaceCommand{WorkspaceID: command.WorkspaceID}
	workspaceQuery := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), event.Context(), workspaceCommand)
	workspaceQuery.BindLaneKey(eventcontract.QueryLaneKey(workspaceevents.TopicWorkspaceCommandGet, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	workspaceResponse := m.SendEvent(workspaceQuery)
	if workspaceResponse == nil {
		setCapabilityError(result, fmt.Errorf("raw output workspace query returned no result"))
		return
	}
	workspaceData, workspaceErr := workspaceResponse.Get()
	if workspaceErr != nil {
		setCapabilityError(result, workspaceErr)
		return
	}
	workspace, ok := workspaceData.(workspaceevents.WorkspaceResult)
	if !ok || workspace.ValidateFor(workspaceCommand) != nil {
		setCapabilityError(result, common.ErrConflict)
		return
	}
	store, err := openShellProjectRawStore(workspace.Workspace.InternalDirectory)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	raw, readErr := store.Read(event.Context(), shellProjectRawScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID, ExecutionKey: receipt.ExecutionKey, StageID: command.StageID, Stream: command.Stream}, identity.Storage.Reference)
	closeErr := store.Close()
	if readErr != nil {
		setCapabilityError(result, readErr)
		return
	}
	if closeErr != nil {
		setCapabilityError(result, closeErr)
		return
	}
	expected := identity.Clone()
	expected.Storage = nil
	if shellProjectOutputIdentity(raw) != expected {
		setCapabilityError(result, fmt.Errorf("%w: saved raw output does not match original receipt", common.ErrConflict))
		return
	}
	digest := sha256.Sum256(raw.Raw)
	output := shellevents.QueryRuntimeValidationRawOutputResult{SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream, Identity: *identity, SavedSHA256: hex.EncodeToString(digest[:]), Raw: raw.Raw}
	if err := output.ValidateFor(command); err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(output.Clone(), nil)
}
