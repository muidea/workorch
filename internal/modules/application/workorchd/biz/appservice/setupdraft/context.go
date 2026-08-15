package setupdraft

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func BuildGenerationRequest(ctx context.Context, hub magicevent.Hub, cmd GenerationCommand) (GenerationRequest, error) {
	workspace := Workspace{
		Title:                strings.TrimSpace(cmd.Workspace.Title),
		Description:          strings.TrimSpace(cmd.Workspace.Description),
		DeliverableDirectory: strings.TrimSpace(cmd.Workspace.DeliverableDirectory),
	}
	messages := NormalizeMessages(cmd.Messages)
	if workspace.Title == "" && len(messages) == 0 && strings.TrimSpace(cmd.CurrentDraft.GoalText) == "" {
		return GenerationRequest{}, common.ErrInvalidArgument
	}
	template, souls, err := TemplateContext(ctx, hub, cmd.TemplateID)
	if err != nil {
		return GenerationRequest{}, err
	}
	return GenerationRequest{
		Template:       template,
		Souls:          souls,
		Workspace:      workspace,
		Messages:       messages,
		CurrentDraft:   NormalizeDraft(cmd.CurrentDraft),
		DialogLanguage: strings.TrimSpace(cmd.DialogLanguage),
		Regenerate:     cmd.Regenerate,
	}, nil
}

func TemplateContext(ctx context.Context, hub magicevent.Hub, templateID string) (workorchmanagementcontract.WorkspaceTemplate, []Soul, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return workorchmanagementcontract.WorkspaceTemplate{}, nil, nil
	}
	if hub == nil {
		return workorchmanagementcontract.WorkspaceTemplate{}, nil, common.ErrInvalidArgument
	}
	item, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, templateID)
	if err != nil {
		return workorchmanagementcontract.WorkspaceTemplate{}, nil, err
	}
	template := workspacecore.WorkspaceTemplateFromOwner(item)
	return template, templateSouls(ctx, hub, template), nil
}

func templateSouls(ctx context.Context, hub magicevent.Hub, template workorchmanagementcontract.WorkspaceTemplate) []Soul {
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
	ret := make([]Soul, 0, len(ids))
	for soulID := range ids {
		item, err := ownerreadports.ReadSoulByOwnerCommand(ctx, hub, common.SoulID(soulID))
		if err == nil {
			ret = append(ret, SoulFromRepository(item))
		}
	}
	return ret
}
