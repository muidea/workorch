package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLITemplate(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLITemplateCreateCommand:
		item, err := CreateWorkspaceTemplateWithSession(ctx, d.borrow(), cliWorkspaceTemplate(command.Template))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateCreateResult{Template: readmodelports.PortTemplateInfo(item)}, nil)
	case workorchdevents.CLITemplateGetCommand:
		item, err := ReadWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateGetResult{Template: readmodelports.PortTemplateInfo(item)}, nil)
	case workorchdevents.CLITemplateListCommand:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateListResult{Templates: readmodelports.PortTemplateInfos(items)}, nil)
	case workorchdevents.CLITemplateUpdateCommand:
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), command.TemplateID, cliWorkspaceTemplate(command.Template))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLITemplateUpdateResult{Template: readmodelports.PortTemplateInfo(item)}, nil)
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

func cliWorkspaceTemplate(template workorchport.TemplateInfo) workspacecore.WorkspaceTemplate {
	bindings := make(map[string]string, len(template.SoulBindings))
	for key, value := range template.SoulBindings {
		bindings[key] = string(value)
	}
	return workspacecore.WorkspaceTemplate{
		ID: template.ID, Name: template.Name, Description: template.Description, GoalText: template.GoalText,
		DoDItems: append([]string(nil), template.DoDItems...), DefaultSoulID: string(template.DefaultSoulID),
		SoulBindings: bindings, EvidencePolicies: workorcheventpayload.MapSlice(template.EvidencePolicies),
	}
}
