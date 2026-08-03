package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateSoulCommand struct {
	SoulID                      common.SoulID
	Name                        string
	Description                 string
	BusinessDomains             []string
	SoulMD                      string
	AllowedSkillRefs            []string
	AllowedMCPServerRefs        []string
	AllowedCapabilityRefs       []string
	AllowedCapabilityCategories []string
	AllowedToolKinds            []string
}

type UpdateSoulCommand struct {
	SoulID                      common.SoulID
	Name                        string
	Description                 string
	BusinessDomains             []string
	SoulMD                      string
	AllowedSkillRefs            []string
	AllowedMCPServerRefs        []string
	AllowedCapabilityRefs       []string
	AllowedCapabilityCategories []string
	AllowedToolKinds            []string
}

type DeleteSoulCommand struct {
	SoulID common.SoulID
}

type ValidateCapabilitySelectionCommand struct {
	SoulID               common.SoulID
	SkillRefs            []string
	MCPServerRefs        []string
	CapabilityRefs       []string
	CapabilityCategories []string
	ToolKinds            []string
}

type GetSoulCommand struct {
	SoulID common.SoulID
}

type ListSoulsCommand struct{}
type EnsureDefaultsCommand struct{}

type EnsureDefaultsResult struct {
	Ensured bool
}

type SoulCommandResult struct {
	Soul  *soulmodel.Soul
	Souls []*soulmodel.Soul
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{}
	for _, topic := range []string{
		TopicSoulCommandCreate,
		TopicSoulCommandGet,
		TopicSoulCommandList,
		TopicSoulCommandUpdate,
		TopicSoulCommandDelete,
		TopicSoulCommandValidateCapability,
		TopicSoulCommandEnsureDefaults,
	} {
		class := eventcontract.CommandClassWrite
		lane := eventcontract.LanePolicyState
		switch topic {
		case TopicSoulCommandGet, TopicSoulCommandList, TopicSoulCommandValidateCapability:
			class = eventcontract.CommandClassRead
			lane = eventcontract.LanePolicyQuery
		}
		switch topic {
		case TopicSoulCommandCreate:
			ret = append(ret, eventcontract.CommandTopic[CreateSoulCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetSoulCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandList:
			ret = append(ret, eventcontract.CommandTopic[ListSoulsCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandUpdate:
			ret = append(ret, eventcontract.CommandTopic[UpdateSoulCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteSoulCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandValidateCapability:
			ret = append(ret, eventcontract.CommandTopic[ValidateCapabilitySelectionCommand, SoulCommandResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandEnsureDefaults:
			ret = append(ret, eventcontract.CommandTopic[EnsureDefaultsCommand, EnsureDefaultsResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	return ret
}
