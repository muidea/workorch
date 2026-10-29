package orchestrator

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 只从持久前像重编 query；原 request、当前 decision 与 caller directory 都不是来源权威。
func (m *Biz) queryRuntimeFileReadDependenciesFromAggregate(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID string) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if requestID == "" || requestID != strings.TrimSpace(requestID) {
		return nil, false, common.ErrInvalidArgument
	}
	source, err := runtime.CompileRuntimeOperationReadSource(aggregate, dispatchID, operationID, receiptHash)
	if err != nil {
		return nil, false, err
	}
	if runtimecontract.CanonicalRuntimeBlockKind(source.Operation.Kind) != runtimecontract.CanonicalRuntimeBlockKind(runtimecontract.StepKindFileRead) {
		return nil, false, common.ErrInvalidArgument
	}
	sealed, err := fileevents.SealRuntimeOperation(source.Operation)
	if err != nil {
		return nil, false, err
	}
	return m.queryRuntimeFileReadDependencies(ctx, fileevents.QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: source.Context, SealedOperation: sealed,
		RequestID: requestID, IssuedAtStateRevision: aggregate.StateRevision,
		SourceReceiptHash: source.SourceReceiptHash, OriginalDependencies: source.OriginalDependencies,
	})
}

// 具体 owner query 留在 EventHub adapter；此入口不写 aggregate，也不授 stale/reopen 或 credit。
func (m *Biz) queryRuntimeFileMutationApplicabilityFromAggregate(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID string) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if requestID == "" || requestID != strings.TrimSpace(requestID) {
		return nil, false, common.ErrInvalidArgument
	}
	source, err := runtime.CompileRuntimeOperationReadSource(aggregate, dispatchID, operationID, receiptHash)
	if err != nil {
		return nil, false, err
	}
	return m.queryRuntimeFileMutationApplicabilityFromSource(ctx, source, aggregate.StateRevision, requestID)
}

// Only same-call complete aggregate compilers supply this detached owner read.
// The receiving File owner still verifies the exact original receipt and fence.
func (m *Biz) queryRuntimeFileMutationApplicabilityFromSource(ctx context.Context, source runtime.RuntimeOperationReadSource, stateRevision uint64, requestID string) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if requestID == "" || requestID != strings.TrimSpace(requestID) {
		return nil, false, common.ErrInvalidArgument
	}
	if source.ApplicabilitySource == nil {
		return nil, false, fmt.Errorf("File mutation has no original versioned applicability source")
	}
	switch source.Operation.Kind {
	case runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply:
	default:
		return nil, false, common.ErrInvalidArgument
	}
	sealed, err := fileevents.SealRuntimeOperation(source.Operation)
	if err != nil {
		return nil, false, err
	}
	return m.queryRuntimeFileReadDependencies(ctx, fileevents.QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: source.Context, SealedOperation: sealed,
		RequestID: requestID, IssuedAtStateRevision: stateRevision,
		SourceReceiptHash: source.SourceReceiptHash, OriginalDependencies: source.OriginalDependencies,
		ApplicabilitySource: runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(source.ApplicabilitySource),
	})
}

func (m *Biz) queryRuntimeFileReadDependencies(ctx context.Context, command fileevents.QueryRuntimeFileReadDependenciesCommand) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if m == nil || m.EventHub() == nil {
		return nil, false, common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, command.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	command.RuntimeOperationExecutionContext = runtimecontract.CloneRuntimeOperationExecutionContext(command.RuntimeOperationExecutionContext)
	command.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
	command.ApplicabilitySource = runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(command.ApplicabilitySource)
	command.WorkspaceDeliverableDirectory = workspace.DeliverableDirectory
	command.WorkspaceInternalDirectory = workspace.InternalDirectory
	event := magicevent.NewEventWithContext(fileevents.TopicQueryRuntimeFileReadDependencies, ModuleID, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(eventcontract.QueryLaneKey(fileevents.TopicQueryRuntimeFileReadDependencies, eventcontract.LaneScope{
		WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID), SubjectID: command.RequestID,
	}))
	result := m.SendEvent(event)
	if result == nil {
		return nil, false, fmt.Errorf("File read dependency query returned no result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, false, resultErr
	}
	response, ok := data.(fileevents.QueryRuntimeFileReadDependenciesResult)
	if !ok {
		return nil, false, fmt.Errorf("File read dependency query returned invalid result type %T", data)
	}
	return fileevents.ValidateRuntimeFileReadDependenciesResult(command, response)
}
