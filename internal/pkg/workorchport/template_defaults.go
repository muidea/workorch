package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

var genericDoDItems = []string{"目标已完成，输出包含明确的执行结果"}

// DefaultDoDItemsForWorkspace returns the Workspace template DoD when the
// Workspace's persisted Soul routing matches a template initialized at create
// time. It deliberately does not mutate Workspace bindings or infer template
// state at Run activation.
func DefaultDoDItemsForWorkspace(ctx context.Context, be Backend, wsID common.WorkspaceID) []string {
	items, ok := TemplateDoDItemsForWorkspace(ctx, be, wsID)
	if ok {
		return items
	}
	return append([]string(nil), genericDoDItems...)
}

func TemplateDoDItemsForWorkspace(ctx context.Context, be Backend, wsID common.WorkspaceID) ([]string, bool) {
	if be == nil || wsID == "" {
		return nil, false
	}
	ws, err := be.Workspace().Get(ctx, wsID)
	if err != nil || ws == nil {
		return nil, false
	}
	templates, err := be.Template().List(ctx)
	if err != nil {
		return nil, false
	}
	for _, template := range templates {
		if len(template.DoDItems) == 0 {
			continue
		}
		if workspaceMatchesTemplate(*ws, template) {
			return append([]string(nil), template.DoDItems...), true
		}
	}
	return nil, false
}

func workspaceMatchesTemplate(ws projectioncontract.Workspace, template workorchmanagementcontract.WorkspaceTemplate) bool {
	if ws.TemplateID != "" {
		return ws.TemplateID == template.ID
	}
	if template.DefaultSoulID != "" && string(ws.DefaultSoulID) != template.DefaultSoulID {
		return false
	}
	if len(template.SoulBindings) == 0 {
		return template.DefaultSoulID != ""
	}
	if len(ws.SoulBindings) == 0 {
		return false
	}
	for stage, templateSoulID := range template.SoulBindings {
		if templateSoulID == "" {
			continue
		}
		if string(ws.SoulBindings[stage]) != templateSoulID {
			return false
		}
	}
	return true
}
