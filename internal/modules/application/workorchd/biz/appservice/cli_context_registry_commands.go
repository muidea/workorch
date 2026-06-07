package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleContextRegistryCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandSoulCreate:
		item, err := CreateSoulWithSession(ctx, d.borrow(), *clicore.DecodeOrZero[CreateSoulCommand](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandSoulGet:
		item, err := ReadSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandSoulList:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case events.CommandSoulUpdate:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), cmd.SubjectID, *clicore.DecodeOrZero[UpdateSoulCommand](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandSoulDelete:
		err := DeleteSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case events.CommandSkillCreate:
		item, err := CreateSkillWithSession(ctx, d.borrow(), *clicore.DecodeOrZero[CreateSkillCommand](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandSkillGet:
		item, err := ReadSkillWithSession(ctx, d.borrow(), cmd.SubjectID, SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return clicore.ResourceMap(item), nil, err
	case events.CommandSkillList:
		items, err := ListSkillsWithSession(ctx, d.borrow(), SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, clicore.ResourceMaps(items), err
	case events.CommandMCPServerCreate:
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), *clicore.DecodeOrZero[CreateMCPServerCommand](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandMCPServerGet:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandMCPServerList:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case events.CommandMCPServerUpdate:
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID, *clicore.DecodeOrZero[UpdateMCPServerCommand](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandMCPCapabilityList:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), MCPCapabilityQuery{})
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}
