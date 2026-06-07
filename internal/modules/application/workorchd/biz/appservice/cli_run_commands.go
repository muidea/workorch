package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleRunCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandRunActivate:
		item, err := ActivateWorkspaceExecutionWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.ActivateWorkspaceExecutionCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item.Run), nil, err
	case events.CommandRunResume:
		item, err := ResumeRunWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.MapValue(cmd.Payload, "executionContextPatch"))
		return clicore.ResourceMap(item), nil, err
	case events.CommandRunGet:
		item, err := ReadRunWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandRunList:
		items, err := ListRunsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, clicore.ResourceMaps(items), err
	case events.CommandRunStatus:
		item, err := ReadRunStatusWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}
