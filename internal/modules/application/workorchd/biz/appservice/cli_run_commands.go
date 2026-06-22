package appservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleRunCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	if err := validateCLIRunCommandScope(cmd); err != nil {
		return nil, nil, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
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

func validateCLIRunCommandScope(cmd events.CLICommandPayload) error {
	if cmd.WorkspaceID == "" {
		return fmt.Errorf("%w: run command %s requires workspace scope", common.ErrInvalidArgument, cmd.Command)
	}
	switch cmd.Command {
	case events.CommandRunActivate, events.CommandRunList:
		return nil
	case events.CommandRunResume, events.CommandRunGet, events.CommandRunStatus:
		if strings.TrimSpace(cmd.SubjectID) == "" {
			return fmt.Errorf("%w: run command %s requires run subject id", common.ErrInvalidArgument, cmd.Command)
		}
		if cmd.RunID != "" && string(cmd.RunID) != cmd.SubjectID {
			return fmt.Errorf("%w: run command %s run scope %s does not match subject %s", common.ErrConflict, cmd.Command, cmd.RunID, cmd.SubjectID)
		}
		return nil
	default:
		return nil
	}
}
