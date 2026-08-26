package events

import (
	"fmt"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
const CommandDestination = modulecommon.ModuleID

const (
	DefaultListLimit       = 100
	MaxListLimit           = 5000
	MaxHighWatermarkScopes = 500
)

// EventQuery is the Eventstream owner's public query DTO. It deliberately does
// not expose the repository query type across EventHub.
type EventQuery struct {
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

func (q EventQuery) Normalized() (EventQuery, error) {
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.RunID = common.RunID(strings.TrimSpace(string(q.RunID)))
	q.Topic = strings.TrimSpace(q.Topic)
	q.SubjectType = strings.TrimSpace(q.SubjectType)
	q.SubjectID = strings.TrimSpace(q.SubjectID)
	q.CursorAfterID = common.EventRecordID(strings.TrimSpace(string(q.CursorAfterID)))
	q.OccurredAfter = strings.TrimSpace(q.OccurredAfter)
	q.OccurredBefore = strings.TrimSpace(q.OccurredBefore)
	seen := map[string]struct{}{}
	excluded := make([]string, 0, len(q.ExcludeTopics))
	for _, topic := range q.ExcludeTopics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		excluded = append(excluded, topic)
	}
	q.ExcludeTopics = excluded
	if q.Limit == 0 {
		q.Limit = DefaultListLimit
	}
	if q.Limit < 1 || q.Limit > MaxListLimit {
		return EventQuery{}, fmt.Errorf("%w: eventstream list limit must be between 1 and %d", common.ErrInvalidArgument, MaxListLimit)
	}
	return q, nil
}

type ListEventsCommand struct {
	Query EventQuery
}

type HighWatermarksCommand struct {
	WorkspaceIDs []common.WorkspaceID
}

type WorkspaceHighWatermark struct {
	WorkspaceID common.WorkspaceID
	Revision    int64
}

type ListEventsResult struct{ Events []EventRecordData }

type HighWatermarksResult struct{ HighWatermarks []WorkspaceHighWatermark }

type EventRecordAppendedData struct {
	Record EventRecordData
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[ListEventsCommand, ListEventsResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandList),
		eventcontract.CommandTopic[HighWatermarksCommand, HighWatermarksResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandHighWatermarks),
		eventcontract.CommandTopic[SealWorkspaceEventStreamDeletionCommand, SealWorkspaceEventStreamDeletionResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandSealWorkspaceDeletion),
		eventcontract.CommandTopic[PurgeWorkspaceCommand, PurgeWorkspaceResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandPurgeWorkspace),
		eventcontract.DataTopic[EventRecordAppendedData]("eventstream", definitionPackage, eventcontract.LanePolicyState, TopicEventStreamRecordAppended),
	)
}
