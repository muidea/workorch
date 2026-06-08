package contextregistryports

import (
	"context"
	"errors"

	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateSkill(ctx context.Context, hub events.Hub, cmd contextregistrycore.Skill) (contextregistrycore.Skill, error) {
	response, err := PublishCommandWithResponse(ctx, hub, skillevents.TopicSkillCommandCreate, "skill", cmd.ID, map[string]any{
		events.PayloadKeyCommand: skillCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.Skill{}, err
	}
	return contextregistrycore.SkillFromPayload(response.Payload, events.PayloadKeySkill)
}

func ReadSkill(ctx context.Context, hub events.Hub, skillID string) (contextregistrycore.Skill, error) {
	response, err := PublishCommandWithResponse(ctx, hub, skillevents.TopicSkillCommandGet, "skill", skillID, map[string]any{
		events.PayloadKeySkillID: skillID,
	})
	if err != nil {
		return contextregistrycore.Skill{}, err
	}
	return contextregistrycore.SkillFromPayload(response.Payload, events.PayloadKeySkill)
}

func ReadSkillView(ctx context.Context, hub events.Hub, skillID string, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) (contextregistrycore.Skill, error) {
	ret, err := ReadSkill(ctx, hub, skillID)
	if err == nil {
		return ret, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return contextregistrycore.Skill{}, err
	}
	return contextregistrycore.ReadSkillHubSkill(ctx, skillHubCatalog, skillID, query)
}

func ListSkills(ctx context.Context, hub events.Hub) ([]contextregistrycore.Skill, error) {
	response, err := PublishCommandWithResponse(ctx, hub, skillevents.TopicSkillCommandList, "skill", "all", nil)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.SkillsFromPayload(response.Payload, events.PayloadKeySkills)
}

func ListSkillsView(ctx context.Context, hub events.Hub, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) ([]contextregistrycore.Skill, error) {
	ret, err := ListSkills(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret = append(ret, contextregistrycore.ListSkillHubSkillsForQuery(ctx, skillHubCatalog, query)...)
	return ret, nil
}
