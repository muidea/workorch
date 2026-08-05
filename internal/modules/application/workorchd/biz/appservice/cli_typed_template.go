package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLITemplate(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLITemplateCreateCommand:
		template, mapErr := cliWorkspaceTemplate(command.Template)
		if setTypedCLIError(result, mapErr) {
			return
		}
		item, err := CreateWorkspaceTemplateWithSession(ctx, d.borrow(), template)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateCreateResult{Template: workorchdeventmapping.TemplateDTO(port.TemplateInfoFromManagement(item))}, nil)
	case workorchdevents.CLITemplateGetCommand:
		item, err := ReadWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateGetResult{Template: workorchdeventmapping.TemplateDTO(port.TemplateInfoFromManagement(item))}, nil)
	case workorchdevents.CLITemplateListCommand:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateListResult{Templates: workorchdeventmapping.TemplateDTOs(port.TemplateInfosFromManagement(items))}, nil)
	case workorchdevents.CLITemplateUpdateCommand:
		template, mapErr := cliWorkspaceTemplate(command.Template)
		if setTypedCLIError(result, mapErr) {
			return
		}
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID, template)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateUpdateResult{Template: workorchdeventmapping.TemplateDTO(port.TemplateInfoFromManagement(item))}, nil)
	case workorchdevents.CLITemplateDeleteCommand:
		err := DeleteWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateDeleteResult{Deleted: true}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}

func cliWorkspaceTemplate(template workorchdevents.CLITemplateInfo) (workorchmanagementcontract.WorkspaceTemplate, error) {
	portTemplate, err := workorchdeventmapping.TemplatePort(template)
	if err != nil {
		return workorchmanagementcontract.WorkspaceTemplate{}, err
	}
	return workorchmanagementcontract.WorkspaceTemplate{
		ID: portTemplate.ID, Name: portTemplate.Name, Description: portTemplate.Description, GoalText: portTemplate.GoalText,
		DoDItems: append([]string(nil), portTemplate.DoDItems...), DefaultSoulID: string(portTemplate.DefaultSoulID),
		SoulBindings: portTemplate.SoulBindings, ContextHints: portTemplate.ContextHints.Map(), EvidencePolicies: workorcheventpayload.MapSlice(portTemplate.EvidencePolicies),
	}, nil
}
