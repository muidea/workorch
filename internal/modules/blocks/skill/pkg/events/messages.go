package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateSkillCommand struct {
	SkillID                common.SkillID
	Name                   string
	Version                string
	Description            string
	TriggerRules           []string
	Instruction            string
	ReferenceRefs          []string
	RequiredToolKinds      []string
	RequiredMCPServerRefs  []string
	RequiredCapabilityRefs []string
	Constraints            []string
}

type GetSkillCommand struct {
	SkillID common.SkillID
}

type ListSkillsCommand struct {
	CursorAfterID common.SkillID
	Limit         int
}

type CreateSkillResult struct {
	Skill *skillmodel.Skill
}

func (r CreateSkillResult) Clone() CreateSkillResult {
	return CreateSkillResult{Skill: skillmodel.CloneSkill(r.Skill)}
}

type GetSkillResult struct {
	Skill *skillmodel.Skill
}

func (r GetSkillResult) Clone() GetSkillResult {
	return GetSkillResult{Skill: skillmodel.CloneSkill(r.Skill)}
}

type ListSkillsResult struct {
	Skills     []*skillmodel.Skill
	NextCursor common.SkillID
}

func (r ListSkillsResult) Clone() ListSkillsResult {
	return ListSkillsResult{Skills: skillmodel.CloneSkills(r.Skills), NextCursor: r.NextCursor}
}

func MessageContracts() []eventcontract.MessageContract {
	return []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateSkillCommand, CreateSkillResult]("skill", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicSkillCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetSkillCommand, GetSkillResult]("skill", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicSkillCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListSkillsCommand, ListSkillsResult]("skill", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicSkillCommandList).MessageContract(),
	}
}
