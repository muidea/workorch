package setupdraftports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BuildGenerationRequest(ctx context.Context, hub magicevent.Hub, cmd setupdraftcore.GenerationCommand) (setupdraftcore.GenerationRequest, error) {
	workspace := setupdraftcore.Workspace{
		Title:                strings.TrimSpace(cmd.Workspace.Title),
		Description:          strings.TrimSpace(cmd.Workspace.Description),
		DeliverableDirectory: strings.TrimSpace(cmd.Workspace.DeliverableDirectory),
	}
	messages := setupdraftcore.NormalizeMessages(cmd.Messages)
	if workspace.Title == "" && len(messages) == 0 && strings.TrimSpace(cmd.CurrentDraft.GoalText) == "" {
		return setupdraftcore.GenerationRequest{}, common.ErrInvalidArgument
	}
	template, souls, err := TemplateContext(ctx, hub, cmd.TemplateID)
	if err != nil {
		return setupdraftcore.GenerationRequest{}, err
	}
	return setupdraftcore.GenerationRequest{
		Template:       template,
		Souls:          souls,
		Workspace:      workspace,
		Messages:       messages,
		CurrentDraft:   setupdraftcore.NormalizeDraft(cmd.CurrentDraft),
		DialogLanguage: strings.TrimSpace(cmd.DialogLanguage),
		Regenerate:     cmd.Regenerate,
	}, nil
}

func TemplateContext(ctx context.Context, hub magicevent.Hub, templateID string) (workspacecore.WorkspaceTemplate, []setupdraftcore.Soul, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return workspacecore.WorkspaceTemplate{}, nil, nil
	}
	if hub == nil {
		return workspacecore.WorkspaceTemplate{}, nil, common.ErrInvalidArgument
	}
	item, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, templateID)
	if err != nil {
		return workspacecore.WorkspaceTemplate{}, nil, err
	}
	template := workspacecore.WorkspaceTemplateFromOwner(item)
	return template, templateSouls(ctx, hub, template), nil
}

func templateSouls(ctx context.Context, hub magicevent.Hub, template workspacecore.WorkspaceTemplate) []setupdraftcore.Soul {
	ids := map[string]struct{}{}
	if strings.TrimSpace(template.DefaultSoulID) != "" {
		ids[strings.TrimSpace(template.DefaultSoulID)] = struct{}{}
	}
	for _, soulID := range template.SoulBindings {
		soulID = strings.TrimSpace(soulID)
		if soulID != "" {
			ids[soulID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ret := make([]setupdraftcore.Soul, 0, len(ids))
	for soulID := range ids {
		item, err := ownerreadports.ReadSoulByOwnerCommand(ctx, hub, common.SoulID(soulID))
		if err == nil {
			ret = append(ret, setupdraftcore.SoulFromRepository(item))
		}
	}
	return ret
}
