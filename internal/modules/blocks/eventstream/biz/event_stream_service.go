package eventstream

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runcommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspacecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventrecordcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// Biz is the Eventstream Block lifecycle owner. Repository-facing operations
// remain private; callers interact only through concrete pkg/events contracts.
type Biz struct {
	basebiz.Base
	repo               repository.EventRecordRepository
	workspaceDeletions repository.EventStreamWorkspaceDeletionRepository
	workspacePurger    repository.EventStreamWorkspacePurger
	unsubscribers      []func()
	topics             []string
	appendMu           sync.Mutex
	revisions          map[common.WorkspaceID]int64
}

// Dependencies are the EventStream owner's construction inputs. Keeping this
// in biz prevents the module lifecycle bridge from owning EventHub types.
type Dependencies struct {
	OwnerRepositories repository.EventStreamOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

var eventRecordIDSequence atomic.Uint64

const (
	stableRunNotificationRecordIDPrefix = "run-notification/"
	stableMemoryRecallRecordIDPrefix    = "memory-recall-notification/"
	stableLLMUsageRecordIDPrefix        = "llm-usage-notification/"
	applicationCommandSource            = "application/workorchd"
)

func New(deps Dependencies) *Biz {
	return &Biz{
		Base: basebiz.New(modulecommon.ModuleID, deps.EventHub, deps.BackgroundRoutine), repo: deps.OwnerRepositories.EventRecords(),
		workspaceDeletions: deps.OwnerRepositories.WorkspaceDeletions(), workspacePurger: deps.OwnerRepositories.WorkspacePurger(),
	}
}

func (s *Biz) highWatermarks(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
	if s == nil {
		return nil, common.ErrInvalidArgument
	}
	command, err := (eventstreamevents.HighWatermarksCommand{WorkspaceIDs: workspaceIDs}).Normalized()
	if err != nil {
		return nil, err
	}
	if s.repo == nil || s.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	revisions := map[common.WorkspaceID]int64{}
	for _, workspaceID := range command.WorkspaceIDs {
		hidden, err := s.workspaceDeletionHidden(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			revisions[workspaceID] = 0
			continue
		}
		bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		items, err := s.repo.List(bound, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID, Limit: 1})
		if err != nil {
			return nil, err
		}
		revision := int64(0)
		if len(items) == 1 && items[0] != nil {
			revision = items[0].Revision
		}
		revisions[workspaceID] = revision
	}
	ret := make([]eventstreamevents.WorkspaceHighWatermark, 0, len(revisions))
	for workspaceID, revision := range revisions {
		ret = append(ret, eventstreamevents.WorkspaceHighWatermark{WorkspaceID: workspaceID, Revision: revision})
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i].WorkspaceID) < string(ret[j].WorkspaceID) })
	return ret, nil
}

func (s *Biz) Start() error {
	if s == nil || s.repo == nil || s.workspaceDeletions == nil || s.workspacePurger == nil || s.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if len(s.unsubscribers) > 0 {
		return nil
	}
	s.subscribeDecisionNotifications()
	s.subscribeApprovalNotifications()
	s.subscribeGovernanceNotifications()
	s.subscribeRunStateNotifications()
	s.subscribeWorkspaceNotifications()
	s.subscribeResultNotifications()
	s.subscribeMemoryNotifications()
	s.subscribeLLMNotifications()
	s.topics = []string{
		eventstreamevents.TopicEventStreamCommandList,
		eventstreamevents.TopicEventStreamCommandHighWatermarks,
		eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion,
		eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
		eventstreamevents.TopicEventStreamCommandRecordApprovalResolution,
	}
	for _, topic := range s.topics {
		s.SubscribeFunc(topic, s.handleCommand)
	}
	return nil
}

func (s *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	// Every subscribed command is request/response. Reject Post delivery before
	// executing reads or strong writes so purge/record commands cannot mutate
	// state without an acknowledgement channel.
	if result == nil {
		return
	}
	if ev != nil && ev.ID() == eventstreamevents.TopicEventStreamCommandRecordApprovalResolution {
		response, err := s.executeRecordApprovalResolution(ev)
		if err != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
			return
		}
		result.Set(response, nil)
		return
	}
	response, err := s.executeCommand(ev)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (s *Biz) executeRecordApprovalResolution(ev magicevent.Event) (eventstreamevents.RecordApprovalResolutionResult, error) {
	if s == nil || s.repo == nil || ev == nil || ev.Source() != approvalcommon.ModuleID {
		return eventstreamevents.RecordApprovalResolutionResult{}, common.ErrInvalidArgument
	}
	command, ok := ev.Data().(eventstreamevents.RecordApprovalResolutionCommand)
	if !ok || command.Data.Validate() != nil {
		return eventstreamevents.RecordApprovalResolutionResult{}, common.ErrInvalidArgument
	}
	fact := command.Data
	data := approvalevents.ApprovalChangedData{
		NotificationID: fact.NotificationID, ResolutionID: fact.ResolutionID, ApprovalID: fact.ApprovalID,
		WorkspaceID: fact.WorkspaceID, RunID: fact.RunID, StepID: fact.StepID, Reason: fact.Reason, Action: fact.Action,
		Context: append([]workorcheventpayload.Field(nil), fact.Context...), ResolutionContext: append([]workorcheventpayload.Field(nil), fact.ResolutionContext...),
		Principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalKind(fact.PrincipalKind), SubjectID: fact.PrincipalSubjectID},
		State:     fact.State,
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), data.WorkspaceID)
	if err != nil {
		return eventstreamevents.RecordApprovalResolutionResult{}, err
	}
	record := approvalRecord(approvalevents.TopicApprovalResolved, data)
	if err := s.appendRecord(ctx, record); err != nil {
		if !errors.Is(err, common.ErrConflict) || !s.samePersistedApprovalResolution(ctx, record) {
			return eventstreamevents.RecordApprovalResolutionResult{}, err
		}
	}
	return eventstreamevents.RecordApprovalResolutionResult{
		NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
		WorkspaceID: data.WorkspaceID, RunID: data.RunID,
	}, nil
}

func (s *Biz) samePersistedApprovalResolution(ctx context.Context, expected *eventstreammodel.EventRecord) bool {
	if s == nil || s.repo == nil || expected == nil || expected.ID == "" {
		return false
	}
	current, err := s.repo.Get(ctx, expected.ID)
	if err != nil || current == nil {
		return false
	}
	return current.ID == expected.ID && current.Topic == expected.Topic && current.SourceModule == expected.SourceModule &&
		current.SubjectType == expected.SubjectType && current.SubjectID == expected.SubjectID && current.WorkspaceID == expected.WorkspaceID &&
		current.RunID == expected.RunID && current.StepID == expected.StepID && current.Payload.Equal(expected.Payload)
}

func (s *Biz) executeCommand(ev magicevent.Event) (any, error) {
	if s == nil || s.repo == nil || ev == nil {
		return nil, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case eventstreamevents.TopicEventStreamCommandList:
		if ev.Source() != applicationCommandSource {
			return eventstreamevents.ListEventsResult{}, common.ErrInvalidArgument
		}
		cmd, ok := ev.Data().(eventstreamevents.ListEventsCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		ownerQuery, err := cmd.Query.Normalized()
		if err != nil {
			return nil, err
		}
		query := repositoryEventRecordQuery(ownerQuery)
		ctx := ev.Context()
		if query.WorkspaceID != "" {
			var err error
			ctx, err = common.BindRuntimeWorkspaceContext(ctx, query.WorkspaceID)
			if err != nil {
				return nil, err
			}
		}
		items, err := s.listEvents(ctx, query)
		if err != nil {
			return nil, err
		}
		data := make([]eventstreamevents.EventRecordData, 0, len(items))
		for _, item := range items {
			converted, convertErr := eventstreamevents.EventRecordDataFromModel(item)
			if convertErr != nil {
				return nil, convertErr
			}
			data = append(data, converted)
		}
		return eventstreamevents.ListEventsResult{Events: data}, nil
	case eventstreamevents.TopicEventStreamCommandHighWatermarks:
		if ev.Source() != applicationCommandSource {
			return eventstreamevents.HighWatermarksResult{}, common.ErrInvalidArgument
		}
		cmd, ok := ev.Data().(eventstreamevents.HighWatermarksCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		normalized, err := cmd.Normalized()
		if err != nil {
			return nil, err
		}
		items, err := s.highWatermarks(ev.Context(), normalized.WorkspaceIDs)
		response := eventstreamevents.HighWatermarksResult{HighWatermarks: items}
		if err == nil {
			err = response.ValidateFor(normalized)
		}
		return response, err
	case eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion,
		eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		return s.executeWorkspaceDeletionCommand(ev.Context(), ev)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func repositoryEventRecordQuery(query eventstreamevents.EventQuery) workorchquerycontract.EventRecordQuery {
	return workorchquerycontract.EventRecordQuery{
		WorkspaceID: query.WorkspaceID, RunID: query.RunID, Topic: query.Topic,
		ExcludeTopics: append([]string(nil), query.ExcludeTopics...), SubjectType: query.SubjectType, SubjectID: query.SubjectID,
		CursorAfterID: query.CursorAfterID, OccurredAfter: query.OccurredAfter, OccurredBefore: query.OccurredBefore,
		Limit: query.Limit, IncludePayload: query.IncludePayload,
	}
}

func (s *Biz) subscribeMemoryNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/memory/consumers/blocks/eventstream"
	memoryTopics := []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted}
	for _, topic := range memoryTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(memoryevents.MemoryChangedData)
			if !ok || event.Source() != memorycommon.ModuleID || !validMemoryChangedData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, memoryRecord(topic, event.Source(), data), nil)
		})
	}
	s.SubscribeMatchedFunc(observerID, observerID, memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(memoryevents.MemoryRecallRecordedData)
		if !ok || event.Source() != memorycommon.ModuleID || !validMemoryRecallRecordedData(data) {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, memoryRecallRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range memoryTopics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
		s.UnsubscribeMatchedFunc(observerID, memoryevents.TopicMemoryRecallRecorded)
	})
}

func (s *Biz) subscribeLLMNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/llm/consumers/blocks/eventstream"
	s.SubscribeMatchedFunc(observerID, observerID, llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(llmevents.LLMUsageRecordedData)
		if !ok || event.Source() != llmcommon.ModuleID || !validLLMUsageRecordedData(data) {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, llmUsageRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		s.UnsubscribeMatchedFunc(observerID, llmevents.TopicLLMUsageRecorded)
	})
}

func (s *Biz) subscribeResultNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/result/consumers/blocks/eventstream"
	resultTopics := []string{
		resultevents.TopicResultCreated,
		resultevents.TopicResultAccepted,
		resultevents.TopicResultRejected,
		resultevents.TopicResultSuperseded,
	}
	for _, topic := range resultTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			if event.Source() != resultcommon.ModuleID {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			data, ok := event.Data().(resultevents.ResultChangedData)
			if !ok || !validResultChangedData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, resultRecord(topic, event.Source(), data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range resultTopics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
	})
}

func (s *Biz) subscribeWorkspaceNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/workspace/consumers/blocks/eventstream"
	// TopicWorkspaceDeleted is a post-seal transport receipt. The permanent
	// Eventstream fence deliberately excludes it from the durable audit lineage;
	// EventHub projection/SSE consumers still receive the Workspace-owned fact.
	workspaceTopics := []string{
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceStateChanged,
	}
	for _, topic := range workspaceTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(workspaceevents.WorkspaceChangedData)
			if !ok || event.Source() != workspacecommon.ModuleID || !validWorkspaceChangedData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, workspaceRecord(topic, event.Source(), data), nil)
		})
	}
	s.SubscribeMatchedFunc(observerID, observerID, workspaceevents.TopicGoalDefined, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(workspaceevents.GoalDefinedData)
		if !ok || event.Source() != workspacecommon.ModuleID || !validGoalDefinedData(data) {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, goalRecord(event.Source(), data), nil)
	})
	s.SubscribeMatchedFunc(observerID, observerID, workspaceevents.TopicDoDDefined, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(workspaceevents.DoDDefinedData)
		if !ok || event.Source() != workspacecommon.ModuleID || !validDoDDefinedData(data) {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, dodRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range workspaceTopics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
		s.UnsubscribeMatchedFunc(observerID, workspaceevents.TopicGoalDefined)
		s.UnsubscribeMatchedFunc(observerID, workspaceevents.TopicDoDDefined)
	})
}

func (s *Biz) subscribeRunStateNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/run/consumers/blocks/eventstream"
	topics := []string{
		runevents.TopicRunCreated,
		runevents.TopicRunStarted,
		runevents.TopicRunBlockedOnDecision,
		runevents.TopicRunBlockedOnApproval,
		runevents.TopicRunCompleted,
		runevents.TopicRunFailed,
		runevents.TopicRunCancelled,
		runevents.TopicRunSuperseded,
		runevents.TopicRunStateChanged,
	}
	for _, topic := range topics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(runevents.RunStateChangedData)
			if !ok || event.Source() != runcommon.ModuleID || !validRunStateNotification(topic, data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, runStateRecord(topic, event.Source(), data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
	})
}

func (s *Biz) subscribeGovernanceNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/governance/consumers/blocks/eventstream"
	topics := []string{
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	}
	for _, topic := range topics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(governanceevents.GovernanceData)
			if !ok || event.Source() != governancecommon.ModuleID || !validGovernanceData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, governanceRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
	})
}

func (s *Biz) subscribeApprovalNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/approval/consumers/blocks/eventstream"
	topics := []string{
		approvalevents.TopicApprovalResolved,
	}
	for _, topic := range topics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(approvalevents.ApprovalChangedData)
			if !ok || event.Source() != approvalcommon.ModuleID || data.Validate() != nil {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, approvalRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
	})
}

func (s *Biz) subscribeDecisionNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/decision/consumers/blocks/eventstream"
	changedTopics := []string{
		decisionevents.TopicTicketCreated,
	}
	for _, topic := range changedTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(decisionevents.TicketChangedData)
			if !ok || event.Source() != decisioncommon.ModuleID || !validTicketChangedData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, decisionTicketRecord(topic, data), nil)
		})
	}
	resolvedTopics := []string{decisionevents.TopicTicketResolved}
	for _, topic := range resolvedTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(decisionevents.TicketResolvedData)
			if !ok || event.Source() != decisioncommon.ModuleID || !validTicketResolvedData(data) {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, decisionResolvedRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range changedTopics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
		for _, topic := range resolvedTopics {
			s.UnsubscribeMatchedFunc(observerID, topic)
		}
	})
}

func validMemoryChangedData(data memoryevents.MemoryChangedData) bool {
	return data.MemoryID != "" && data.WorkspaceID != ""
}

func validMemoryRecallRecordedData(data memoryevents.MemoryRecallRecordedData) bool {
	traceID := string(data.TraceID)
	return traceID != "" && traceID == strings.TrimSpace(traceID) && data.WorkspaceID != "" && data.RunID != "" && strings.TrimSpace(data.Status) != ""
}

func validLLMUsageRecordedData(data llmevents.LLMUsageRecordedData) bool {
	return data.Validate() == nil
}

func validRunStateNotification(topic string, data runevents.RunStateChangedData) bool {
	if data.Validate() != nil {
		return false
	}
	var specificTopic string
	switch data.State {
	case common.RunStatePending:
		specificTopic = runevents.TopicRunCreated
	case common.RunStateActive:
		specificTopic = runevents.TopicRunStarted
	case common.RunStateBlockedOnDecision:
		specificTopic = runevents.TopicRunBlockedOnDecision
	case common.RunStateBlockedOnApproval:
		specificTopic = runevents.TopicRunBlockedOnApproval
	case common.RunStateCompleted:
		specificTopic = runevents.TopicRunCompleted
	case common.RunStateFailed:
		specificTopic = runevents.TopicRunFailed
	case common.RunStateCancelled:
		specificTopic = runevents.TopicRunCancelled
	case common.RunStateSuperseded:
		specificTopic = runevents.TopicRunSuperseded
	default:
		return false
	}
	return topic == runevents.TopicRunStateChanged || topic == specificTopic
}

func validResultChangedData(data resultevents.ResultChangedData) bool {
	return strings.TrimSpace(data.NotificationID) != "" && data.ResultID != "" && data.WorkspaceID != "" && data.RunID != "" && data.State != ""
}

func validWorkspaceChangedData(data workspaceevents.WorkspaceChangedData) bool {
	return data.WorkspaceID != "" && data.State != ""
}

func validGoalDefinedData(data workspaceevents.GoalDefinedData) bool {
	return data.GoalID != "" && data.WorkspaceID != "" && strings.TrimSpace(data.DefinitionRevision) != "" && strings.TrimSpace(data.LineageState) != ""
}

func validDoDDefinedData(data workspaceevents.DoDDefinedData) bool {
	return data.DoDID != "" && data.WorkspaceID != "" && len(data.Items) > 0 && data.State != "" && strings.TrimSpace(data.DefinitionRevision) != "" && strings.TrimSpace(data.LineageState) != ""
}

func validGovernanceData(data governanceevents.GovernanceData) bool {
	trace := data.Trace
	return trace.ID != "" && trace.WorkspaceID != "" && trace.RunID != "" && strings.TrimSpace(trace.ItemType) != "" && strings.TrimSpace(trace.ItemID) != "" && strings.TrimSpace(string(trace.Status)) != ""
}

func validTicketChangedData(data decisionevents.TicketChangedData) bool {
	return strings.TrimSpace(data.NotificationID) != "" && data.TicketID != "" && data.WorkspaceID != "" && data.RunID != "" && data.State != ""
}

func validTicketResolvedData(data decisionevents.TicketResolvedData) bool {
	return strings.TrimSpace(data.NotificationID) != "" && validTicketChangedData(data.Ticket) && data.ResolutionID != "" && strings.TrimSpace(data.ResolutionType) != ""
}

func (s *Biz) finishRecordAppend(event magicevent.Event, record *eventstreammodel.EventRecord, inputErr error) {
	err := s.appendNotificationRecord(event, record, inputErr)
	if err != nil {
		topic, source := "", ""
		if event != nil {
			topic, source = event.ID(), event.Source()
		}
		slog.Error("workorch eventstream append failed", "topic", topic, "source", source, "err", err)
	}
}

func (s *Biz) appendNotificationRecord(event magicevent.Event, record *eventstreammodel.EventRecord, inputErr error) error {
	if event == nil {
		return common.ErrInvalidArgument
	}
	err := inputErr
	if err == nil {
		for attempt := 0; attempt < 3; attempt++ {
			err = s.appendRecord(event.Context(), record)
			if err == nil {
				return nil
			}
			if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrInvalidArgument) {
				break
			}
		}
	}
	// Stable owner notification identities are idempotent only when every
	// persisted source/topic/scope/payload field still matches. A collision with
	// drift remains a visible conflict; arbitrary record IDs never gain replay
	// semantics merely because the repository reports a conflict.
	if errors.Is(err, common.ErrConflict) && hasStableNotificationRecordID(record) &&
		s.samePersistedRecord(event.Context(), record) {
		return nil
	}
	return err
}

func hasStableNotificationRecordID(record *eventstreammodel.EventRecord) bool {
	if record == nil || record.ID == "" {
		return false
	}
	id := string(record.ID)
	for _, prefix := range []string{
		"result-notification/",
		"decision-notification/",
		"approval-notification/",
		stableRunNotificationRecordIDPrefix,
		stableMemoryRecallRecordIDPrefix,
		stableLLMUsageRecordIDPrefix,
	} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func (s *Biz) samePersistedRecord(ctx context.Context, expected *eventstreammodel.EventRecord) bool {
	if s == nil || s.repo == nil || expected == nil || expected.ID == "" {
		return false
	}
	if expected.WorkspaceID != "" {
		bound, err := common.BindRuntimeWorkspaceContext(ctx, expected.WorkspaceID)
		if err != nil {
			return false
		}
		ctx = bound
	}
	current, err := s.repo.Get(ctx, expected.ID)
	if err != nil || current == nil {
		return false
	}
	return current.ID == expected.ID && current.Version == expected.Version && current.Topic == expected.Topic &&
		current.SourceModule == expected.SourceModule && current.SubjectType == expected.SubjectType && current.SubjectID == expected.SubjectID &&
		current.WorkspaceID == expected.WorkspaceID && current.RunID == expected.RunID && current.StepID == expected.StepID &&
		current.Payload.Equal(expected.Payload)
}

func (s *Biz) appendRecord(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.repo == nil || record == nil || record.Topic == "" {
		return common.ErrInvalidArgument
	}
	s.appendMu.Lock()
	defer s.appendMu.Unlock()
	if record.WorkspaceID != "" {
		hidden, err := s.workspaceDeletionHidden(ctx, record.WorkspaceID)
		if err != nil {
			return err
		}
		if hidden {
			return common.ErrNotFound
		}
	}
	if record.ID == "" {
		record.ID = common.EventRecordID(fmt.Sprintf("evt-%d-%d", time.Now().UTC().UnixNano(), eventRecordIDSequence.Add(1)))
	}
	if record.Version == "" {
		record.Version = recordpayload.WorkorchEventVersion
	}
	if record.OccurredAt == "" {
		record.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		canonicalOccurredAt, err := workorcheventrecordcontract.CanonicalOccurredAt(record.OccurredAt)
		if err != nil {
			return err
		}
		record.OccurredAt = canonicalOccurredAt
	}
	record.Payload = recordpayload.SanitizeAuditPayload(record.Payload)
	record.PayloadSummary = recordpayload.SanitizeAuditPayload(record.PayloadSummary)
	if record.StreamPosition == 0 {
		position, err := s.reserveStreamPosition(ctx)
		if err != nil {
			return err
		}
		record.StreamPosition = position
	}
	if record.WorkspaceID != "" {
		return s.appendWorkspaceRecord(ctx, record)
	}
	if err := validateEventRecordForAppend(record); err != nil {
		return err
	}
	if err := s.repo.Append(ctx, record); err != nil {
		return err
	}
	return s.publishRecordAppended(ctx, record)
}

func (s *Biz) reserveStreamPosition(ctx context.Context) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, common.ErrInvalidArgument
	}
	return s.repo.ReserveEventRecordStreamPosition(ctx)
}

func (s *Biz) appendWorkspaceRecord(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.repo == nil || record == nil || record.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if s.revisions == nil {
		s.revisions = map[common.WorkspaceID]int64{}
	}
	current, ok := s.revisions[record.WorkspaceID]
	if !ok {
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, record.WorkspaceID)
		if err != nil {
			return err
		}
		items, err := s.repo.List(scopedCtx, workorchquerycontract.EventRecordQuery{WorkspaceID: record.WorkspaceID, Limit: 1})
		if err != nil {
			return err
		}
		if len(items) == 1 && items[0] != nil {
			current = items[0].Revision
		}
	}
	record.Revision = current + 1
	if err := validateEventRecordForAppend(record); err != nil {
		return err
	}
	if err := s.repo.Append(ctx, record); err != nil {
		return err
	}
	s.revisions[record.WorkspaceID] = record.Revision
	return s.publishRecordAppended(ctx, record)
}

func validateEventRecordForAppend(record *eventstreammodel.EventRecord) error {
	data, err := eventstreamevents.EventRecordDataFromModel(record)
	if err != nil {
		return err
	}
	return data.Validate()
}

func decisionTicketRecord(topic string, data decisionevents.TicketChangedData) *eventstreammodel.EventRecord {
	ret := &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: decisioncommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(data.TicketID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload:      (recordpayload.DecisionTicketPayloadData{Reason: data.Reason, State: data.State}).EventPayload(),
	}
	if data.NotificationID != "" {
		ret.ID = common.EventRecordID(data.NotificationID)
	}
	return ret
}

func approvalRecord(topic string, data approvalevents.ApprovalChangedData) *eventstreammodel.EventRecord {
	ret := &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: approvalcommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(data.ApprovalID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: (recordpayload.ApprovalPayloadData{
			Reason: data.Reason, Action: data.Action, Context: workorcheventpayload.New(data.Context...), State: data.State,
			ResolutionID: data.ResolutionID, ResolutionContext: workorcheventpayload.New(data.ResolutionContext...),
			ResolvedByKind: string(data.Principal.Kind), ResolvedBy: data.Principal.SubjectID,
		}).EventPayload(),
	}
	if data.NotificationID != "" {
		ret.ID = common.EventRecordID(data.NotificationID)
	}
	return ret
}

func governanceRecord(topic string, data governanceevents.GovernanceData) *eventstreammodel.EventRecord {
	trace := data.Trace
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: governancecommon.ModuleID,
		SubjectType:  "auto_governance_trace",
		SubjectID:    string(trace.ID),
		WorkspaceID:  trace.WorkspaceID,
		RunID:        trace.RunID,
		StepID:       trace.StepID,
		Payload: (recordpayload.AutoGovernancePayloadData{
			TraceID: trace.ID, ItemType: trace.ItemType, ItemID: trace.ItemID, Action: trace.Action,
			RiskLevel: trace.RiskLevel, Confidence: trace.Confidence, PolicyMatched: trace.PolicyMatched, PolicyBlocked: trace.PolicyBlocked,
			BudgetBefore: trace.BudgetBefore, BudgetAfter: trace.BudgetAfter, LLMProviderID: trace.LLMProviderID, LLMModel: trace.LLMModel,
			LLMDecisionPreview: trace.LLMDecisionPreview, PromptFingerprint: trace.PromptFingerprint,
			Status: string(trace.Status), Reason: trace.Reason, EscalationReason: data.EscalationReason,
		}).EventPayload(),
	}
}

func runStateRecord(topic string, source string, data runevents.RunStateChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:           stableNotificationRecordID(stableRunNotificationRecordIDPrefix, topic, data.WorkspaceID, data.RunID, data.CausationID),
		Topic:        topic,
		SourceModule: source,
		SubjectType:  "run",
		SubjectID:    string(data.RunID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		Payload: (recordpayload.RunPayloadData{
			GoalID: data.GoalID, DoDID: data.DoDID, State: data.State,
			BlockingTicketID: data.BlockingTicketID, BlockingApprovalID: data.BlockingApprovalID, FailureReason: data.FailureReason,
			RuntimeStateRevision: int(data.RuntimeStateRevision), RuntimeStateRef: data.RuntimeStateRef,
		}).EventPayload(),
	}
}

func workspaceRecord(topic string, source string, data workspaceevents.WorkspaceChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: source,
		SubjectType:  "workspace",
		SubjectID:    string(data.WorkspaceID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		Payload: (recordpayload.WorkspacePayloadData{
			Title: data.Title, Description: data.Description, DeliverableDirectory: data.DeliverableDirectory,
			InternalDirectory: data.InternalDirectory, State: data.State, ActiveRunID: data.ActiveRunID,
		}).EventPayload(),
	}
}

func goalRecord(source string, data workspaceevents.GoalDefinedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        workspaceevents.TopicGoalDefined,
		SourceModule: source,
		SubjectType:  "goal",
		SubjectID:    string(data.GoalID),
		WorkspaceID:  data.WorkspaceID,
		Payload: (recordpayload.GoalPayloadData{
			Text: data.Text, DefinitionRevision: data.DefinitionRevision, PreviousDefinitionHash: data.PreviousDefinitionHash,
			PreviousGoalID: data.PreviousGoalID, LineageState: data.LineageState,
		}).EventPayload(),
	}
}

func dodRecord(source string, data workspaceevents.DoDDefinedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        workspaceevents.TopicDoDDefined,
		SourceModule: source,
		SubjectType:  "dod",
		SubjectID:    string(data.DoDID),
		WorkspaceID:  data.WorkspaceID,
		Payload: (recordpayload.DoDPayloadData{
			Items: data.Items, State: data.State, DefinitionRevision: data.DefinitionRevision,
			PreviousDefinitionHash: data.PreviousDefinitionHash, PreviousDoDID: data.PreviousDoDID, LineageState: data.LineageState,
		}).EventPayload(),
	}
}

func resultRecord(topic string, source string, data resultevents.ResultChangedData) *eventstreammodel.EventRecord {
	artifactRefs := make([]string, 0, len(data.ArtifactRefs))
	for _, ref := range data.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	ret := &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: source,
		SubjectType:  "result",
		SubjectID:    string(data.ResultID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		Payload: (recordpayload.ResultPayloadData{
			GoalID: data.GoalID, DoDID: data.DoDID, State: data.State, Title: data.Title, Summary: data.Summary,
			SourceType: data.SourceType, SourceRef: data.SourceRef, EvidenceRefs: data.EvidenceRefs, ArtifactRefs: artifactRefs,
		}).EventPayload(),
	}
	if data.NotificationID != "" {
		ret.ID = common.EventRecordID(data.NotificationID)
	}
	return ret
}

func memoryRecord(topic string, source string, data memoryevents.MemoryChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "memory", SubjectID: string(data.MemoryID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		Payload: (recordpayload.MemoryPayloadData{
			MemoryType: data.MemoryType, Summary: data.Summary, Content: data.Content, SourceType: data.SourceType,
			SourceRef: data.SourceRef, Tags: data.Tags, Importance: data.Importance, Confidence: data.Confidence, Visibility: data.Visibility,
		}).EventPayload(),
	}
}

func memoryRecallRecord(source string, data memoryevents.MemoryRecallRecordedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:    stableNotificationRecordID(stableMemoryRecallRecordIDPrefix, memoryevents.TopicMemoryRecallRecorded, data.WorkspaceID, data.RunID, string(data.TraceID)),
		Topic: memoryevents.TopicMemoryRecallRecorded, SourceModule: source, SubjectType: "memory_recall", SubjectID: string(data.TraceID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, OccurredAt: data.OccurredAt,
		Payload: (recordpayload.MemoryRecallPayloadData{
			TraceID: data.TraceID, Stage: data.Stage, Source: data.Source, Policy: data.Policy, Status: data.Status,
			ProviderID: data.ProviderID, Operation: data.Operation, RetrievedMemoryIDs: data.RetrievedMemoryIDs,
			SelectionEvidence: workorchmemorycontract.CloneRecallSelectionEvidence(data.SelectionEvidence), Reason: data.Reason,
		}).EventPayload(),
	}
}

func llmUsageRecord(source string, data llmevents.LLMUsageRecordedData) *eventstreammodel.EventRecord {
	subjectID := data.RequestID
	if subjectID == "" {
		subjectID = string(data.RunID)
	}
	return &eventstreammodel.EventRecord{
		ID:           stableNotificationRecordID(stableLLMUsageRecordIDPrefix, llmevents.TopicLLMUsageRecorded, data.WorkspaceID, data.RunID, data.RequestID),
		Topic:        llmevents.TopicLLMUsageRecorded,
		SourceModule: source,
		SubjectType:  "llm_usage",
		SubjectID:    subjectID,
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       common.StepID(data.StepID),
		OccurredAt:   data.OccurredAt,
		Payload: (recordpayload.LLMUsagePayloadData{
			RequestID:             data.RequestID,
			LogicalRequestID:      data.LogicalRequestID,
			PhysicalCallID:        data.PhysicalCallID,
			WorkspaceID:           string(data.WorkspaceID),
			RunID:                 string(data.RunID),
			StepID:                data.StepID,
			ProviderID:            data.Target.ProviderID,
			Provider:              data.Provider,
			APISurface:            data.APISurface,
			Model:                 data.Target.ModelID,
			Operation:             data.Operation,
			CapabilityRevision:    data.CapabilityRevision,
			ConfigurationRevision: data.ConfigurationRevision,
			StablePrefixHash:      data.StablePrefixHash,
			Kind:                  data.Kind,
			InputTokens:           data.InputTokens,
			OutputTokens:          data.OutputTokens,
			TotalTokens:           data.TotalTokens,
			Estimated:             data.Estimated,
			UsageSource:           data.UsageSource,
			RequestBytes:          data.RequestBytes,
			ResponseBytes:         data.ResponseBytes,
			PhysicalResponseMode:  data.PhysicalResponseMode,
			ContextBudget:         data.ContextBudget,
			CacheUsage:            data.CacheUsage,
			StartedAt:             data.StartedAt,
			CompletedAt:           data.CompletedAt,
			DurationMs:            data.DurationMs,
			Status:                data.Status,
			FailureClass:          data.FailureClass,
			ProviderSubmitted:     data.ProviderSubmitted,
		}).EventPayload(),
	}
}

func stableNotificationRecordID(prefix, topic string, workspaceID common.WorkspaceID, runID common.RunID, identity string) common.EventRecordID {
	if prefix == "" || topic == "" || topic != strings.TrimSpace(topic) || identity == "" || identity != strings.TrimSpace(identity) {
		return ""
	}
	// Length-prefix every field so arbitrary owner IDs cannot create delimiter
	// ambiguity. Source and payload are deliberately excluded: they must collide
	// with the same stable identity and be rejected as drift. Topic is included
	// because Run legitimately publishes a state-specific and canonical fact with
	// the same causation identity.
	material := fmt.Sprintf(
		"eventstream/stable-notification/v1\n%d:%s\n%d:%s\n%d:%s\n%d:%s",
		len(topic), topic,
		len(workspaceID), workspaceID,
		len(runID), runID,
		len(identity), identity,
	)
	digest := sha256.Sum256([]byte(material))
	return common.EventRecordID(fmt.Sprintf("%s%x", prefix, digest))
}

func decisionResolvedRecord(topic string, data decisionevents.TicketResolvedData) *eventstreammodel.EventRecord {
	ret := &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: decisioncommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(data.Ticket.TicketID),
		WorkspaceID:  data.Ticket.WorkspaceID,
		RunID:        data.Ticket.RunID,
		StepID:       data.Ticket.StepID,
		Payload: (recordpayload.DecisionResolutionPayloadData{
			DecisionTicketPayloadData: recordpayload.DecisionTicketPayloadData{Reason: data.Ticket.Reason, State: data.Ticket.State},
			ResolutionID:              data.ResolutionID, Type: data.ResolutionType, Payload: workorcheventpayload.New(data.ResolutionPayload...),
			ContinuationAction: data.ContinuationAction, ResumeRunID: data.ResumeRunID, SuccessorRunID: data.SuccessorRunID,
		}).EventPayload(),
	}
	if data.NotificationID != "" {
		ret.ID = common.EventRecordID(data.NotificationID)
	}
	return ret
}

func (s *Biz) publishRecordAppended(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.EventHub() == nil || record == nil {
		return common.ErrInvalidArgument
	}
	data, err := eventstreamevents.EventRecordDataFromModel(record)
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		modulecommon.ModuleID,
		eventstreamevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.EventRecordAppendedData{Record: data},
	)
	event.BindLaneKey(events.EventHubLaneKeyFor(eventstreamevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: record.SubjectType,
		SubjectID:   record.SubjectID,
		WorkspaceID: string(record.WorkspaceID),
		RunID:       string(record.RunID),
		StepID:      string(record.StepID),
	}))
	s.PostEvent(event)
	return nil
}

func (s *Biz) Stop() {
	if s == nil {
		return
	}
	for idx := len(s.topics) - 1; idx >= 0; idx-- {
		s.UnsubscribeFunc(s.topics[idx])
	}
	s.topics = nil
	unsubscribers := s.unsubscribers
	s.unsubscribers = nil
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

func (s *Biz) listEvents(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if s == nil || s.repo == nil || s.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	if query.WorkspaceID != "" {
		hidden, err := s.workspaceDeletionHidden(ctx, query.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, nil
		}
	}
	items, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, err
	}
	if query.WorkspaceID != "" || len(items) == 0 {
		return items, nil
	}
	visible := items[:0]
	visibility := map[common.WorkspaceID]bool{}
	for _, item := range items {
		if item == nil || item.WorkspaceID == "" {
			visible = append(visible, item)
			continue
		}
		hidden, known := visibility[item.WorkspaceID]
		if !known {
			hidden, err = s.workspaceDeletionHidden(ctx, item.WorkspaceID)
			if err != nil {
				return nil, err
			}
			visibility[item.WorkspaceID] = hidden
		}
		if !hidden {
			visible = append(visible, item)
		}
	}
	if len(visible) == 0 {
		return nil, nil
	}
	return visible, nil
}
