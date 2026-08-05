package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLIRun(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIRunGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadRunWithSession(ctx, d.borrow(), string(command.RunID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIRunGetResult{Run: readmodelports.PortRunInfo(item)}, nil)
	case workorchdevents.CLIRunListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListRunsWithSession(ctx, d.borrow(), string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIRunListResult{Runs: readmodelports.PortRunInfos(items)}, nil)
	case workorchdevents.CLIRunStatusCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadRunStatusWithSession(ctx, d.borrow(), string(command.RunID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIRunStatusResult{Status: port.RunStatusInfoFromProjection(item)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
