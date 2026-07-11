package eventstream

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalcommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
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
	s.subscribeRunStepNotifications()
	s.subscribeRunLLMNotifications()
	s.subscribeWorkspaceNotifications()
	s.subscribeResultNotifications()
	s.subscribeMemoryNotifications()
	s.subscribeOrchestratorDetailNotifications()
	s.subscribeContractViolationNotifications()
	return nil
}

func (s *Service) subscribeContractViolationNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/orchestrator/consumers/kernel/eventstream/contract-violation"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	topics := []string{
		orchestratorevents.TopicOrchestratorContractViolation,
		orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated,
	}
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(orchestratorevents.ContractViolationData); ok {
				_ = s.appendRecord(event.Context(), contractViolationRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
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
			if data, ok := event.Data().(orchestratorevents.PatchChangedData); ok {
				_ = s.appendRecord(event.Context(), patchRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
			if data, ok := event.Data().(orchestratorevents.ArtifactContinuationChangedData); ok {
				_ = s.appendRecord(event.Context(), artifactContinuationRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
			if data, ok := event.Data().(memoryevents.MemoryChangedData); ok {
				_ = s.appendRecord(event.Context(), memoryRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	observer.Subscribe(memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(memoryevents.MemoryRecallRecordedData); ok {
			_ = s.appendRecord(event.Context(), memoryRecallRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range memoryTopics {
			observer.Unsubscribe(topic)
		}
		observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded)
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
			if data, ok := event.Data().(resultevents.ResultChangedData); ok {
				_ = s.appendRecord(event.Context(), resultRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	observer.Subscribe(resultevents.TopicArtifactCreated, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(resultevents.ArtifactCreatedData); ok {
			_ = s.appendRecord(event.Context(), artifactRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
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
			if data, ok := event.Data().(workspaceevents.WorkspaceChangedData); ok {
				_ = s.appendRecord(event.Context(), workspaceRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	observer.Subscribe(workspaceevents.TopicGoalDefined, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.GoalDefinedData); ok {
			_ = s.appendRecord(event.Context(), goalRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(workspaceevents.TopicDoDDefined, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.DoDDefinedData); ok {
			_ = s.appendRecord(event.Context(), dodRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range workspaceTopics {
			observer.Unsubscribe(topic)
		}
		observer.Unsubscribe(workspaceevents.TopicGoalDefined)
		observer.Unsubscribe(workspaceevents.TopicDoDDefined)
	})
}

func (s *Service) subscribeRunStepNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/run/consumers/kernel/eventstream/step"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	topics := []string{
		runevents.TopicStepCreated,
		runevents.TopicStepDispatched,
		runevents.TopicStepSucceeded,
		runevents.TopicStepFailed,
		runevents.TopicStepSkipped,
		runevents.TopicStepStateChanged,
	}
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(runevents.StepChangedData); ok {
				_ = s.appendRecord(event.Context(), runStepRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	s.unsubscribers = append(s.unsubscribers, func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
}

func (s *Service) subscribeRunLLMNotifications() {
	if s == nil || s.commandHub == nil {
		return
	}
	const observerID = "/run/consumers/kernel/eventstream/llm"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, s.commandHub)
	observer.Subscribe(runevents.TopicRunLLMStreamProgress, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.LLMStreamProgressData); ok {
			_ = s.appendRecord(event.Context(), runLLMStreamProgressRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunLLMUsageRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.LLMUsageRecordedData); ok {
			_ = s.appendRecord(event.Context(), runLLMUsageRecord(event.Source(), data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	s.unsubscribers = append(s.unsubscribers, func() {
		observer.Unsubscribe(runevents.TopicRunLLMStreamProgress)
		observer.Unsubscribe(runevents.TopicRunLLMUsageRecorded)
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
			if data, ok := event.Data().(runevents.RunStateChangedData); ok {
				_ = s.appendRecord(event.Context(), runStateRecord(topic, event.Source(), data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
			if data, ok := event.Data().(governanceevents.GovernanceData); ok {
				_ = s.appendRecord(event.Context(), governanceRecord(topic, data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
			if data, ok := event.Data().(approvalevents.ApprovalChangedData); ok {
				_ = s.appendRecord(event.Context(), approvalRecord(topic, data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
			if data, ok := event.Data().(decisionevents.TicketChangedData); ok {
				_ = s.appendRecord(event.Context(), decisionTicketRecord(topic, data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	resolvedTopics := []string{decisionevents.TopicTicketResolved, decisionevents.TopicDecisionTicketResolved}
	for _, topic := range resolvedTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(decisionevents.TicketResolvedData); ok {
				_ = s.appendRecord(event.Context(), decisionResolvedRecord(topic, data))
			}
			if result != nil {
				result.Set(nil, nil)
			}
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
		revision, err := s.nextWorkspaceRevision(ctx, record.WorkspaceID)
		if err != nil {
			return err
		}
		record.Revision = revision
	}
	if err := s.repo.Append(ctx, record); err != nil && err != common.ErrConflict {
		return err
	}
	return s.publishRecordAppended(ctx, record)
}

func (s *Service) nextWorkspaceRevision(ctx context.Context, workspaceID common.WorkspaceID) (int64, error) {
	s.revisionMu.Lock()
	defer s.revisionMu.Unlock()
	if s.revisions == nil {
		s.revisions = map[common.WorkspaceID]int64{}
	}
	current, ok := s.revisions[workspaceID]
	if !ok {
		items, err := s.repo.List(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), repository.EventRecordQuery{WorkspaceID: workspaceID})
		if err != nil {
			return 0, err
		}
		for _, item := range items {
			if item != nil && item.Revision > current {
				current = item.Revision
			}
		}
	}
	current++
	s.revisions[workspaceID] = current
	return current, nil
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
		Payload:      events.DecisionTicketPayload(data.Reason, data.State),
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
		Payload:      events.ApprovalPayload(data.Reason, data.Action, approvalEvidenceMap(data.Context), data.State),
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
		Payload: events.AutoGovernancePayload(
			trace.ID,
			trace.ItemType,
			trace.ItemID,
			trace.Action,
			trace.RiskLevel,
			trace.Confidence,
			trace.PolicyMatched,
			trace.PolicyBlocked,
			trace.BudgetBefore,
			trace.BudgetAfter,
			trace.LLMProviderID,
			trace.LLMModel,
			trace.LLMDecisionPreview,
			trace.PromptFingerprint,
			trace.Status,
			trace.Reason,
			data.EscalationReason,
		),
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
		Payload: events.RunPayload(
			data.GoalID,
			data.DoDID,
			data.State,
			data.BlockingTicketID,
			data.BlockingApprovalID,
			data.FailureReason,
		),
	}
}

func runStepRecord(topic string, source string, data runevents.StepChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        topic,
		SourceModule: source,
		SubjectType:  "step",
		SubjectID:    string(data.StepID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: events.StepPayload(
			data.Kind,
			data.State,
			runStepSnapshotMap(data.Input, "inputRef"),
			runStepSnapshotMap(data.Output, "outputRef"),
		),
	}
}

func runStepSnapshotMap(snapshot runevents.StepSnapshot, refKey string) map[string]any {
	ret := snapshot.Summary.Map()
	if len(ret) > 0 {
		ret = map[string]any{"summary": ret}
	}
	if snapshot.Ref != "" {
		if ret == nil {
			ret = map[string]any{}
		}
		ret[refKey] = snapshot.Ref
	}
	return ret
}

func runLLMStreamProgressRecord(source string, data runevents.LLMStreamProgressData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        runevents.TopicRunLLMStreamProgress,
		SourceModule: source,
		SubjectType:  "llm_stream_progress",
		SubjectID:    string(data.StepID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: events.LLMStreamProgressPayload(
			data.WorkspaceID,
			data.RunID,
			data.StepID,
			data.ProviderID,
			data.Model,
			data.ChunkCount,
			data.ReceivedBytes,
			data.ElapsedMs,
			data.Status,
			data.Summary,
		),
	}
}

func runLLMUsageRecord(source string, data runevents.LLMUsageRecordedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        runevents.TopicRunLLMUsageRecorded,
		SourceModule: source,
		SubjectType:  "llm_usage",
		SubjectID:    string(data.StepID),
		WorkspaceID:  data.WorkspaceID,
		RunID:        data.RunID,
		StepID:       data.StepID,
		Payload: events.LLMUsagePayload(events.LLMUsagePayloadData{
			WorkspaceID:   data.WorkspaceID,
			RunID:         data.RunID,
			StepID:        data.StepID,
			ProviderID:    data.ProviderID,
			Provider:      data.Provider,
			APISurface:    data.APISurface,
			Model:         data.Model,
			Operation:     data.Operation,
			InputTokens:   data.InputTokens,
			OutputTokens:  data.OutputTokens,
			TotalTokens:   data.TotalTokens,
			Estimated:     data.Estimated,
			UsageSource:   data.UsageSource,
			RequestBytes:  data.RequestBytes,
			ResponseBytes: data.ResponseBytes,
			Streaming:     data.Streaming,
			ContextBudget: data.ContextBudget.Map(),
			CacheUsage:    data.CacheUsage.Map(),
			CacheMarkers:  data.CacheMarkers.Map(),
			StartedAt:     data.StartedAt,
			CompletedAt:   data.CompletedAt,
			DurationMs:    data.DurationMs,
			Status:        data.Status,
			FailureClass:  data.FailureClass,
		}),
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
		Payload: events.WorkspacePayload(
			data.Title,
			data.Description,
			data.DeliverableDirectory,
			data.InternalDirectory,
			data.State,
			data.ActiveRunID,
		),
	}
}

func goalRecord(source string, data workspaceevents.GoalDefinedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        workspaceevents.TopicGoalDefined,
		SourceModule: source,
		SubjectType:  "goal",
		SubjectID:    string(data.GoalID),
		WorkspaceID:  data.WorkspaceID,
		Payload: events.GoalPayload(
			data.Text,
			data.DefinitionRevision,
			data.PreviousDefinitionHash,
			data.PreviousGoalID,
			data.LineageState,
		),
	}
}

func dodRecord(source string, data workspaceevents.DoDDefinedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic:        workspaceevents.TopicDoDDefined,
		SourceModule: source,
		SubjectType:  "dod",
		SubjectID:    string(data.DoDID),
		WorkspaceID:  data.WorkspaceID,
		Payload: events.DoDPayload(
			data.Items,
			data.State,
			data.DefinitionRevision,
			data.PreviousDefinitionHash,
			data.PreviousDoDID,
			data.LineageState,
		),
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
		Payload: events.ResultPayload(
			data.GoalID,
			data.DoDID,
			data.State,
			data.Title,
			data.Summary,
			data.SourceType,
			data.SourceRef,
			data.EvidenceRefs,
			artifactRefs,
		),
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
		Payload: events.ArtifactPayload(
			data.ResultID,
			data.StepID,
			data.ToolCallID,
			data.PatchID,
			data.ValidationResultID,
			data.ArtifactType,
			data.Name,
			data.Summary,
			data.URI,
			data.ContentType,
			data.State,
			data.EvidenceRefs,
		),
	}
}

func memoryRecord(topic string, source string, data memoryevents.MemoryChangedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "memory", SubjectID: string(data.MemoryID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		Payload: events.MemoryPayload(data.MemoryType, data.Summary, data.Content, data.SourceType, data.SourceRef, data.Tags, data.Importance, data.Confidence, data.Visibility),
	}
}

func memoryRecallRecord(source string, data memoryevents.MemoryRecallRecordedData) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		Topic: memoryevents.TopicMemoryRecallRecorded, SourceModule: source, SubjectType: "memory_recall", SubjectID: string(data.TraceID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, OccurredAt: data.OccurredAt,
		Payload: events.MemoryRecallPayload(data.TraceID, data.Stage, data.Source, data.Policy, data.Status, data.ProviderID, data.Operation, data.RetrievedMemoryIDs, data.Reason),
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
		Payload: events.PatchPayload(data.PatchID, data.StepID, data.ToolCallID, data.PatchType, data.TargetRef, data.Summary, data.State, data.AddedLines, data.DeletedLines, data.ContentHash, data.DiffHash, data.ApprovalID, data.Risk, data.Error),
	}
}

func artifactContinuationRecord(topic string, source string, data orchestratorevents.ArtifactContinuationChangedData) *eventstreammodel.EventRecord {
	payload := map[string]any{
		"id": string(data.ContinuationID), "runId": string(data.RunID), "artifactId": data.ArtifactID,
		"targetRef": data.TargetRef, "state": string(data.State), "expectedSequence": data.ExpectedSequence,
		"finalReceived": data.FinalReceived, "bytesWritten": data.BytesWritten,
	}
	if data.RollingHash != "" {
		payload["rollingHash"] = data.RollingHash
	}
	if data.LastChunkHash != "" {
		payload["lastChunkHash"] = data.LastChunkHash
	}
	if len(data.ReceivedSequences) > 0 {
		payload["receivedSequences"] = append([]int(nil), data.ReceivedSequences...)
	}
	if len(data.CoverageTargets) > 0 {
		payload["coverageTargets"] = append([]string(nil), data.CoverageTargets...)
	}
	if len(data.EvidenceRefs) > 0 {
		payload["evidenceRefs"] = append([]string(nil), data.EvidenceRefs...)
	}
	if data.FailureReason != "" {
		payload["failureReason"] = data.FailureReason
	}
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "artifact_continuation", SubjectID: string(data.ContinuationID),
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, Payload: payload,
	}
}

func contractViolationRecord(topic string, source string, data orchestratorevents.ContractViolationData) *eventstreammodel.EventRecord {
	violation := data.Violation.Map()
	payload := make(map[string]any, len(violation)+8)
	for key, value := range violation {
		payload[key] = value
	}
	payload["contractViolation"] = violation
	payload["workspaceId"] = string(data.WorkspaceID)
	payload["runId"] = string(data.RunID)
	payload["sourceStepId"] = string(data.SourceStepID)
	payload["stepId"] = string(data.SourceStepID)
	payload["topic"] = data.Topic
	payload["summary"] = data.Summary
	if data.RetryStepID != "" {
		payload["retryStepId"] = string(data.RetryStepID)
	}
	if data.TicketID != "" {
		payload["ticketId"] = string(data.TicketID)
	}
	if contextPayload := data.Context.Map(); len(contextPayload) > 0 {
		payload["context"] = contextPayload
	}
	return &eventstreammodel.EventRecord{
		Topic: topic, SourceModule: source, SubjectType: "contract_violation", SubjectID: data.SubjectID,
		WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.SourceStepID, Payload: payload,
	}
}

func approvalEvidenceMap(fields []approvalevents.ApprovalEvidenceField) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(fields))
	for _, field := range fields {
		ret[field.Name] = approvalEvidenceValue(field.Value)
	}
	return ret
}

func approvalEvidenceValue(value approvalevents.ApprovalEvidenceValue) any {
	switch value.Kind {
	case approvalevents.ApprovalEvidenceValueNull:
		return nil
	case approvalevents.ApprovalEvidenceValueString:
		return value.String
	case approvalevents.ApprovalEvidenceValueBool:
		return value.Bool
	case approvalevents.ApprovalEvidenceValueInt:
		return value.Int
	case approvalevents.ApprovalEvidenceValueUint:
		return value.Uint
	case approvalevents.ApprovalEvidenceValueFloat:
		return value.Float
	case approvalevents.ApprovalEvidenceValueObject:
		return approvalEvidenceMap(value.Object)
	case approvalevents.ApprovalEvidenceValueArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, approvalEvidenceValue(item))
		}
		return ret
	default:
		return nil
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
		Payload: events.DecisionResolutionPayload(
			data.Ticket.Reason,
			data.Ticket.State,
			data.ResolutionID,
			data.ResolutionType,
			decisionEvidenceMap(data.ResolutionPayload),
			events.DecisionResolutionPayloadData{
				ContinuationAction: data.ContinuationAction,
				ResumeRunID:        data.ResumeRunID,
				SuccessorRunID:     data.SuccessorRunID,
			},
		),
	}
}

func decisionEvidenceMap(fields []decisionevents.DecisionEvidenceField) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(fields))
	for _, field := range fields {
		ret[field.Name] = decisionEvidenceValue(field.Value)
	}
	return ret
}

func decisionEvidenceValue(value decisionevents.DecisionEvidenceValue) any {
	switch value.Kind {
	case decisionevents.DecisionEvidenceValueNull:
		return nil
	case decisionevents.DecisionEvidenceValueString:
		return value.String
	case decisionevents.DecisionEvidenceValueBool:
		return value.Bool
	case decisionevents.DecisionEvidenceValueInt:
		return value.Int
	case decisionevents.DecisionEvidenceValueUint:
		return value.Uint
	case decisionevents.DecisionEvidenceValueFloat:
		return value.Float
	case decisionevents.DecisionEvidenceValueObject:
		return decisionEvidenceMap(value.Object)
	case decisionevents.DecisionEvidenceValueArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, decisionEvidenceValue(item))
		}
		return ret
	default:
		return nil
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
