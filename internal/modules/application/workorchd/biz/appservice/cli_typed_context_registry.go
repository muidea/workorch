package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLIContextRegistry(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLISoulCreateCommand:
		item, err := CreateSoulWithSession(ctx, d.borrow(), workorchmanagementcontract.Soul{ID: string(command.Soul.ID), Name: command.Soul.Name, Description: command.Soul.Description, BusinessDomains: append([]string(nil), command.Soul.BusinessDomains...), SoulMD: command.Soul.SoulMD})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulCreateResult{Soul: port.SoulInfoFromManagement(item)}, nil)
	case workorchdevents.CLISoulGetCommand:
		item, err := ReadSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulGetResult{Soul: port.SoulInfoFromManagement(item)}, nil)
	case workorchdevents.CLISoulListCommand:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulListResult{Souls: port.SoulInfosFromManagement(items)}, nil)
	case workorchdevents.CLISoulUpdateCommand:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), string(command.SoulID), workorchmanagementcontract.Soul{ID: string(command.Soul.ID), Name: command.Soul.Name, Description: command.Soul.Description, BusinessDomains: append([]string(nil), command.Soul.BusinessDomains...), SoulMD: command.Soul.SoulMD})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulUpdateResult{Soul: port.SoulInfoFromManagement(item)}, nil)
	case workorchdevents.CLISoulDeleteCommand:
		err := DeleteSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLISkillCreateCommand:
		item, err := CreateSkillWithSession(ctx, d.borrow(), workorchmanagementcontract.Skill{ID: string(command.Skill.ID), Name: command.Skill.Name, Version: command.Skill.Version, Description: command.Skill.Description, Instruction: command.Skill.Instruction})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillCreateResult{Skill: port.SkillInfoFromManagement(item)}, nil)
	case workorchdevents.CLISkillGetCommand:
		item, err := ReadSkillWithSession(ctx, d.borrow(), string(command.SkillID), contextregistrycore.SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillGetResult{Skill: port.SkillInfoFromManagement(item)}, nil)
	case workorchdevents.CLISkillListCommand:
		items, err := ListSkillsWithSession(ctx, d.borrow(), contextregistrycore.SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillListResult{Skills: port.SkillInfosFromManagement(items)}, nil)
	case workorchdevents.CLIMCPServerCreateCommand:
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), workorchmanagementcontract.MCPServer{ID: string(command.Server.ID), Name: command.Server.Name, Transport: command.Server.Transport, Endpoint: command.Server.Endpoint, Enabled: command.Server.Enabled})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerCreateResult{Server: port.MCPServerInfoFromManagement(item)}, nil)
	case workorchdevents.CLIMCPServerGetCommand:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), string(command.ServerID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerGetResult{Server: port.MCPServerInfoFromManagement(item)}, nil)
	case workorchdevents.CLIMCPServerListCommand:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerListResult{Servers: port.MCPServerInfosFromManagement(items)}, nil)
	case workorchdevents.CLIMCPServerUpdateCommand:
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), string(command.ServerID), workorchmanagementcontract.MCPServer{ID: string(command.Server.ID), Name: command.Server.Name, Transport: command.Server.Transport, Endpoint: command.Server.Endpoint, Enabled: command.Server.Enabled})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerUpdateResult{Server: port.MCPServerInfoFromManagement(item)}, nil)
	case workorchdevents.CLIMCPCapabilityListCommand:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), contextregistrycore.MCPCapabilityQuery{})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPCapabilityListResult{Capabilities: port.MCPCapabilityInfosFromManagement(items)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
