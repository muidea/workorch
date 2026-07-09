package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/common"
	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type DeleteSoulCommand = soulcontracts.DeleteSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand

type GetSoulCommand struct {
	SoulID common.SoulID
}

type ListSoulsCommand struct{}

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
		}
	}
	return ret
}
