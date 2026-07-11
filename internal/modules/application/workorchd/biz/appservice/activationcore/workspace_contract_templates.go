package activationcore

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func MatchingWorkspaceTemplateContextHints(ctx context.Context, templates WorkspaceTemplateReader, workspace *workspacemodel.Workspace) map[string]any {
	if ctx == nil || templates == nil || workspace == nil {
		return nil
	}
	if templateID := strings.TrimSpace(workspace.TemplateID); templateID != "" {
		item, err := templates.Get(ctx, templateID)
		if err == nil && item != nil {
			return cloneContextMapDeep(item.ContextHints)
		}
		return nil
	}
	items, err := templates.List(ctx)
	if err != nil {
		return nil
	}
	var selected map[string]any
	for _, item := range items {
		if item == nil || len(item.ContextHints) == 0 {
			continue
		}
		if !WorkspaceMatchesTemplateRouting(workspace, item) {
			continue
		}
		selected = cloneContextMapDeep(item.ContextHints)
	}
	return selected
}

func WorkspaceMatchesTemplateRouting(workspace *workspacemodel.Workspace, template *repository.WorkspaceTemplate) bool {
	if workspace == nil || template == nil {
		return false
	}
	if strings.TrimSpace(template.DefaultSoulID) == "" && len(template.SoulBindings) == 0 {
		return false
	}
	if string(workspace.DefaultSoulID) != strings.TrimSpace(template.DefaultSoulID) {
		return false
	}
	if len(workspace.SoulBindings) != len(template.SoulBindings) {
		return false
	}
	for stage, templateSoulID := range template.SoulBindings {
		if string(workspace.SoulBindings[stage]) != strings.TrimSpace(templateSoulID) {
			return false
		}
	}
	return true
}
