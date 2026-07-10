package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
)

func (d *CLICommandDispatcher) handleContextRegistryCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandSoulCreate:
		item, err := CreateSoulWithSession(ctx, d.borrow(), clicore.SoulCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandSoulGet:
		item, err := ReadSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandSoulList:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandSoulUpdate:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.SoulCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandSoulDelete:
		err := DeleteSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case clicore.CommandSkillCreate:
		item, err := CreateSkillWithSession(ctx, d.borrow(), clicore.SkillCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandSkillGet:
		item, err := ReadSkillWithSession(ctx, d.borrow(), cmd.SubjectID, SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandSkillList:
		items, err := ListSkillsWithSession(ctx, d.borrow(), SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandMCPServerCreate:
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), clicore.MCPServerCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMCPServerGet:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMCPServerList:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandMCPServerUpdate:
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.MCPServerCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMCPCapabilityList:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), MCPCapabilityQuery{})
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}
