package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

type runtimeNotificationRecord struct {
	Topic          string
	WorkspaceID    string
	RunID          string
	StepID         string
	State          string
	ActiveRunID    string
	ProviderID     string
	Model          string
	ChunkCount     int
	ReceivedBytes  int
	TotalTokens    int
	Streaming      bool
	SubjectID      string
	TargetRef      string
	ResultID       string
	ArtifactID     string
	ContinuationID string
}

type runtimeNotificationRecorder struct {
	rawHub     magicevent.Hub
	observers  []magicevent.SimpleObserver
	topics     [][]string
	mu         sync.Mutex
	records    []runtimeNotificationRecord
	notifiedCh chan struct{}
}

func newRuntimeNotificationRecorder(t *testing.T) *runtimeNotificationRecorder {
	t.Helper()
	rawHub := magicevent.NewHub(64)
	recorder := &runtimeNotificationRecorder{
		rawHub:     rawHub,
		notifiedCh: make(chan struct{}, 64),
	}
	runTopics := []string{
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
		runevents.TopicStepDispatched,
		runevents.TopicStepStateChanged,
		runevents.TopicRunLLMStreamProgress,
		runevents.TopicRunLLMUsageRecorded,
	}
	recorder.subscribe(
		"/run/consumers/kernel/orchestrator/runtime-notification-recorder",
		runTopics,
		func(event magicevent.Event) (runtimeNotificationRecord, bool) {
			switch data := event.Data().(type) {
			case runevents.RunStateChangedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), State: string(data.State)}, true
			case runevents.StepChangedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: string(data.State)}, true
			case runevents.LLMStreamProgressData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: data.Status, ProviderID: data.ProviderID, Model: data.Model, ChunkCount: data.ChunkCount, ReceivedBytes: data.ReceivedBytes}, true
			case runevents.LLMUsageRecordedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: data.Status, ProviderID: data.ProviderID, Model: data.Model, TotalTokens: data.TotalTokens, Streaming: data.Streaming}, true
			default:
				return runtimeNotificationRecord{}, false
			}
		},
	)
	orchestratorTopics := []string{
		orchestratorevents.TopicPatchPreviewed,
		orchestratorevents.TopicPatchApplied,
		orchestratorevents.TopicPatchFailed,
		orchestratorevents.TopicPatchReverted,
		orchestratorevents.TopicArtifactContinuationCreated,
		orchestratorevents.TopicArtifactContinuationChunkRecorded,
		orchestratorevents.TopicArtifactContinuationChunkRejected,
		orchestratorevents.TopicArtifactContinuationAssembled,
		orchestratorevents.TopicArtifactContinuationApplied,
		orchestratorevents.TopicArtifactContinuationFailed,
	}
	recorder.subscribe(
		"/orchestrator/consumers/kernel/orchestrator/runtime-notification-recorder",
		orchestratorTopics,
		func(event magicevent.Event) (runtimeNotificationRecord, bool) {
			switch data := event.Data().(type) {
			case orchestratorevents.PatchChangedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: string(data.State), SubjectID: data.SubjectID, TargetRef: data.TargetRef}, true
			case orchestratorevents.ArtifactContinuationChangedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: string(data.State), ContinuationID: string(data.ContinuationID), ArtifactID: data.ArtifactID, TargetRef: data.TargetRef}, true
			default:
				return runtimeNotificationRecord{}, false
			}
		},
	)
	resultTopics := []string{
		resultevents.TopicResultCreated,
		resultevents.TopicResultAccepted,
		resultevents.TopicResultRejected,
		resultevents.TopicResultSuperseded,
		resultevents.TopicArtifactCreated,
	}
	recorder.subscribe(
		"/result/consumers/kernel/orchestrator/runtime-notification-recorder",
		resultTopics,
		func(event magicevent.Event) (runtimeNotificationRecord, bool) {
			switch data := event.Data().(type) {
			case resultevents.ResultChangedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), State: string(data.State), ResultID: string(data.ResultID), SubjectID: string(data.ResultID)}, true
			case resultevents.ArtifactCreatedData:
				return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID), State: string(data.State), ResultID: string(data.ResultID), ArtifactID: string(data.ArtifactID), SubjectID: string(data.ArtifactID)}, true
			default:
				return runtimeNotificationRecord{}, false
			}
		},
	)
	workspaceTopics := []string{workspaceevents.TopicWorkspaceStateChanged}
	recorder.subscribe(
		"/workspace/consumers/kernel/orchestrator/runtime-notification-recorder",
		workspaceTopics,
		func(event magicevent.Event) (runtimeNotificationRecord, bool) {
			data, ok := event.Data().(workspaceevents.WorkspaceChangedData)
			if !ok {
				return runtimeNotificationRecord{}, false
			}
			return runtimeNotificationRecord{Topic: event.ID(), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), State: string(data.State), ActiveRunID: string(data.ActiveRunID)}, true
		},
	)
	t.Cleanup(func() { recorder.stop() })
	return recorder
}

func (r *runtimeNotificationRecorder) subscribe(observerID string, topics []string, project func(magicevent.Event) (runtimeNotificationRecord, bool)) {
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, r.rawHub)
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if record, ok := project(event); ok {
				r.mu.Lock()
				r.records = append(r.records, record)
				r.mu.Unlock()
				select {
				case r.notifiedCh <- struct{}{}:
				default:
				}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	r.observers = append(r.observers, observer)
	r.topics = append(r.topics, append([]string(nil), topics...))
}

func (r *runtimeNotificationRecorder) attach(orchestrator *RuntimeOrchestrator) {
	if r == nil || orchestrator == nil {
		return
	}
	orchestrator.eventPublisher().notificationHub = r.rawHub
}

func (r *runtimeNotificationRecorder) snapshot() []runtimeNotificationRecord {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ret := make([]runtimeNotificationRecord, len(r.records))
	copy(ret, r.records)
	return ret
}

func (r *runtimeNotificationRecorder) waitAtLeast(count int, timeout time.Duration) []runtimeNotificationRecord {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if records := r.snapshot(); len(records) >= count {
			return records
		}
		select {
		case <-r.notifiedCh:
		case <-deadline.C:
			return r.snapshot()
		}
	}
}

func (r *runtimeNotificationRecorder) waitFor(timeout time.Duration, match func(runtimeNotificationRecord) bool) (runtimeNotificationRecord, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		for _, record := range r.snapshot() {
			if match(record) {
				return record, true
			}
		}
		select {
		case <-r.notifiedCh:
		case <-deadline.C:
			return runtimeNotificationRecord{}, false
		}
	}
}

func (r *runtimeNotificationRecorder) stop() {
	if r == nil {
		return
	}
	for idx, observer := range r.observers {
		for _, topic := range r.topics[idx] {
			observer.Unsubscribe(topic)
		}
	}
	if r.rawHub != nil {
		r.rawHub.Terminate(context.Background())
	}
}
