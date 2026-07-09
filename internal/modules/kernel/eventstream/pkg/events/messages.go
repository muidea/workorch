package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamcontracts "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/contracts"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
const CommandDestination = modulecommon.ModuleID

type ListEventsCommand struct {
	Query repository.EventRecordQuery
}

type PurgeWorkspaceCommand = eventstreamcontracts.PurgeWorkspaceCommand

type EventStreamCommandResult struct {
	Events       []*eventstreammodel.EventRecord
	DeletedCount int
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[ListEventsCommand, EventStreamCommandResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandList),
		eventcontract.CommandTopic[PurgeWorkspaceCommand, EventStreamCommandResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandPurgeWorkspace),
	)
}
