package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 具体 owner query 留在 EventHub adapter；此入口不写 aggregate，也不授 stale/reopen 或 credit。
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
