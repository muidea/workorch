package eventstream

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Service struct {
	repo          repository.EventRecordRepository
	commandHub    magicevent.Hub
	unsubscribers []func()
	revisionMu    sync.Mutex
	revisions     map[common.WorkspaceID]int64
}

var eventRecordIDSequence atomic.Uint64

func NewService(repo repository.EventRecordRepository, commandHub magicevent.Hub) *Service {
	return &Service{repo: repo, commandHub: commandHub}
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
	items, err := s.repo.List(ctx, repository.EventRecordQuery{})
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
	if s == nil || s.repo == nil || s.commandHub == nil {
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
	s.subscribeOrchestratorDetailNotifications()
	return nil
}

func (s *Service) subscribeOrchestratorDetailNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/orchestrator/consumers/kernel/eventstream/detail"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	patchTopics := []string{
		orchestratorevents.TopicPatchPreviewed,
		orchestratorevents.TopicPatchApplied,
		orchestratorevents.TopicPatchFailed,
		orchestratorevents.TopicPatchReverted,
	}
	for _, topic := range patchTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(orchestratorevents.PatchChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, patchRecord(topic, event.Source(), data), nil)
		})
	}
	continuationTopics := []string{
		orchestratorevents.TopicArtifactContinuationCreated,
		orchestratorevents.TopicArtifactContinuationChunkRecorded,
		orchestratorevents.TopicArtifactContinuationChunkRejected,
		orchestratorevents.TopicArtifactContinuationAssembled,
		orchestratorevents.TopicArtifactContinuationApplied,
		orchestratorevents.TopicArtifactContinuationFailed,
	}
	for _, topic := range continuationTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(orchestratorevents.ArtifactContinuationChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, artifactContinuationRecord(topic, event.Source(), data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range patchTopics {
			observer.Unsubscribe(topic)
		}
		for _, topic := range continuationTopics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) subscribeMemoryNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/memory/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	memoryTopics := []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted}
	for _, topic := range memoryTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(memoryevents.MemoryChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, memoryRecord(topic, event.Source(), data), nil)
		})
	}
	observer.Subscribe(memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(memoryevents.MemoryRecallRecordedData)
		if !ok {
			s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, result, memoryRecallRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range memoryTopics {
			observer.Unsubscribe(topic)
		}
		observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded)
	})
}

func (s *Service) subscribeLLMNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/llm/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	observer.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(llmevents.LLMUsageRecordedData)
		if !ok {
			s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, result, llmUsageRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		observer.Unsubscribe(llmevents.TopicLLMUsageRecorded)
	})
}

func (s *Service) subscribeResultNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/result/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	resultTopics := []string{
		resultevents.TopicResultCreated,
		resultevents.TopicResultAccepted,
		resultevents.TopicResultRejected,
		resultevents.TopicResultSuperseded,
	}
	for _, topic := range resultTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(resultevents.ResultChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, resultRecord(topic, event.Source(), data), nil)
		})
	}
	observer.Subscribe(resultevents.TopicArtifactCreated, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(resultevents.ArtifactCreatedData)
		if !ok {
			s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, result, artifactRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range resultTopics {
			observer.Unsubscribe(topic)
		}
		observer.Unsubscribe(resultevents.TopicArtifactCreated)
	})
}

func (s *Service) subscribeWorkspaceNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/workspace/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	workspaceTopics := []string{
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceStateChanged,
		workspaceevents.TopicWorkspaceDeleted,
	}
	for _, topic := range workspaceTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(workspaceevents.WorkspaceChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, workspaceRecord(topic, event.Source(), data), nil)
		})
	}
	observer.Subscribe(workspaceevents.TopicGoalDefined, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(workspaceevents.GoalDefinedData)
		if !ok {
			s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, result, goalRecord(event.Source(), data), nil)
	})
	observer.Subscribe(workspaceevents.TopicDoDDefined, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(workspaceevents.DoDDefinedData)
		if !ok {
			s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
			return
		}
		s.finishRecordAppend(event, result, dodRecord(event.Source(), data), nil)
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range workspaceTopics {
			observer.Unsubscribe(topic)
		}
		observer.Unsubscribe(workspaceevents.TopicGoalDefined)
		observer.Unsubscribe(workspaceevents.TopicDoDDefined)
	})
}

func (s *Service) subscribeRunStateNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/run/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	topics := []string{
		runevents.TopicRunCreated,
		runevents.TopicRunStarted,
		runevents.TopicRunResumed,
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
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(runevents.RunStateChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, runStateRecord(topic, event.Source(), data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) subscribeGovernanceNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/governance/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	topics := []string{
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	}
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(governanceevents.GovernanceData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, governanceRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) subscribeApprovalNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/approval/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	topics := []string{
		approvalevents.TopicApprovalCreated,
		approvalevents.TopicApprovalApproved,
		approvalevents.TopicApprovalRejected,
		approvalevents.TopicApprovalResolved,
		approvalevents.TopicApprovalSuperseded,
	}
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(approvalevents.ApprovalChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, approvalRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) subscribeDecisionNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/decision/consumers/kernel/eventstream"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	changedTopics := []string{
		decisionevents.TopicTicketCreated,
		decisionevents.TopicTicketUpdated,
		decisionevents.TopicTicketSuperseded,
		decisionevents.TopicDecisionTicketCreated,
		decisionevents.TopicDecisionTicketSuperseded,
	}
	for _, topic := range changedTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(decisionevents.TicketChangedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, decisionTicketRecord(topic, data), nil)
		})
	}
	resolvedTopics := []string{decisionevents.TopicTicketResolved, decisionevents.TopicDecisionTicketResolved}
	for _, topic := range resolvedTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			data, ok := event.Data().(decisionevents.TicketResolvedData)
			if !ok {
				s.finishRecordAppend(event, result, nil, common.ErrInvalidArgument)
				return
			}
			s.finishRecordAppend(event, result, decisionResolvedRecord(topic, data), nil)
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range changedTopics {
			observer.Unsubscribe(topic)
		}
		for _, topic := range resolvedTopics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) finishRecordAppend(event magicevent.Event, result magicevent.Result, record *eventstreammodel.EventRecord, inputErr error) {
	err := inputErr
	if err == nil {
		err = s.appendRecord(event.Context(), record)
	}
	if err != nil {
		slog.Error("workorch eventstream append failed", "topic", event.ID(), "source", event.Source(), "err", err)
		if result != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		}
		return
	}
	if result != nil {
		result.Set(nil, nil)
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
		record.Version = events.WorkorchEventVersion
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
		items, err := s.repo.List(common.ContextWithRuntimeWorkspaceID(ctx, record.WorkspaceID), repository.EventRecordQuery{WorkspaceID: record.WorkspaceID})
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
		Payload:      (events.DecisionTicketPayloadData{Reason: data.Reason, State: data.State}).EventPayload(),
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
		Payload: (events.ApprovalPayloadData{
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
		Payload: (events.AutoGovernancePayloadData{
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
		Payload: (events.RunPayloadData{
			GoalID: data.GoalID, DoDID: data.DoDID, State: data.State,
			BlockingTicketID: data.BlockingTicketID, BlockingApprovalID: data.BlockingApprovalID, FailureReason: data.FailureReason,
			RuntimeStateRevision: int(data.RuntimeStateRevision), RuntimeConvergenceState: string(data.RuntimeConvergenceState),
			RuntimeAction: string(data.RuntimeAction), RuntimeFocusID: data.RuntimeFocusID, AuthoritySnapshotID: data.AuthoritySnapshotID,
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
		Payload: (events.WorkspacePayloadData{
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
		Payload: (events.GoalPayloadData{
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
		Payload: (events.DoDPayloadData{
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
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: source,
		SubjectType:  "result",
		SubjectID:    string(data.ResultID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		Payload: (events.ResultPayloadData{
			GoalID: data.GoalID, DoDID: data.DoDID, State: data.State, Title: data.Title, Summary: data.Summary,
			SourceType: data.SourceType, SourceRef: data.SourceRef, EvidenceRefs: data.EvidenceRefs, ArtifactRefs: artifactRefs,
		}).EventPayload(),
	}
}

func artifactRecord(source string, data resultevents.ArtifactCreatedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        resultevents.TopicArtifactCreated,
		SourceModule: source,
		SubjectType:  "artifact",
		SubjectID:    string(data.ArtifactID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: (events.ArtifactPayloadData{
			ResultID: data.ResultID, StepID: data.StepID, ToolCallID: data.ToolCallID, PatchID: data.PatchID,
			ValidationResultID: data.ValidationResultID, ArtifactType: data.ArtifactType, Name: data.Name,
			Summary: data.Summary, URI: data.URI, ContentType: data.ContentType, State: data.State, EvidenceRefs: data.EvidenceRefs,
		}).EventPayload(),
	}
}

func memoryRecord(topic string, source string, data memoryevents.MemoryChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "memory", SubjectID: string(data.MemoryID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		Payload: (events.MemoryPayloadData{
			MemoryType: data.MemoryType, Summary: data.Summary, Content: data.Content, SourceType: data.SourceType,
			SourceRef: data.SourceRef, Tags: data.Tags, Importance: data.Importance, Confidence: data.Confidence, Visibility: data.Visibility,
		}).EventPayload(),
	}
}

func memoryRecallRecord(source string, data memoryevents.MemoryRecallRecordedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: memoryevents.TopicMemoryRecallRecorded, SourceModule: source, SubjectType: "memory_recall", SubjectID: string(data.TraceID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, OccurredAt: data.OccurredAt,
		Payload: (events.MemoryRecallPayloadData{
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
		Payload: (events.LLMUsagePayloadData{
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

func patchRecord(topic string, source string, data orchestratorevents.PatchChangedData) *eventstreammodel.EventRecord {
	subjectID := data.SubjectID
	if subjectID == "" {
		subjectID = string(data.PatchID)
	}
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "patch", SubjectID: subjectID,
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID,
		Payload: (events.PatchPayloadData{
			PatchID: data.PatchID, StepID: data.StepID, ToolCallID: data.ToolCallID, PatchType: data.PatchType,
			TargetRef: data.TargetRef, Summary: data.Summary, State: data.State, AddedLines: data.AddedLines, DeletedLines: data.DeletedLines,
			ContentHash: data.ContentHash, DiffHash: data.DiffHash, ApprovalID: data.ApprovalID, Risk: data.Risk, Error: data.Error,
		}).EventPayload(),
	}
}

func artifactContinuationRecord(topic string, source string, data orchestratorevents.ArtifactContinuationChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "artifact_continuation", SubjectID: string(data.ContinuationID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID,
		Payload: (events.ArtifactContinuationPayloadData{
			ContinuationID: data.ContinuationID, RunID: data.RunID, ArtifactID: data.ArtifactID, TargetRef: data.TargetRef,
			State: data.State, ExpectedSequence: data.ExpectedSequence, FinalReceived: data.FinalReceived, BytesWritten: data.BytesWritten,
			RollingHash: data.RollingHash, LastChunkHash: data.LastChunkHash, ReceivedSequences: data.ReceivedSequences,
			CoverageTargets: data.CoverageTargets, EvidenceRefs: data.EvidenceRefs, FailureReason: data.FailureReason,
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
		Payload: (events.DecisionResolutionPayloadData{
			DecisionTicketPayloadData: events.DecisionTicketPayloadData{Reason: data.Ticket.Reason, State: data.Ticket.State},
			ResolutionID:              data.ResolutionID, Type: data.ResolutionType, Payload: workorcheventpayload.New(data.ResolutionPayload...),
			ContinuationAction: data.ContinuationAction, ResumeRunID: data.ResumeRunID, SuccessorRunID: data.SuccessorRunID,
		}).EventPayload(),
	}
}

func (s *Service) publishRecordAppended(ctx context.Context, record *eventstreammodel.EventRecord) error {
	if s == nil || s.commandHub == nil || record == nil {
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
	event.BindLaneKey(events.EventHubLaneKeyFor(event.ID(), events.EventHubLaneScope{
		SubjectType: record.SubjectType,
		SubjectID:   record.SubjectID,
		WorkspaceID: string(record.WorkspaceID),
		RunID:       string(record.RunID),
		StepID:      string(record.StepID),
	}))
	s.commandHub.Post(event)
	return nil
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	unsubscribers := s.unsubscribers
	s.unsubscribers = nil
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

func (s *Service) ListEvents(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, common.ErrInvalidArgument
	}
	return s.repo.List(ctx, query)
}

func (s *Service) PurgeWorkspaceEvents(ctx context.Context, workspaceID common.WorkspaceID) (int, error) {
	if s == nil || s.repo == nil || workspaceID == "" {
		return 0, common.ErrInvalidArgument
	}
	items, err := s.repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
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
