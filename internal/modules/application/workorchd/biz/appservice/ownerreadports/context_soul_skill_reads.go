package ownerreadports

import (
	"context"

	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"

	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadSoulByOwnerCommand(ctx context.Context, hub events.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	if hub == nil || soulID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, soulevents.TopicSoulCommandGet, "soul", string(soulID), "", "", map[string]any{
		events.PayloadKeySoulID: string(soulID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ReadSkillByOwnerCommand(ctx context.Context, hub events.Hub, skillID common.SkillID) (*skillmodel.Skill, error) {
	if hub == nil || skillID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, skillevents.TopicSkillCommandGet, "skill", string(skillID), "", "", map[string]any{
		events.PayloadKeySkillID: string(skillID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListSkillsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*skillmodel.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, skillevents.TopicSkillCommandList, "skill", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySkills].([]*skillmodel.Skill)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
