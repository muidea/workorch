package workspaceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ApplyWorkspaceTemplateOnCreate(ctx context.Context, hub magicevent.Hub, item *workspacemodel.Workspace, templateID string) (*workspacemodel.Workspace, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return item, nil
	}
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	templateItem, err := ReadWorkspaceTemplateRecord(ctx, hub, templateID)
	if err != nil {
		return nil, err
	}
	if err := ValidateWorkspaceTemplateSoulRefs(ctx, hub, templateItem); err != nil {
		return nil, err
	}
	updated, err := workspacecore.ApplyTemplateToWorkspace(item, workspacecore.WorkspaceTemplateFromRepository(templateItem))
	if err != nil {
		return nil, err
	}
	if workspacecore.TemplateRequestsGitInit(updated.ContextHints.Map()) {
		if err := workspacecore.InitializeGitRepository(ctx, updated.DeliverableDirectory); err != nil {
			return nil, err
		}
	}
	if err := UpdateWorkspaceRecord(ctx, hub, updated); err != nil {
		return nil, err
	}
	return ReadWorkspaceRecord(ctx, hub, updated.ID)
}

func ValidateWorkspaceTemplateSoulRefs(ctx context.Context, hub magicevent.Hub, template *repository.WorkspaceTemplate) error {
	if template == nil {
		return common.ErrInvalidArgument
	}
	bindings := map[string]common.SoulID{}
	for stage, soulID := range template.SoulBindings {
		bindings[stage] = common.SoulID(soulID)
	}
	return validateSoulRefs(ctx, hub, common.SoulID(template.DefaultSoulID), bindings)
}
