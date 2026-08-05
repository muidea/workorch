package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func (d *CLICommandDispatcher) handleTypedCLIContextRegistry(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLISoulCreateCommand:
		item, err := CreateSoulWithSession(ctx, d.borrow(), command.Soul)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulCreateResult{Soul: item}, nil)
	case workorchdevents.CLISoulGetCommand:
		item, err := ReadSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulGetResult{Soul: item}, nil)
	case workorchdevents.CLISoulListCommand:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulListResult{Souls: items}, nil)
	case workorchdevents.CLISoulUpdateCommand:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), string(command.SoulID), command.Soul)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulUpdateResult{Soul: item}, nil)
	case workorchdevents.CLISoulDeleteCommand:
		err := DeleteSoulWithSession(ctx, d.borrow(), string(command.SoulID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISoulDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLISkillCreateCommand:
		item, err := CreateSkillWithSession(ctx, d.borrow(), command.Skill)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillCreateResult{Skill: item}, nil)
	case workorchdevents.CLISkillGetCommand:
		item, err := ReadSkillWithSession(ctx, d.borrow(), string(command.SkillID), contextregistrycore.SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillGetResult{Skill: item}, nil)
	case workorchdevents.CLISkillListCommand:
		items, err := ListSkillsWithSession(ctx, d.borrow(), contextregistrycore.SkillQuery{WorkspaceID: string(command.WorkspaceID)})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLISkillListResult{Skills: items}, nil)
	case workorchdevents.CLIMCPServerCreateCommand:
		mutation, err := workorchdeventmapping.MCPServerMutation(command.Server)
		if setTypedCLIError(result, err) {
			return
		}
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), mutation)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerCreateResult{Server: workorchdeventmapping.MCPServerDTO(item)}, nil)
	case workorchdevents.CLIMCPServerGetCommand:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), string(command.ServerID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerGetResult{Server: workorchdeventmapping.MCPServerDTO(item)}, nil)
	case workorchdevents.CLIMCPServerListCommand:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerListResult{Servers: workorchdeventmapping.MCPServerDTOs(items)}, nil)
	case workorchdevents.CLIMCPServerUpdateCommand:
		mutation, err := workorchdeventmapping.MCPServerMutation(command.Server)
		if setTypedCLIError(result, err) {
			return
		}
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), string(command.ServerID), mutation)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPServerUpdateResult{Server: workorchdeventmapping.MCPServerDTO(item)}, nil)
	case workorchdevents.CLIMCPCapabilityListCommand:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), contextregistrycore.MCPCapabilityQuery{})
		if setTypedCLIError(result, err) {
			return
		}
		capabilities, err := workorchdeventmapping.MCPCapabilityDTOs(items)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIMCPCapabilityListResult{Capabilities: capabilities}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
