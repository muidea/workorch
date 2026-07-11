package workspaceviewcore

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Manager struct {
	loader               Loader
	commandHub           magicevent.Hub
	schedule             Scheduler
	store                *Store
	rebuildCoalesceDelay time.Duration

	mu                 sync.Mutex
	unsubscribers      []func()
	started            bool
	closed             bool
	rebuildAllInFlight bool
	rebuildAllPending  bool
	workspaceRebuilds  map[string]*workspaceRebuildState
	metrics            managerMetrics
}

var managerObserverSequence atomic.Uint64

type workspaceRebuildState struct {
	inFlight         bool
	pending          bool
	pendingAfterFull bool
	scheduled        bool
}

type managerMetrics struct {
	fullRebuildTotal               uint64
	fullRebuildFailedTotal         uint64
	fullRebuildDurationMsTotal     int64
	workspaceRebuildDebouncedTotal uint64
	scheduleDroppedTotal           uint64
	workspaceRebuilds              map[string]*workspaceRebuildMetrics
}

type workspaceRebuildMetrics struct {
	rebuildTotal    uint64
	failedTotal     uint64
	durationMsTotal int64
}

func NewManager(options ManagerOptions) *Manager {
	return &Manager{
		loader:               options.Loader,
		commandHub:           options.CommandHub,
		schedule:             options.Schedule,
		store:                NewStore(),
		rebuildCoalesceDelay: options.RebuildCoalesceDelay,
		workspaceRebuilds:    map[string]*workspaceRebuildState{},
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if m == nil || m.loader == nil {
		return common.ErrInvalidArgument
	}
	if m.schedule == nil {
		return fmt.Errorf("%w: workspace projection scheduler is not configured", common.ErrInvalidArgument)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fmt.Errorf("%w: workspace projection manager closed", common.ErrConflict)
	}
	if m.started {
		m.mu.Unlock()
		return nil
	}
	if !m.started {
		m.started = true
		if m.commandHub != nil {
			observerID := fmt.Sprintf("/eventstream/consumers/application/workorchd/workspace-view/%d", managerObserverSequence.Add(1))
			observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, m.commandHub)
			observer.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
				data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
				if ok {
					_ = m.handleEventRecord(event.Context(), data.Record)
				}
				if result != nil {
					result.Set(nil, nil)
				}
			})
			m.unsubscribers = append(m.unsubscribers, func() {
				observer.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended)
			})
		}
	}
	m.mu.Unlock()
	if m.ScheduleRebuildAll() {
		return nil
	}
	err := fmt.Errorf("%w: workspace projection baseline scheduling failed", common.ErrConflict)
	m.mu.Lock()
	unsubscribers := append([]func(){}, m.unsubscribers...)
	m.unsubscribers = nil
	m.started = false
	m.mu.Unlock()
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	m.store.setFailed(err)
	return err
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	unsubscribers := append([]func(){}, m.unsubscribers...)
	m.unsubscribers = nil
	m.closed = true
	m.mu.Unlock()
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	m.store.setStopped()
}

func (m *Manager) Status() Status {
	if m == nil || m.store == nil {
		return Status{State: StateFailed, Error: "workspace projection manager is not configured"}
	}
	return m.store.Status()
}

func (m *Manager) Metrics() Metrics {
	if m == nil {
		return Metrics{}
	}
	var heartbeatMetrics Metrics
	if m.store != nil {
		heartbeatMetrics = m.store.RunHeartbeatMetrics(time.Now().UTC())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := Metrics{
		FullRebuildTotal:                m.metrics.fullRebuildTotal,
		FullRebuildFailedTotal:          m.metrics.fullRebuildFailedTotal,
		FullRebuildDurationMsTotal:      m.metrics.fullRebuildDurationMsTotal,
		PendingWorkspaceRebuilds:        m.pendingWorkspaceRebuildsLocked(),
		WorkspaceRebuildDebouncedTotal:  m.metrics.workspaceRebuildDebouncedTotal,
		ScheduleDroppedTotal:            m.metrics.scheduleDroppedTotal,
		ActiveRunHeartbeatTotal:         heartbeatMetrics.ActiveRunHeartbeatTotal,
		ActiveRunHeartbeatAgeSecondsMax: heartbeatMetrics.ActiveRunHeartbeatAgeSecondsMax,
		StalledRunTotal:                 heartbeatMetrics.StalledRunTotal,
		IneffectiveRetryRunTotal:        heartbeatMetrics.IneffectiveRetryRunTotal,
		ConvergenceActiveSlices:         heartbeatMetrics.ConvergenceActiveSlices,
		ConvergenceOpenFocuses:          heartbeatMetrics.ConvergenceOpenFocuses,
		ConvergenceRepairAttemptsTotal:  heartbeatMetrics.ConvergenceRepairAttemptsTotal,
		ConvergenceNoDeltaRoundsTotal:   heartbeatMetrics.ConvergenceNoDeltaRoundsTotal,
		ConvergenceBudgetEscalations:    heartbeatMetrics.ConvergenceBudgetEscalations,
	}
	if len(m.metrics.workspaceRebuilds) > 0 {
		ids := make([]string, 0, len(m.metrics.workspaceRebuilds))
		for workspaceID := range m.metrics.workspaceRebuilds {
			ids = append(ids, workspaceID)
		}
		sort.Strings(ids)
		ret.WorkspaceRebuilds = make([]WorkspaceRebuildMetrics, 0, len(ids))
		for _, workspaceID := range ids {
			item := m.metrics.workspaceRebuilds[workspaceID]
			if item == nil {
				continue
			}
			ret.WorkspaceRebuilds = append(ret.WorkspaceRebuilds, WorkspaceRebuildMetrics{
				WorkspaceID:     workspaceID,
				RebuildTotal:    item.rebuildTotal,
				FailedTotal:     item.failedTotal,
				DurationMsTotal: item.durationMsTotal,
			})
		}
	}
	return ret
}

func (m *Manager) ListWorkspaces() ([]appmodelcore.Workspace, Status, error) {
	if m == nil || m.store == nil {
		return nil, Status{State: StateFailed}, common.ErrConflict
	}
	return m.store.List()
}

func (m *Manager) ReadWorkspaceSnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshot, Status, error) {
	if m == nil || m.store == nil {
		return readmodelcore.WorkspaceSnapshot{}, Status{State: StateFailed}, common.ErrConflict
	}
	return m.store.GetSnapshot(workspaceID)
}

func (m *Manager) ReadWorkspaceSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshot, Status, error) {
	if m == nil || m.store == nil {
		return readmodelcore.WorkspaceSnapshot{}, Status{State: StateFailed}, common.ErrConflict
	}
	return m.store.GetSummarySnapshot(workspaceID)
}

func (m *Manager) ReadWorkspaceSnapshotWithContext(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, Status, error) {
	snapshot, status, err := m.ReadWorkspaceSnapshot(workspaceID)
	if err == nil || m == nil || m.store == nil || !m.store.NeedsSnapshot(workspaceID) {
		return snapshot, status, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if rebuildErr := m.RebuildWorkspace(ctx, workspaceID); rebuildErr != nil {
		reloaded, reloadedStatus, readErr := m.ReadWorkspaceSnapshot(workspaceID)
		if readErr == nil {
			return reloaded, reloadedStatus, nil
		}
		return reloaded, reloadedStatus, rebuildErr
	}
	return m.ReadWorkspaceSnapshot(workspaceID)
}

func (m *Manager) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if m == nil || m.store == nil {
		return
	}
	m.store.UpsertWorkspace(workspace)
	m.publishProjectionChanged(context.Background(), workspace.ID, "overview")
}

func (m *Manager) UpsertSnapshot(snapshot readmodelcore.WorkspaceSnapshot) {
	if m == nil || m.store == nil {
		return
	}
	m.store.UpsertSnapshot(snapshot)
	m.publishProjectionChanged(context.Background(), snapshot.Workspace.ID, "detail")
}

func (m *Manager) ApplyRunCommandAccepted(update RunCommandAcceptedUpdate) bool {
	if m == nil || m.store == nil {
		return false
	}
	if !m.store.ApplyRunCommandAccepted(update) {
		return false
	}
	m.publishProjectionChanged(context.Background(), update.WorkspaceID, "run_command_accepted")
	return true
}

func (m *Manager) ApplyRunCommandRejected(update RunCommandRejectedUpdate) bool {
	if m == nil || m.store == nil {
		return false
	}
	if !m.store.ApplyRunCommandRejected(update) {
		return false
	}
	m.publishProjectionChanged(context.Background(), update.WorkspaceID, "run_command_rejected")
	return true
}

func (m *Manager) DeleteWorkspace(workspaceID string) {
	if m == nil || m.store == nil {
		return
	}
	m.store.DeleteWorkspace(workspaceID)
	m.publishProjectionChanged(context.Background(), workspaceID, "deleted")
}

func (m *Manager) RebuildAll(ctx context.Context) error {
	if m == nil || m.loader == nil || m.store == nil {
		return common.ErrInvalidArgument
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fmt.Errorf("%w: workspace projection manager closed", common.ErrConflict)
	}
	if m.rebuildAllInFlight {
		m.rebuildAllPending = true
		m.mu.Unlock()
		return nil
	}
	m.rebuildAllInFlight = true
	m.mu.Unlock()
	defer m.finishRebuildAll()

	return m.rebuildAll(ctx)
}

func (m *Manager) rebuildAll(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() {
		m.recordFullRebuild(time.Since(startedAt), err)
	}()
	m.store.setBuilding()
	workspaces, err := m.loader.ListWorkspaces(ctx)
	if err != nil {
		m.store.setFailed(err)
		return err
	}
	if m.isClosed() {
		return fmt.Errorf("%w: workspace projection manager closed", common.ErrConflict)
	}
	m.store.ReplaceWorkspaceList(workspaces, nil)
	m.publishProjectionChanged(ctx, "", "baseline")
	return nil
}

func (m *Manager) RebuildWorkspace(ctx context.Context, workspaceID string) (err error) {
	if m == nil || m.loader == nil || m.store == nil {
		return common.ErrInvalidArgument
	}
	if m.isClosed() {
		return fmt.Errorf("%w: workspace projection manager closed", common.ErrConflict)
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return m.RebuildAll(ctx)
	}
	startedAt := time.Now()
	defer func() {
		m.recordWorkspaceRebuild(workspaceID, time.Since(startedAt), err)
	}()
	snapshot, err := m.loader.ReadWorkspaceSnapshot(ctx, workspaceID)
	if err != nil {
		workspace, readErr := m.loader.ReadWorkspace(ctx, workspaceID)
		if readErr == nil {
			m.store.UpsertWorkspace(workspace)
			m.publishProjectionChanged(ctx, workspaceID, "overview")
		}
		m.store.MarkWorkspaceDegraded(workspaceID, err)
		m.publishProjectionChanged(ctx, workspaceID, "detail_failed")
		return err
	}
	m.store.UpsertSnapshot(snapshot)
	m.publishProjectionChanged(ctx, workspaceID, "detail")
	return nil
}

func (m *Manager) ScheduleRebuildWorkspace(workspaceID string) {
	if m == nil {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		m.ScheduleRebuildAll()
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	rebuildState := m.workspaceRebuildStateLocked(workspaceID)
	if rebuildState.inFlight {
		rebuildState.pending = true
		m.metrics.workspaceRebuildDebouncedTotal++
		m.mu.Unlock()
		return
	}
	delay := m.rebuildCoalesceDelay
	if delay <= 0 {
		rebuildState.inFlight = true
		m.mu.Unlock()
		m.submitWorkspaceRebuild(workspaceID, 0)
		return
	}
	if rebuildState.scheduled {
		m.metrics.workspaceRebuildDebouncedTotal++
		m.mu.Unlock()
		return
	}
	rebuildState.scheduled = true
	m.mu.Unlock()
	m.submitWorkspaceRebuild(workspaceID, delay)
}

func (m *Manager) submitWorkspaceRebuild(workspaceID string, delay time.Duration) {
	if m == nil {
		return
	}
	if !m.submit("workspace_projection_rebuild", func(ctx context.Context) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
		}
		m.mu.Lock()
		rebuildState := m.workspaceRebuilds[workspaceID]
		if rebuildState == nil || m.closed {
			if rebuildState != nil {
				rebuildState.scheduled = false
				m.pruneWorkspaceRebuildStateLocked(workspaceID)
			}
			m.mu.Unlock()
			return
		}
		rebuildState.scheduled = false
		rebuildState.inFlight = true
		m.mu.Unlock()
		if err := m.RebuildWorkspace(ctx, workspaceID); err != nil && !m.isClosed() {
			slog.Warn("workorch workspace projection rebuild failed", "workspaceID", workspaceID, "err", err)
		}
		reschedule := false
		m.mu.Lock()
		rebuildState = m.workspaceRebuilds[workspaceID]
		if rebuildState != nil && rebuildState.pending && !m.closed {
			rebuildState.pending = false
			reschedule = true
		}
		if rebuildState != nil {
			rebuildState.inFlight = false
		}
		m.pruneWorkspaceRebuildStateLocked(workspaceID)
		m.mu.Unlock()
		if reschedule {
			m.ScheduleRebuildWorkspace(workspaceID)
		}
	}) {
		m.mu.Lock()
		m.metrics.scheduleDroppedTotal++
		if rebuildState := m.workspaceRebuilds[workspaceID]; rebuildState != nil {
			rebuildState.inFlight = false
			rebuildState.pending = false
			rebuildState.pendingAfterFull = false
			rebuildState.scheduled = false
		}
		m.pruneWorkspaceRebuildStateLocked(workspaceID)
		m.mu.Unlock()
	}
}

func (m *Manager) ScheduleRebuildAll() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	if m.closed || m.rebuildAllInFlight {
		if !m.closed && m.rebuildAllInFlight {
			m.rebuildAllPending = true
		}
		accepted := !m.closed
		m.mu.Unlock()
		return accepted
	}
	m.rebuildAllInFlight = true
	m.mu.Unlock()
	if !m.submit("workspace_projection_rebuild_all", func(ctx context.Context) {
		defer m.finishRebuildAll()
		if err := m.rebuildAll(ctx); err != nil && !m.isClosed() {
			slog.Warn("workorch workspace projection full rebuild failed", "err", err)
		}
	}) {
		m.mu.Lock()
		m.metrics.scheduleDroppedTotal++
		m.rebuildAllInFlight = false
		m.rebuildAllPending = false
		m.mu.Unlock()
		return false
	}
	return true
}

func (m *Manager) submit(name string, fn func(context.Context)) bool {
	if m == nil || fn == nil {
		return false
	}
	if m.schedule != nil {
		return m.schedule(name, fn)
	}
	slog.Warn("workorch workspace projection task scheduling skipped because scheduler is not configured", "task", name)
	return false
}

func (m *Manager) finishRebuildAll() {
	if m == nil {
		return
	}
	rescheduleFull := false
	var pendingWorkspaces []string
	m.mu.Lock()
	if m.rebuildAllPending && !m.closed {
		m.rebuildAllPending = false
		rescheduleFull = true
	}
	if !rescheduleFull && len(m.workspaceRebuilds) > 0 && !m.closed {
		pendingWorkspaces = make([]string, 0, len(m.workspaceRebuilds))
		for workspaceID, rebuildState := range m.workspaceRebuilds {
			if rebuildState == nil || !rebuildState.pendingAfterFull {
				continue
			}
			rebuildState.pendingAfterFull = false
			pendingWorkspaces = append(pendingWorkspaces, workspaceID)
			m.pruneWorkspaceRebuildStateLocked(workspaceID)
		}
	}
	m.rebuildAllInFlight = false
	m.mu.Unlock()
	if rescheduleFull {
		m.ScheduleRebuildAll()
		return
	}
	for _, workspaceID := range pendingWorkspaces {
		m.ScheduleRebuildWorkspace(workspaceID)
	}
}

func (m *Manager) isClosed() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Manager) handleEventRecord(_ context.Context, record eventstreamevents.EventRecordData) error {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(record.Topic) == "" || record.Topic == workorchdevents.TopicWorkspaceProjectionChanged {
		return nil
	}
	if projectionEventIsRuntimeProgressOnly(record.Topic) {
		return m.handleRuntimeProgressEvent(record)
	}
	workspaceID := strings.TrimSpace(string(record.WorkspaceID))
	if workspaceID != "" {
		m.store.MarkStale(workspaceID, record.Revision)
		if m.isRebuildAllInFlight() {
			m.rememberPendingWorkspaceRebuild(workspaceID)
			return nil
		}
		m.ScheduleRebuildWorkspace(workspaceID)
		return nil
	}
	switch strings.TrimSpace(record.SubjectType) {
	case "workspace":
		m.store.MarkStale("", record.Revision)
		m.ScheduleRebuildAll()
	}
	return nil
}

func projectionEventIsRuntimeProgressOnly(topic string) bool {
	switch strings.TrimSpace(topic) {
	case runevents.TopicRunLLMStreamProgress,
		runevents.TopicRunLLMUsageRecorded,
		runevents.TopicStepDispatched,
		runevents.TopicStepStateChanged:
		return true
	default:
		return false
	}
}

func (m *Manager) handleRuntimeProgressEvent(record eventstreamevents.EventRecordData) error {
	if m == nil || m.store == nil {
		return nil
	}
	payload := record.Payload.Map()
	progress, _ := events.DecodeLLMStreamProgressPayload(payload)
	workspaceID := strings.TrimSpace(firstNonEmptyString(string(record.WorkspaceID), string(progress.WorkspaceID), stringFromPayload(payload, events.PayloadKeyWorkspaceID)))
	if workspaceID == "" {
		return nil
	}
	update := RuntimeProgressUpdate{
		WorkspaceID:   workspaceID,
		RunID:         strings.TrimSpace(firstNonEmptyString(string(record.RunID), string(progress.RunID), stringFromPayload(payload, events.PayloadKeyRunID))),
		StepID:        strings.TrimSpace(firstNonEmptyString(string(record.StepID), string(progress.StepID), stringFromPayload(payload, events.PayloadKeyStepID))),
		StepKind:      strings.TrimSpace(stringFromPayload(payload, events.PayloadKeyKind)),
		StepState:     strings.TrimSpace(firstNonEmptyString(progress.Status, stringFromPayload(payload, events.PayloadKeyState), stringFromPayload(payload, events.PayloadKeyStatus))),
		Topic:         record.Topic,
		Revision:      record.Revision,
		OccurredAt:    strings.TrimSpace(firstNonEmptyString(record.OccurredAt, stringFromPayload(payload, "occurredAt"))),
		Event:         runtimeProgressEventName(record.Topic),
		Stage:         strings.TrimSpace(firstNonEmptyString(stringFromPayload(payload, events.PayloadKeyStage), runtimeProgressStageName(record.Topic))),
		Summary:       strings.TrimSpace(firstNonEmptyString(progress.Summary, stringFromPayload(payload, events.PayloadKeySummary), runtimeProgressSummary(record.Topic))),
		RequestID:     strings.TrimSpace(stringFromPayload(payload, "requestId")),
		QueueKey:      strings.TrimSpace(stringFromPayload(payload, "queueKey")),
		WorkerID:      strings.TrimSpace(stringFromPayload(payload, "workerId")),
		WorkerEventAt: strings.TrimSpace(firstNonEmptyString(stringFromPayload(payload, "heartbeatAt"), stringFromPayload(payload, "workerHeartbeatAt"))),
	}
	if m.store.ApplyRuntimeProgress(update) {
		return nil
	}
	m.store.MarkStale(workspaceID, record.Revision)
	if m.isRebuildAllInFlight() {
		m.rememberPendingWorkspaceRebuild(workspaceID)
		return nil
	}
	m.ScheduleRebuildWorkspace(workspaceID)
	return nil
}

func runtimeProgressEventName(topic string) string {
	switch strings.TrimSpace(topic) {
	case runevents.TopicRunLLMStreamProgress:
		return "runtime_step_progress"
	case runevents.TopicRunLLMUsageRecorded:
		return "runtime_usage_recorded"
	case runevents.TopicStepDispatched:
		return "runtime_step_dispatched"
	case runevents.TopicStepStateChanged:
		return "runtime_step_state_changed"
	default:
		return "runtime_progress"
	}
}

func runtimeProgressStageName(topic string) string {
	switch strings.TrimSpace(topic) {
	case runevents.TopicRunLLMStreamProgress:
		return "llm_stream"
	case runevents.TopicRunLLMUsageRecorded:
		return "llm_usage"
	case runevents.TopicStepDispatched:
		return "step_dispatched"
	case runevents.TopicStepStateChanged:
		return "step_state"
	default:
		return "runtime_progress"
	}
}

func runtimeProgressSummary(topic string) string {
	switch strings.TrimSpace(topic) {
	case runevents.TopicRunLLMStreamProgress:
		return "provider stream receiving"
	case runevents.TopicRunLLMUsageRecorded:
		return "llm usage recorded"
	case runevents.TopicStepDispatched:
		return "step dispatched"
	case runevents.TopicStepStateChanged:
		return "step state changed"
	default:
		return "runtime progress"
	}
}

func (m *Manager) rememberPendingWorkspaceRebuild(workspaceID string) {
	if m == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.workspaceRebuildStateLocked(workspaceID).pendingAfterFull = true
	}
}

func (m *Manager) workspaceRebuildStateLocked(workspaceID string) *workspaceRebuildState {
	if m.workspaceRebuilds == nil {
		m.workspaceRebuilds = map[string]*workspaceRebuildState{}
	}
	rebuildState := m.workspaceRebuilds[workspaceID]
	if rebuildState == nil {
		rebuildState = &workspaceRebuildState{}
		m.workspaceRebuilds[workspaceID] = rebuildState
	}
	return rebuildState
}

func (m *Manager) pendingWorkspaceRebuildsLocked() int {
	if len(m.workspaceRebuilds) == 0 {
		return 0
	}
	ret := 0
	for _, item := range m.workspaceRebuilds {
		if item != nil && (item.pending || item.pendingAfterFull || item.scheduled) {
			ret++
		}
	}
	return ret
}

func (m *Manager) recordFullRebuild(duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.fullRebuildTotal++
	m.metrics.fullRebuildDurationMsTotal += duration.Milliseconds()
	if err != nil {
		m.metrics.fullRebuildFailedTotal++
	}
}

func (m *Manager) recordWorkspaceRebuild(workspaceID string, duration time.Duration, err error) {
	if m == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.metrics.workspaceRebuilds == nil {
		m.metrics.workspaceRebuilds = map[string]*workspaceRebuildMetrics{}
	}
	item := m.metrics.workspaceRebuilds[workspaceID]
	if item == nil {
		item = &workspaceRebuildMetrics{}
		m.metrics.workspaceRebuilds[workspaceID] = item
	}
	item.rebuildTotal++
	item.durationMsTotal += duration.Milliseconds()
	if err != nil {
		item.failedTotal++
	}
}

func (m *Manager) pruneWorkspaceRebuildStateLocked(workspaceID string) {
	rebuildState := m.workspaceRebuilds[workspaceID]
	if rebuildState == nil {
		return
	}
	if !rebuildState.inFlight && !rebuildState.pending && !rebuildState.pendingAfterFull && !rebuildState.scheduled {
		delete(m.workspaceRebuilds, workspaceID)
	}
}

func (m *Manager) isRebuildAllInFlight() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuildAllInFlight
}

func (m *Manager) publishProjectionChanged(ctx context.Context, workspaceID string, changeKind string) {
	if m == nil || m.commandHub == nil || m.store == nil {
		return
	}
	status := m.store.StatusForWorkspace(workspaceID)
	data := workorchdevents.WorkspaceProjectionChangedData{
		WorkspaceID:             common.WorkspaceID(workspaceID),
		GlobalRevision:          status.GlobalRevision,
		WorkspaceRevision:       status.WorkspaceRevision,
		SourceEventRevision:     status.SourceEventRevision,
		ProjectedSourceRevision: status.ProjectedSourceRevision,
		WorkspaceTotal:          status.WorkspaceTotal,
		ChangeKind:              changeKind,
		ProjectionState:         status.State,
		UpdatedAt:               status.UpdatedAt,
		RebuiltAt:               status.RebuiltAt,
		StaleReason:             status.StaleReason,
		Error:                   status.Error,
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkspaceProjectionChanged, "workorchd.workspace_projection", workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(event.ID(), events.EventHubLaneScope{SubjectType: "workspace_projection", SubjectID: workspaceID, WorkspaceID: workspaceID}))
	m.commandHub.Post(event)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringFromPayload(payload map[string]any, key string) string {
	if len(payload) == 0 || key == "" {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}
