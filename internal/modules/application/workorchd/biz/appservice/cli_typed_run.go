package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
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
		result.Set(workorchdevents.CLIRunGetResult{Run: appmodelcore.ProjectRun(item)}, nil)
	case workorchdevents.CLIRunListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListRunsWithSession(ctx, d.borrow(), string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIRunListResult{Runs: appmodelcore.ProjectRuns(items)}, nil)
	case workorchdevents.CLIRunStatusCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadRunStatusWithSession(ctx, d.borrow(), string(command.RunID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIRunStatusResult{Status: projectioncontract.CloneRunStatus(item)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
