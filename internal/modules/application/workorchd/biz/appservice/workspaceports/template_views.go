package workspaceports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListWorkspaceTemplates(ctx context.Context, hub events.Hub) ([]workspacecore.WorkspaceTemplate, error) {
	items, err := ListWorkspaceTemplateRecords(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret := make([]workspacecore.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, workspacecore.WorkspaceTemplateFromRepository(item))
	}
	return ret, nil
}

func ReadWorkspaceTemplate(ctx context.Context, hub events.Hub, templateID string) (workspacecore.WorkspaceTemplate, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return workspacecore.WorkspaceTemplate{}, common.ErrInvalidArgument
	}
	item, err := ReadWorkspaceTemplateRecord(ctx, hub, templateID)
	if err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	return workspacecore.WorkspaceTemplateFromRepository(item), nil
}

func RefreshWorkspaceTemplateBinding(ctx context.Context, hub events.Hub, workspaceID string) (appmodelcore.Workspace, error) {
	item, err := ReadWorkspaceRecord(ctx, hub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return appmodelcore.Workspace{}, err
	}
	templateID := strings.TrimSpace(item.TemplateID)
	if templateID == "" {
		return appmodelcore.Workspace{}, common.ErrInvalidArgument
	}
	template, err := ReadWorkspaceTemplate(ctx, hub, templateID)
	if err != nil {
		return appmodelcore.Workspace{}, err
	}
	if _, err := workspacecore.ApplyTemplateToWorkspace(item, template); err != nil {
		return appmodelcore.Workspace{}, err
	}
	if err := UpdateWorkspaceRecord(ctx, hub, item); err != nil {
		return appmodelcore.Workspace{}, err
	}
	return workspacecore.MapWorkspace(item), nil
}

func CreateWorkspaceTemplate(ctx context.Context, hub events.Hub, template workspacecore.WorkspaceTemplate) (workspacecore.WorkspaceTemplate, error) {
	item, err := workspacecore.NormalizeWorkspaceTemplate(template)
	if err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	record := workspacecore.WorkspaceTemplateToRepository(item)
	if err := ValidateWorkspaceTemplateSoulRefs(ctx, hub, record); err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	if err := CreateWorkspaceTemplateRecord(ctx, hub, record); err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	return item, nil
}

func UpdateWorkspaceTemplate(ctx context.Context, hub events.Hub, templateID string, template workspacecore.WorkspaceTemplate) (workspacecore.WorkspaceTemplate, error) {
	template.ID = strings.TrimSpace(templateID)
	item, err := workspacecore.NormalizeWorkspaceTemplate(template)
	if err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	record := workspacecore.WorkspaceTemplateToRepository(item)
	if err := ValidateWorkspaceTemplateSoulRefs(ctx, hub, record); err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	if err := UpdateWorkspaceTemplateRecord(ctx, hub, record); err != nil {
		return workspacecore.WorkspaceTemplate{}, err
	}
	return item, nil
}

func DeleteWorkspaceTemplate(ctx context.Context, hub events.Hub, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return common.ErrInvalidArgument
	}
	return DeleteWorkspaceTemplateRecord(ctx, hub, templateID)
}
