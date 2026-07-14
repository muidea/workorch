package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadSoulByOwnerCommand(ctx context.Context, hub magicevent.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	if hub == nil || soulID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandGet, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.GetSoulCommand{SoulID: soulID})
	ownerevent.BindLane(event, soulevents.TopicSoulCommandGet, "soul", string(soulID), "", "", "")
	result, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandGet)
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
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandGet, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.GetSkillCommand{SkillID: skillID})
	ownerevent.BindLane(event, skillevents.TopicSkillCommandGet, "skill", string(skillID), "", "", "")
	result, err := ownerevent.Skill(hub.Send(event), skillevents.TopicSkillCommandGet)
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
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandList, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.ListSkillsCommand{})
	ownerevent.BindLane(event, skillevents.TopicSkillCommandList, "skill", "all", "", "", "")
	result, err := ownerevent.Skill(hub.Send(event), skillevents.TopicSkillCommandList)
	if err != nil {
		return nil, err
	}
	return result.Skills, nil
}
