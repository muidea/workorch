package events

import (
	"fmt"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
const CommandDestination = modulecommon.ModuleID

const (
	DefaultListLimit       = 100
	MaxListLimit           = 5000
	MaxHighWatermarkScopes = 500
	// MaxHighWatermarkWorkspaceIDBytes bounds each raw workspace ID before
	// normalization so padding cannot bypass the owner input budget.
	MaxHighWatermarkWorkspaceIDBytes = 4 * 1024
	MaxQueryStringBytes              = 4 * 1024
	MaxExcludeTopics                 = 128
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
	stringFields := []string{
		string(q.WorkspaceID), string(q.RunID), q.Topic, q.SubjectType, q.SubjectID,
		string(q.CursorAfterID), q.OccurredAfter, q.OccurredBefore,
	}
	for _, value := range stringFields {
		if len(value) > MaxQueryStringBytes || !utf8.ValidString(value) {
			return EventQuery{}, fmt.Errorf("%w: eventstream query string exceeds owner bound or is not UTF-8", common.ErrInvalidArgument)
		}
	}
	if len(q.ExcludeTopics) > MaxExcludeTopics {
		return EventQuery{}, fmt.Errorf("%w: eventstream exclude-topic count exceeds %d", common.ErrInvalidArgument, MaxExcludeTopics)
	}
	q.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(q.WorkspaceID)))
	q.RunID = common.RunID(strings.TrimSpace(string(q.RunID)))
	q.Topic = strings.TrimSpace(q.Topic)
	q.SubjectType = strings.TrimSpace(q.SubjectType)
	q.SubjectID = strings.TrimSpace(q.SubjectID)
	q.CursorAfterID = common.EventRecordID(strings.TrimSpace(string(q.CursorAfterID)))
	seen := map[string]struct{}{}
	excluded := make([]string, 0, len(q.ExcludeTopics))
	for _, rawTopic := range q.ExcludeTopics {
		if len(rawTopic) > MaxQueryStringBytes || !utf8.ValidString(rawTopic) {
			return EventQuery{}, fmt.Errorf("%w: eventstream excluded topic exceeds owner bound or is not UTF-8", common.ErrInvalidArgument)
		}
		topic := rawTopic
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
	timeQuery, err := (workorchquerycontract.EventRecordQuery{
		OccurredAfter: q.OccurredAfter, OccurredBefore: q.OccurredBefore,
	}).Canonicalized()
	if err != nil {
		return EventQuery{}, err
	}
	q.OccurredAfter = timeQuery.OccurredAfter
	q.OccurredBefore = timeQuery.OccurredBefore
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

// Normalized validates the entire high-watermark scope before owner I/O,
// trims each workspace ID, rejects blank/invalid/oversized items, and
// collapses canonical duplicates while preserving first-seen order.
func (c HighWatermarksCommand) Normalized() (HighWatermarksCommand, error) {
	if len(c.WorkspaceIDs) == 0 || len(c.WorkspaceIDs) > MaxHighWatermarkScopes {
		return HighWatermarksCommand{}, fmt.Errorf("%w: eventstream high-watermark workspace count must be between 1 and %d", common.ErrInvalidArgument, MaxHighWatermarkScopes)
	}
	seen := make(map[common.WorkspaceID]struct{}, len(c.WorkspaceIDs))
	workspaceIDs := make([]common.WorkspaceID, 0, len(c.WorkspaceIDs))
	for _, rawWorkspaceID := range c.WorkspaceIDs {
		raw := string(rawWorkspaceID)
		if len(raw) > MaxHighWatermarkWorkspaceIDBytes || !utf8.ValidString(raw) {
			return HighWatermarksCommand{}, fmt.Errorf("%w: eventstream high-watermark workspace ID exceeds owner bound or is not UTF-8", common.ErrInvalidArgument)
		}
		workspaceID := common.WorkspaceID(strings.TrimSpace(raw))
		if workspaceID == "" {
			return HighWatermarksCommand{}, fmt.Errorf("%w: eventstream high-watermark workspace ID is blank", common.ErrInvalidArgument)
		}
		if _, ok := seen[workspaceID]; ok {
			continue
		}
		seen[workspaceID] = struct{}{}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	c.WorkspaceIDs = workspaceIDs
	return c, nil
}

type WorkspaceHighWatermark struct {
	WorkspaceID common.WorkspaceID
	Revision    int64
}

type ListEventsResult struct{ Events []EventRecordData }

type HighWatermarksResult struct{ HighWatermarks []WorkspaceHighWatermark }

// ValidateFor binds a high-watermark response to the exact canonical request
// scope. Results are complete, unique and sorted by workspace ID; a negative
// revision or a missing, duplicate or injected workspace fails closed.
func (r HighWatermarksResult) ValidateFor(command HighWatermarksCommand) error {
	command, err := command.Normalized()
	if err != nil {
		return err
	}
	if len(r.HighWatermarks) != len(command.WorkspaceIDs) {
		return fmt.Errorf("%w: eventstream high-watermark result scope count drift", common.ErrInvalidArgument)
	}
	expected := make(map[common.WorkspaceID]struct{}, len(command.WorkspaceIDs))
	for _, workspaceID := range command.WorkspaceIDs {
		expected[workspaceID] = struct{}{}
	}
	seen := make(map[common.WorkspaceID]struct{}, len(r.HighWatermarks))
	previous := ""
	for index, item := range r.HighWatermarks {
		raw := string(item.WorkspaceID)
		if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > MaxHighWatermarkWorkspaceIDBytes || !utf8.ValidString(raw) || item.Revision < 0 {
			return fmt.Errorf("%w: invalid eventstream high-watermark result item", common.ErrInvalidArgument)
		}
		if _, ok := expected[item.WorkspaceID]; !ok {
			return fmt.Errorf("%w: eventstream high-watermark result contains unrequested workspace", common.ErrInvalidArgument)
		}
		if _, ok := seen[item.WorkspaceID]; ok {
			return fmt.Errorf("%w: eventstream high-watermark result contains duplicate workspace", common.ErrInvalidArgument)
		}
		if index > 0 && raw <= previous {
			return fmt.Errorf("%w: eventstream high-watermark result is not canonically ordered", common.ErrInvalidArgument)
		}
		seen[item.WorkspaceID] = struct{}{}
		previous = raw
	}
	return nil
}

func (r HighWatermarksResult) Clone() HighWatermarksResult {
	return HighWatermarksResult{HighWatermarks: append([]WorkspaceHighWatermark(nil), r.HighWatermarks...)}
}

type EventRecordAppendedData struct {
	Record EventRecordData
}

// ApprovalResolutionFact is the immutable Approval-owner observation accepted
// by Eventstream for durable audit recording. It is not Approval transition
// authority.
type ApprovalResolutionFact struct {
	NotificationID     string
	ResolutionID       common.ResolutionID
	ApprovalID         common.ApprovalID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	Reason             string
	Action             string
	Context            []workorcheventpayload.Field
	ResolutionContext  []workorcheventpayload.Field
	PrincipalKind      string
	PrincipalSubjectID string
	State              common.ApprovalState
}

func (d ApprovalResolutionFact) Validate() error {
	if strings.TrimSpace(d.NotificationID) == "" || d.ResolutionID == "" || d.ApprovalID == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.PrincipalSubjectID) == "" {
		return fmt.Errorf("%w: approval resolution fact requires identity, scope and principal", common.ErrInvalidArgument)
	}
	switch strings.TrimSpace(d.PrincipalKind) {
	case "human_operator", "auto_governance":
	default:
		return fmt.Errorf("%w: invalid approval resolution principal kind %q", common.ErrInvalidArgument, d.PrincipalKind)
	}
	if d.State != common.ApprovalStateApproved && d.State != common.ApprovalStateRejected {
		return fmt.Errorf("%w: approval resolution fact requires terminal state", common.ErrInvalidArgument)
	}
	return nil
}

type RecordApprovalResolutionCommand struct{ Data ApprovalResolutionFact }

type RecordApprovalResolutionResult struct {
	NotificationID string
	ResolutionID   common.ResolutionID
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[ListEventsCommand, ListEventsResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandList),
		eventcontract.CommandTopic[HighWatermarksCommand, HighWatermarksResult]("eventstream", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicEventStreamCommandHighWatermarks),
		eventcontract.CommandTopic[SealWorkspaceEventStreamDeletionCommand, SealWorkspaceEventStreamDeletionResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandSealWorkspaceDeletion),
		eventcontract.CommandTopic[PurgeWorkspaceCommand, PurgeWorkspaceResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandPurgeWorkspace),
		eventcontract.CommandTopic[RecordApprovalResolutionCommand, RecordApprovalResolutionResult]("eventstream", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicEventStreamCommandRecordApprovalResolution),
		eventcontract.DataTopic[EventRecordAppendedData]("eventstream", definitionPackage, eventcontract.LanePolicyState, TopicEventStreamRecordAppended),
	)
}
