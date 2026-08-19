package contextregistryports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateSkill(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.SkillMutation) (workorchmanagementcontract.Skill, error) {
	if hub == nil {
		return workorchmanagementcontract.Skill{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandCreate, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.CreateSkillCommand(skillCommandFromMutation(cmd)))
	ownerevent.BindLane(event, skillevents.Contracts(), skillevents.TopicSkillCommandCreate, "skill", cmd.ID, "", "", "")
	result, err := ownerevent.Skill[skillevents.CreateSkillResult](hub.Send(event), skillevents.TopicSkillCommandCreate)
	if err != nil {
		return workorchmanagementcontract.Skill{}, err
	}
	return skillFromModel(result.Skill)
}

func ReadSkill(ctx context.Context, hub magicevent.Hub, skillID string) (workorchmanagementcontract.Skill, error) {
	if hub == nil {
		return workorchmanagementcontract.Skill{}, common.ErrInvalidArgument
	}
	item, err := ownerreadports.ReadSkillByOwnerCommand(ctx, hub, common.SkillID(skillID))
	if err != nil {
		return workorchmanagementcontract.Skill{}, err
	}
	return skillFromModel(item)
}

func ReadSkillView(ctx context.Context, hub magicevent.Hub, skillID string, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) (workorchmanagementcontract.Skill, error) {
	ret, err := ReadSkill(ctx, hub, skillID)
	if err == nil {
		return ret, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return workorchmanagementcontract.Skill{}, err
	}
	return contextregistrycore.ReadSkillHubSkill(ctx, skillHubCatalog, skillID, query)
}

func ListSkills(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := ownerreadports.ListSkillsByOwnerCommand(ctx, hub)
	if err != nil {
		return nil, err
	}
	return skillsFromModels(items)
}

func ListSkillsView(ctx context.Context, hub magicevent.Hub, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) ([]workorchmanagementcontract.Skill, error) {
	ret, err := ListSkills(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret = append(ret, contextregistrycore.ListSkillHubSkillsForQuery(ctx, skillHubCatalog, query)...)
	return ret, nil
}

func skillFromModel(item *skillmodel.Skill) (workorchmanagementcontract.Skill, error) {
	if item == nil {
		return workorchmanagementcontract.Skill{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapRepositorySkill(item), nil
}

func skillsFromModels(items []*skillmodel.Skill) ([]workorchmanagementcontract.Skill, error) {
	ret := make([]workorchmanagementcontract.Skill, 0, len(items))
	for _, item := range items {
		if item == nil {
			return nil, common.ErrInvalidArgument
		}
		ret = append(ret, contextregistrycore.MapRepositorySkill(item))
	}
	return ret, nil
}
