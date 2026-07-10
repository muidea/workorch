package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"

	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadSoulByOwnerCommand(ctx context.Context, hub magicevent.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	if hub == nil || soulID == "" {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[soulevents.GetSoulCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandGet, soulevents.CommandDestination, "soul", string(soulID), soulevents.GetSoulCommand{SoulID: soulID})
	if err != nil {
		return nil, err
	}
	if result.Soul == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Soul, nil
}

func ReadSkillByOwnerCommand(ctx context.Context, hub magicevent.Hub, skillID common.SkillID) (*skillmodel.Skill, error) {
	if hub == nil || skillID == "" {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[skillevents.GetSkillCommand, skillevents.SkillCommandResult](ctx, hub, skillevents.TopicSkillCommandGet, skillevents.CommandDestination, "skill", string(skillID), skillevents.GetSkillCommand{SkillID: skillID})
	if err != nil {
		return nil, err
	}
	if result.Skill == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Skill, nil
}

func ListSkillsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*skillmodel.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[skillevents.ListSkillsCommand, skillevents.SkillCommandResult](ctx, hub, skillevents.TopicSkillCommandList, skillevents.CommandDestination, "skill", "all", skillevents.ListSkillsCommand{})
	if err != nil {
		return nil, err
	}
	return result.Skills, nil
}
