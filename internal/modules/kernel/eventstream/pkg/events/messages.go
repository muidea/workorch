package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamcontracts "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
const CommandDestination = modulecommon.ModuleID

type ListEventsCommand struct {
	Query EventRecordQuery
}

type EventRecordQuery struct {
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Topic          string
	ExcludeTopics  []string
	SubjectType    string
	SubjectID      string
	CursorAfterID  common.EventRecordID
	OccurredAfter  string
	OccurredBefore string
	Limit          int
	IncludePayload bool
}

type PurgeWorkspaceCommand = eventstreamcontracts.PurgeWorkspaceCommand

type HighWatermarksCommand struct {
	WorkspaceIDs []common.WorkspaceID
}

type WorkspaceHighWatermark struct {
	WorkspaceID common.WorkspaceID
	Revision    int64
}

type EventStreamCommandResult struct {
	Events         []EventRecordData
	HighWatermarks []WorkspaceHighWatermark
	DeletedCount   int
}

type EventRecordAppendedData struct {
	Record EventRecordData
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[ListEventsCommand, EventStreamCommandResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandList),
		eventcontract.CommandTopic[HighWatermarksCommand, EventStreamCommandResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandHighWatermarks),
		eventcontract.CommandTopic[PurgeWorkspaceCommand, EventStreamCommandResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandPurgeWorkspace),
		eventcontract.DataTopic[EventRecordAppendedData]("eventstream", definitionPackage, eventcontract.LanePolicyState, TopicEventStreamRecordAppended),
	)
}
