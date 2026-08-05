package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
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
		dto, err := workorchdeventmapping.TemplateDTO(item)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateCreateResult{Template: dto}, nil)
	case workorchdevents.CLITemplateGetCommand:
		item, err := ReadWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID)
		if setTypedCLIError(result, err) {
			return
		}
		dto, err := workorchdeventmapping.TemplateDTO(item)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateGetResult{Template: dto}, nil)
	case workorchdevents.CLITemplateListCommand:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		dtos, err := workorchdeventmapping.TemplateDTOs(items)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateListResult{Templates: dtos}, nil)
	case workorchdevents.CLITemplateUpdateCommand:
		template, mapErr := cliWorkspaceTemplate(command.Template)
		if setTypedCLIError(result, mapErr) {
			return
		}
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID, template)
		if setTypedCLIError(result, err) {
			return
		}
		dto, err := workorchdeventmapping.TemplateDTO(item)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateUpdateResult{Template: dto}, nil)
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
	return workorchdeventmapping.TemplateManagement(template)
}
