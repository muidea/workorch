package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillcontracts "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/contracts"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateSkillCommand = skillcontracts.CreateSkillCommand
type UpdateSkillCommand = skillcontracts.UpdateSkillCommand

type GetSkillCommand struct {
	SkillID common.SkillID
}

type ListSkillsCommand struct{}

type SkillCommandResult struct {
	Skill  *skillmodel.Skill
	Skills []*skillmodel.Skill
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{}
	for _, topic := range []string{
		TopicSkillCommandCreate,
		TopicSkillCommandGet,
		TopicSkillCommandList,
		TopicSkillCommandUpdate,
	} {
		class := eventcontract.CommandClassWrite
		lane := eventcontract.LanePolicyState
		switch topic {
		case TopicSkillCommandGet, TopicSkillCommandList:
			class = eventcontract.CommandClassRead
			lane = eventcontract.LanePolicyQuery
		}
		switch topic {
		case TopicSkillCommandCreate:
			ret = append(ret, eventcontract.CommandTopic[CreateSkillCommand, SkillCommandResult]("skill", definitionPackage, class, lane, topic).MessageContract())
		case TopicSkillCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetSkillCommand, SkillCommandResult]("skill", definitionPackage, class, lane, topic).MessageContract())
		case TopicSkillCommandList:
			ret = append(ret, eventcontract.CommandTopic[ListSkillsCommand, SkillCommandResult]("skill", definitionPackage, class, lane, topic).MessageContract())
		case TopicSkillCommandUpdate:
			ret = append(ret, eventcontract.CommandTopic[UpdateSkillCommand, SkillCommandResult]("skill", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	return ret
}
