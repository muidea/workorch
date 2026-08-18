package eventstream

import (
	"context"
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
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type Service struct {
	basebiz.Base
	repo          repository.EventRecordRepository
	unsubscribers []func()
	topics        []string
	revisionMu    sync.Mutex
	revisions     map[common.WorkspaceID]int64
}

// Dependencies are the EventStream owner's construction inputs. Keeping this
// in biz prevents the module lifecycle bridge from owning EventHub types.
type Dependencies struct {
	EventRecords      repository.EventRecordRepository
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

var eventRecordIDSequence atomic.Uint64

func NewService(repo repository.EventRecordRepository, hub magicevent.Hub, background magictask.BackgroundRoutine) *Service {
	return &Service{Base: basebiz.New(modulecommon.ModuleID, hub, background), repo: repo}
}

func NewServiceWithDependencies(deps Dependencies) *Service {
	return NewService(deps.EventRecords, deps.EventHub, deps.BackgroundRoutine)
}

func (s *Service) HighWatermarks(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
	if s == nil || s.repo == nil {
		return nil, common.ErrInvalidArgument
	}
	revisions := map[common.WorkspaceID]int64{}
	for _, workspaceID := range workspaceIDs {
		if workspaceID == "" {
			continue
		}
		revisions[workspaceID] = 0
	}
	items, err := s.repo.List(ctx, workorchquerycontract.EventRecordQuery{})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		current, ok := revisions[item.WorkspaceID]
		if ok && item.Revision > current {
			revisions[item.WorkspaceID] = item.Revision
		}
	}
	ret := make([]eventstreamevents.WorkspaceHighWatermark, 0, len(revisions))
	for workspaceID, revision := range revisions {
		ret = append(ret, eventstreamevents.WorkspaceHighWatermark{WorkspaceID: workspaceID, Revision: revision})
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i].WorkspaceID) < string(ret[j].WorkspaceID) })
	return ret, nil
}

func (s *Service) Start() error {
	if s == nil || s.repo == nil || s.EventHub() == nil {
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
		eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
	}
	for _, topic := range s.topics {
		s.SubscribeFunc(topic, s.handleCommand)
	}
	return nil
}

func (s *Service) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := s.executeCommand(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (s *Service) executeCommand(ev magicevent.Event) (eventstreamevents.EventStreamCommandResult, error) {
	if s == nil || s.repo == nil {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case eventstreamevents.TopicEventStreamCommandList:
		cmd, ok := ev.Data().(eventstreamevents.ListEventsCommand)
		if !ok {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		query := cmd.Query
		ctx := ev.Context()
		if query.WorkspaceID != "" {
			var err error
			ctx, err = common.BindRuntimeWorkspaceContext(ctx, query.WorkspaceID)
			if err != nil {
				return eventstreamevents.EventStreamCommandResult{}, err
			}
		}
		items, err := s.ListEvents(ctx, query)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		data := make([]eventstreamevents.EventRecordData, 0, len(items))
		for _, item := range items {
			converted, convertErr := eventstreamevents.EventRecordDataFromModel(item)
			if convertErr != nil {
				return eventstreamevents.EventStreamCommandResult{}, convertErr
			}
			data = append(data, converted)
		}
		return eventstreamevents.EventStreamCommandResult{Events: data}, nil
	case eventstreamevents.TopicEventStreamCommandHighWatermarks:
		cmd, ok := ev.Data().(eventstreamevents.HighWatermarksCommand)
		if !ok {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		items, err := s.HighWatermarks(ev.Context(), cmd.WorkspaceIDs)
		return eventstreamevents.EventStreamCommandResult{HighWatermarks: items}, err
	case eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		cmd, ok := ev.Data().(eventstreamevents.PurgeWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		deleted, err := s.PurgeWorkspaceEvents(ctx, cmd.WorkspaceID)
		return eventstreamevents.EventStreamCommandResult{DeletedCount: deleted}, err
	default:
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
}

func (s *Service) subscribeMemoryNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/memory/consumers/blocks/eventstream"
	memoryTopics := []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted}
	for _, topic := range memoryTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(memoryevents.MemoryChangedData)
			if !ok {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, memoryRecord(topic, event.Source(), data), nil)
		})
	}
	s.SubscribeMatchedFunc(observerID, observerID, memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(memoryevents.MemoryRecallRecordedData)
		if !ok {
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

func (s *Service) subscribeLLMNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/llm/consumers/blocks/eventstream"
	s.SubscribeMatchedFunc(observerID, observerID, llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(llmevents.LLMUsageRecordedData)
		if !ok {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, llmUsageRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		s.UnsubscribeMatchedFunc(observerID, llmevents.TopicLLMUsageRecorded)
	})
}

func (s *Service) subscribeResultNotifications() {
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
			if !ok {
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

func (s *Service) subscribeWorkspaceNotifications() {
	if s == nil || s.EventHub() == nil {
		return
	}
	const observerID = "/workspace/consumers/blocks/eventstream"
	workspaceTopics := []string{
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceStateChanged,
		workspaceevents.TopicWorkspaceDeleted,
	}
	for _, topic := range workspaceTopics {
		topic := topic
		s.SubscribeMatchedFunc(observerID, observerID, topic, func(event magicevent.Event, _ magicevent.Result) {
			data, ok := event.Data().(workspaceevents.WorkspaceChangedData)
			if !ok {
				s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, workspaceRecord(topic, event.Source(), data), nil)
		})
	}
	s.SubscribeMatchedFunc(observerID, observerID, workspaceevents.TopicGoalDefined, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(workspaceevents.GoalDefinedData)
		if !ok {
			s.finishRecordAppend(event, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, goalRecord(event.Source(), data), nil)
	})
	s.SubscribeMatchedFunc(observerID, observerID, workspaceevents.TopicDoDDefined, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(workspaceevents.DoDDefinedData)
		if !ok {
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

func (s *Service) subscribeRunStateNotifications() {
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
			if !ok {
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

func (s *Service) subscribeGovernanceNotifications() {
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
			if !ok {
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

func (s *Service) subscribeApprovalNotifications() {
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
			if !ok {
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

func (s *Service) subscribeDecisionNotifications() {
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
			if !ok {
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
			if !ok {
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

func (s *Service) finishRecordAppend(event magicevent.Event, record *eventstreammodel.EventRecord, inputErr error) {
	err := inputErr
	if err == nil {
		err = s.appendRecord(event.Context(), record)
	}
	// Only Result notifications carry a stable owner-derived EventRecord ID.
	// A conflict for that identity is an idempotent replay; conflicts for any
	// other event must remain visible instead of being silently swallowed.
	if errors.Is(err, common.ErrConflict) && record != nil && strings.HasPrefix(string(record.ID), "result-notification/") {
		return
	}
	if err != nil {
		slog.Error("workorch eventstream append failed", "topic", event.ID(), "source", event.Source(), "err", err)
	}
}

func (s *Service) appendRecord(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.repo == nil || record == nil || record.Topic == "" {
		return common.ErrInvalidArgument
	}
	if record.ID == "" {
		record.ID = common.EventRecordID(fmt.Sprintf("evt-%d-%d", time.Now().UTC().UnixNano(), eventRecordIDSequence.Add(1)))
	}
	if record.Version == "" {
		record.Version = recordpayload.WorkorchEventVersion
	}
	if record.OccurredAt == "" {
		record.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if record.WorkspaceID != "" {
		return s.appendWorkspaceRecord(ctx, record)
	}
	if err := s.repo.Append(ctx, record); err != nil {
		return err
	}
	return s.publishRecordAppended(ctx, record)
}

func (s *Service) appendWorkspaceRecord(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.repo == nil || record == nil || record.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	s.revisionMu.Lock()
	defer s.revisionMu.Unlock()
	if s.revisions == nil {
		s.revisions = map[common.WorkspaceID]int64{}
	}
	current, ok := s.revisions[record.WorkspaceID]
	if !ok {
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, record.WorkspaceID)
		if err != nil {
			return err
		}
		items, err := s.repo.List(scopedCtx, workorchquerycontract.EventRecordQuery{WorkspaceID: record.WorkspaceID})
		if err != nil {
			return err
		}
		for _, item := range items {
			if item != nil && item.Revision > current {
				current = item.Revision
			}
		}
	}
	record.Revision = current + 1
	if err := s.repo.Append(ctx, record); err != nil {
		return err
	}
	s.revisions[record.WorkspaceID] = record.Revision
	return s.publishRecordAppended(ctx, record)
}

func decisionTicketRecord(topic string, data decisionevents.TicketChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: decisioncommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(data.TicketID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload:      (recordpayload.DecisionTicketPayloadData{Reason: data.Reason, State: data.State}).EventPayload(),
	}
}

func approvalRecord(topic string, data approvalevents.ApprovalChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: approvalcommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(data.ApprovalID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: (recordpayload.ApprovalPayloadData{
			Reason: data.Reason, Action: data.Action, Context: workorcheventpayload.New(data.Context...), State: data.State,
		}).EventPayload(),
	}
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
			Status: trace.Status, Reason: trace.Reason, EscalationReason: data.EscalationReason,
		}).EventPayload(),
	}
}

func runStateRecord(topic string, source string, data runevents.RunStateChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
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
		Topic: memoryevents.TopicMemoryRecallRecorded, SourceModule: source, SubjectType: "memory_recall", SubjectID: string(data.TraceID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, OccurredAt: data.OccurredAt,
		Payload: (recordpayload.MemoryRecallPayloadData{
			TraceID: data.TraceID, Stage: data.Stage, Source: data.Source, Policy: data.Policy, Status: data.Status,
			ProviderID: data.ProviderID, Operation: data.Operation, RetrievedMemoryIDs: data.RetrievedMemoryIDs, Reason: data.Reason,
		}).EventPayload(),
	}
}

func llmUsageRecord(source string, data llmevents.LLMUsageRecordedData) *eventstreammodel.EventRecord {
	subjectID := data.RequestID
	if subjectID == "" {
		subjectID = string(data.RunID)
	}
	return &eventstreammodel.EventRecord{
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

func decisionResolvedRecord(topic string, data decisionevents.TicketResolvedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
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
}

func (s *Service) publishRecordAppended(ctx context.Context, record *eventstreammodel.EventRecord) error {
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

func (s *Service) Stop() {
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

func (s *Service) ListEvents(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, common.ErrInvalidArgument
	}
	return s.repo.List(ctx, query)
}

func (s *Service) PurgeWorkspaceEvents(ctx context.Context, workspaceID common.WorkspaceID) (int, error) {
	if s == nil || s.repo == nil || workspaceID == "" {
		return 0, common.ErrInvalidArgument
	}
	items, err := s.repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := s.repo.Delete(ctx, item.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
