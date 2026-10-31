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

type UpdateSoulCommand CreateSoulCommand

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

type ListSoulsCommand struct {
	CursorAfterID common.SoulID
	Limit         int
}
type EnsureDefaultsCommand struct{}

type EnsureDefaultsResult struct {
	Ensured bool
}

type CreateSoulResult struct {
	Soul *soulmodel.Soul
}

func (r CreateSoulResult) Clone() CreateSoulResult {
	return CreateSoulResult{Soul: soulmodel.CloneSoul(r.Soul)}
}

type GetSoulResult struct {
	Soul *soulmodel.Soul
}

func (r GetSoulResult) Clone() GetSoulResult {
	return GetSoulResult{Soul: soulmodel.CloneSoul(r.Soul)}
}

type ListSoulsResult struct {
	Souls      []*soulmodel.Soul
	NextCursor common.SoulID
}

func (r ListSoulsResult) Clone() ListSoulsResult {
	return ListSoulsResult{Souls: soulmodel.CloneSouls(r.Souls), NextCursor: r.NextCursor}
}

type UpdateSoulResult struct {
	Soul *soulmodel.Soul
}

func (r UpdateSoulResult) Clone() UpdateSoulResult {
	return UpdateSoulResult{Soul: soulmodel.CloneSoul(r.Soul)}
}

type DeleteSoulResult struct {
	SoulID  common.SoulID
	Deleted bool
}

type ValidateCapabilitySelectionResult struct {
	SoulID    common.SoulID
	Validated bool
}

func MessageContracts() []eventcontract.MessageContract {
	var ret []eventcontract.MessageContract
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
			ret = append(ret, eventcontract.CommandTopic[CreateSoulCommand, CreateSoulResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetSoulCommand, GetSoulResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandList:
			ret = append(ret, eventcontract.CommandTopic[ListSoulsCommand, ListSoulsResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandUpdate:
			ret = append(ret, eventcontract.CommandTopic[UpdateSoulCommand, UpdateSoulResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteSoulCommand, DeleteSoulResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandValidateCapability:
			ret = append(ret, eventcontract.CommandTopic[ValidateCapabilitySelectionCommand, ValidateCapabilitySelectionResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		case TopicSoulCommandEnsureDefaults:
			ret = append(ret, eventcontract.CommandTopic[EnsureDefaultsCommand, EnsureDefaultsResult]("soul", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	return ret
}
