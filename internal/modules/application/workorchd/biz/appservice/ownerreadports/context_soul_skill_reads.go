package ownerreadports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	skillReadPageLimit = 100
	skillReadMaxPages  = 1000
	skillReadMaxItems  = 100000
)

func ReadSoulByOwnerCommand(ctx context.Context, hub magicevent.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	if hub == nil || soulID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandGet, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.GetSoulCommand{SoulID: soulID})
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandGet, "soul", string(soulID), "", "", "")
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
	ownerevent.BindLane(event, skillevents.Contracts(), skillevents.TopicSkillCommandGet, "skill", string(skillID), "", "", "")
	result, err := ownerevent.Skill[skillevents.GetSkillResult](hub.Send(event), skillevents.TopicSkillCommandGet)
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
	ret := make([]*skillmodel.Skill, 0)
	var cursor common.SkillID
	seen := make(map[common.SkillID]struct{})
	for page := 0; page < skillReadMaxPages; page++ {
		event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandList, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.ListSkillsCommand{CursorAfterID: cursor, Limit: skillReadPageLimit})
		ownerevent.BindLane(event, skillevents.Contracts(), skillevents.TopicSkillCommandList, "skill", "all", "", "", "")
		result, err := ownerevent.Skill[skillevents.ListSkillsResult](hub.Send(event), skillevents.TopicSkillCommandList)
		if err != nil {
			return nil, err
		}
		if len(result.Skills) > skillReadPageLimit || len(ret)+len(result.Skills) > skillReadMaxItems {
			return nil, common.ErrConflict
		}
		previous := cursor
		for _, item := range result.Skills {
			if item == nil || item.ID == "" || (previous != "" && item.ID <= previous) {
				return nil, common.ErrConflict
			}
			ret = append(ret, skillmodel.CloneSkill(item))
			previous = item.ID
		}
		if result.NextCursor == "" {
			return ret, nil
		}
		if len(result.Skills) == 0 || result.NextCursor != result.Skills[len(result.Skills)-1].ID || result.NextCursor <= cursor {
			return nil, common.ErrConflict
		}
		if _, duplicate := seen[result.NextCursor]; duplicate {
			return nil, common.ErrConflict
		}
		seen[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
	return nil, fmt.Errorf("%w: Skill owner pagination exceeded %d pages", common.ErrConflict, skillReadMaxPages)
}
