package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) queryRuntimeShellApplicabilityFromAggregate(ctx context.Context, aggregate runtime.RuntimeAggregate, dispatchID, operationID, receiptHash, requestID string) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if m == nil || m.EventHub() == nil {
		return nil, false, common.ErrInvalidArgument
	}
	source, err := runtime.CompileRuntimeOperationReadSource(aggregate, dispatchID, operationID, receiptHash)
	if err != nil {
		return nil, false, err
	}
	return m.queryRuntimeShellApplicabilityFromSource(ctx, source, aggregate.StateRevision, requestID)
}

func (m *Biz) queryRuntimeShellApplicabilityFromSource(ctx context.Context, source runtime.RuntimeOperationReadSource, stateRevision uint64, requestID string) (*runtimecontract.RuntimeOwnerRevisionSet, bool, error) {
	if m == nil || m.EventHub() == nil {
		return nil, false, common.ErrInvalidArgument
	}
	command := shellevents.QueryRuntimeValidationReadDependenciesCommand{
		RuntimeOperationExecutionContext: source.Context, Operation: source.Operation,
		RequestID: requestID, IssuedAtStateRevision: stateRevision,
		SourceReceiptHash: source.SourceReceiptHash, OriginalDependencies: source.OriginalDependencies, ApplicabilitySource: source.ApplicabilitySource,
	}
	if err := command.Validate(); err != nil {
		return nil, false, err
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	event := magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationReadDependencies, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(eventcontract.QueryLaneKey(shellevents.TopicQueryRuntimeValidationReadDependencies, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID), SubjectID: command.RequestID}))
	response := m.SendEvent(event)
	if response == nil {
		return nil, false, fmt.Errorf("Shell dependency query returned no result")
	}
	data, queryErr := response.Get()
	if queryErr != nil {
		return nil, false, queryErr
	}
	result, ok := data.(shellevents.QueryRuntimeValidationReadDependenciesResult)
	if !ok {
		return nil, false, fmt.Errorf("Shell dependency query returned invalid type %T", data)
	}
	return result.ValidateFor(command)
}
