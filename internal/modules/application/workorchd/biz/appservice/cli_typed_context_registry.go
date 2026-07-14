package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func (d *CLICommandDispatcher) handleTypedCLIContextRegistry(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.commandHub)
	switch command := event.Data().(type) {
	case workorchdevents.CLISoulCreateCommand:
		item, err := CreateSoulWithSession(ctx, d.borrow(), contextregistrycore.Soul{ID: string(command.Soul.ID), Name: command.Soul.Name, Description: command.Soul.Description, BusinessDomains: append([]string(nil), command.Soul.BusinessDomains...), SoulMD: command.Soul.SoulMD})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulCreateResult{Soul: readmodelports.PortSoulInfo(item)}, nil)
	case workorchdevents.CLISoulGetCommand:
		item, err := ReadSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulGetResult{Soul: readmodelports.PortSoulInfo(item)}, nil)
	case workorchdevents.CLISoulListCommand:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulListResult{Souls: readmodelports.PortSoulInfos(items)}, nil)
	case workorchdevents.CLISoulUpdateCommand:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), string(command.SoulID), contextregistrycore.Soul{ID: string(command.Soul.ID), Name: command.Soul.Name, Description: command.Soul.Description, BusinessDomains: append([]string(nil), command.Soul.BusinessDomains...), SoulMD: command.Soul.SoulMD})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulUpdateResult{Soul: readmodelports.PortSoulInfo(item)}, nil)
	case workorchdevents.CLISoulDeleteCommand:
		err := DeleteSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLISkillCreateCommand:
		item, err := CreateSkillWithSession(ctx, d.borrow(), contextregistrycore.Skill{ID: string(command.Skill.ID), Name: command.Skill.Name, Version: command.Skill.Version, Description: command.Skill.Description, Instruction: command.Skill.Instruction})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillCreateResult{Skill: readmodelports.PortSkillInfo(item)}, nil)
	case workorchdevents.CLISkillGetCommand:
		item, err := ReadSkillWithSession(ctx, d.borrow(), string(command.SkillID), SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillGetResult{Skill: readmodelports.PortSkillInfo(item)}, nil)
	case workorchdevents.CLISkillListCommand:
		items, err := ListSkillsWithSession(ctx, d.borrow(), SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillListResult{Skills: readmodelports.PortSkillInfos(items)}, nil)
	case workorchdevents.CLIMCPServerCreateCommand:
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), contextregistrycore.MCPServer{ID: string(command.Server.ID), Name: command.Server.Name, Transport: command.Server.Transport, Endpoint: command.Server.Endpoint, Enabled: command.Server.Enabled})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerCreateResult{Server: readmodelports.PortMCPServerInfo(item)}, nil)
	case workorchdevents.CLIMCPServerGetCommand:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), string(command.ServerID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerGetResult{Server: readmodelports.PortMCPServerInfo(item)}, nil)
	case workorchdevents.CLIMCPServerListCommand:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerListResult{Servers: readmodelports.PortMCPServerInfos(items)}, nil)
	case workorchdevents.CLIMCPServerUpdateCommand:
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), string(command.ServerID), contextregistrycore.MCPServer{ID: string(command.Server.ID), Name: command.Server.Name, Transport: command.Server.Transport, Endpoint: command.Server.Endpoint, Enabled: command.Server.Enabled})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerUpdateResult{Server: readmodelports.PortMCPServerInfo(item)}, nil)
	case workorchdevents.CLIMCPCapabilityListCommand:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), MCPCapabilityQuery{})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPCapabilityListResult{Capabilities: readmodelports.PortMCPCapabilityInfos(items)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
