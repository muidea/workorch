package contextregistryports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateSkill(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.Skill) (contextregistrycore.Skill, error) {
	if hub == nil {
		return contextregistrycore.Skill{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandCreate, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.CreateSkillCommand(skillCommandFromView(cmd)))
	ownerevent.BindLane(event, skillevents.TopicSkillCommandCreate, "skill", cmd.ID, "", "", "")
	result, err := ownerevent.Skill(hub.Send(event), skillevents.TopicSkillCommandCreate)
	if err != nil {
		return contextregistrycore.Skill{}, err
	}
	return skillFromCommandResult(result)
}

func ReadSkill(ctx context.Context, hub magicevent.Hub, skillID string) (contextregistrycore.Skill, error) {
	if hub == nil {
		return contextregistrycore.Skill{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandGet, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.GetSkillCommand{SkillID: common.SkillID(skillID)})
	ownerevent.BindLane(event, skillevents.TopicSkillCommandGet, "skill", skillID, "", "", "")
	result, err := ownerevent.Skill(hub.Send(event), skillevents.TopicSkillCommandGet)
	if err != nil {
		return contextregistrycore.Skill{}, err
	}
	return skillFromCommandResult(result)
}

func ReadSkillView(ctx context.Context, hub magicevent.Hub, skillID string, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) (contextregistrycore.Skill, error) {
	ret, err := ReadSkill(ctx, hub, skillID)
	if err == nil {
		return ret, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return contextregistrycore.Skill{}, err
	}
	return contextregistrycore.ReadSkillHubSkill(ctx, skillHubCatalog, skillID, query)
}

func ListSkills(ctx context.Context, hub magicevent.Hub) ([]contextregistrycore.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandList, sourceModule, skillevents.CommandDestination, magicevent.NewHeader(), ctx, skillevents.ListSkillsCommand{})
	ownerevent.BindLane(event, skillevents.TopicSkillCommandList, "skill", "all", "", "", "")
	result, err := ownerevent.Skill(hub.Send(event), skillevents.TopicSkillCommandList)
	if err != nil {
		return nil, err
	}
	return skillsFromCommandResult(result)
}

func ListSkillsView(ctx context.Context, hub magicevent.Hub, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) ([]contextregistrycore.Skill, error) {
	ret, err := ListSkills(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret = append(ret, contextregistrycore.ListSkillHubSkillsForQuery(ctx, skillHubCatalog, query)...)
	return ret, nil
}

func skillFromCommandResult(result skillevents.SkillCommandResult) (contextregistrycore.Skill, error) {
	if result.Skill == nil {
		return contextregistrycore.Skill{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapRepositorySkill(result.Skill), nil
}

func skillsFromCommandResult(result skillevents.SkillCommandResult) ([]contextregistrycore.Skill, error) {
	ret := make([]contextregistrycore.Skill, 0, len(result.Skills))
	for _, item := range result.Skills {
		ret = append(ret, contextregistrycore.MapRepositorySkill(item))
	}
	return ret, nil
}
