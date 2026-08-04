package workspaceviewcore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type Manager struct {
	loader               Loader
	schedule             Scheduler
	eventPorts           EventPorts
	store                *Store
	rebuildCoalesceDelay time.Duration

	mu                 sync.Mutex
	started            bool
	closed             bool
	rebuildAllInFlight bool
	rebuildAllPending  bool
	workspaceRebuilds  map[string]*workspaceRebuildState
	metrics            managerMetrics
}

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
		schedule:             options.Schedule,
		eventPorts:           options.EventPorts,
		store:                NewStore(),
		rebuildCoalesceDelay: options.RebuildCoalesceDelay,
		workspaceRebuilds:    map[string]*workspaceRebuildState{},
	}
}

// SetEventPorts binds the application Biz-owned typed EventHub operations.
// It is intentionally a narrow pair of projection-specific callbacks rather
// than a Hub or a generic dispatcher.
func (m *Manager) SetEventPorts(ports EventPorts) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.eventPorts = ports
	m.mu.Unlock()
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
	m.started = true
	m.mu.Unlock()
	if m.ScheduleRebuildAll() {
		return nil
	}
	err := fmt.Errorf("%w: workspace projection baseline scheduling failed", common.ErrConflict)
	m.mu.Lock()
	m.started = false
	m.mu.Unlock()
	m.store.setFailed(err)
	return err
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.store.setStopped()
}

func (m *Manager) Status() Status {
	if m == nil || m.store == nil {
		return Status{State: StateFailed, Error: "workspace projection manager is not configured"}
	}
	return m.store.Status()
}

// ReconcileSourceHighWatermarks closes the readiness blind spot where a
// projection subscriber misses EventRecordAppended and otherwise compares its
// own stale counters. Both owner workspace identity and EventStream revisions
// are read through typed EventHub/owner ports.
func (m *Manager) ReconcileSourceHighWatermarks(ctx context.Context) error {
	if m == nil || m.loader == nil || m.store == nil || m.eventPorts.ReadHighWatermarks == nil {
		return common.ErrInvalidArgument
	}
	workspaces, err := m.loader.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	workspaceIDs := make([]common.WorkspaceID, 0, len(workspaces))
	ownerIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceID := strings.TrimSpace(workspace.ID)
		if workspaceID == "" {
			continue
		}
		workspaceIDs = append(workspaceIDs, common.WorkspaceID(workspaceID))
		ownerIDs = append(ownerIDs, workspaceID)
	}
	sort.Strings(ownerIDs)
	if !equalProjectionWorkspaceIDs(ownerIDs, m.store.WorkspaceIDs()) {
		m.store.MarkStale("", 0)
		m.ScheduleRebuildAll()
	}
	if len(workspaceIDs) == 0 {
		return nil
	}
	highWatermarks, err := m.eventPorts.ReadHighWatermarks(ctx, workspaceIDs)
	if err != nil {
		return err
	}
	observed := map[common.WorkspaceID]struct{}{}
	for _, item := range highWatermarks {
		observed[item.WorkspaceID] = struct{}{}
		if m.store.ObserveSourceHighWatermark(string(item.WorkspaceID), item.Revision) {
			m.ScheduleRebuildWorkspace(string(item.WorkspaceID))
		}
	}
	for _, workspaceID := range workspaceIDs {
		if _, ok := observed[workspaceID]; !ok {
			return fmt.Errorf("eventstream high-watermark query omitted workspace %s", workspaceID)
		}
	}
	return nil
}

func equalProjectionWorkspaceIDs(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
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

// ReadWorkspaceActivity returns the store-owned activity feed for a workspace.
// RuntimeView must page this feed and must not regenerate ActivityRevision.
func (m *Manager) ReadWorkspaceActivity(workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool) {
	if m == nil || m.store == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	return m.store.GetActivity(workspaceID)
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
	previous, _, _ := m.store.GetSnapshot(snapshot.Workspace.ID)
	m.store.UpsertSnapshot(snapshot)
	m.publishProjectionChangedWithDiff(context.Background(), snapshot.Workspace.ID, "detail", previous, snapshot)
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

func (m *Manager) ApplyRunCommandQueued(update RunCommandQueuedUpdate) bool {
	if m == nil || m.store == nil {
		return false
	}
	if !m.store.ApplyRunCommandQueued(update) {
		return false
	}
	m.publishProjectionChanged(context.Background(), update.WorkspaceID, "run_command_queued")
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
		// A delete is authoritative. EventStream may deliver a pre-delete
		// workspace record after the HTTP deletion path removed its projection.
		// When both owner reads confirm not-found, converge by removing the
		// projection entry instead of degrading global readiness.
		if errors.Is(err, common.ErrNotFound) && errors.Is(readErr, common.ErrNotFound) {
			m.DeleteWorkspace(workspaceID)
			return nil
		}
		if readErr == nil {
			m.store.UpsertWorkspace(workspace)
			m.publishProjectionChanged(ctx, workspaceID, "overview")
		}
		m.store.MarkWorkspaceDegraded(workspaceID, err)
		m.publishProjectionChanged(ctx, workspaceID, "detail_failed")
		return err
	}
	previous, _, _ := m.store.GetSnapshot(workspaceID)
	m.store.UpsertSnapshot(snapshot)
	m.publishProjectionChangedWithDiff(ctx, workspaceID, "detail", previous, snapshot)
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

func (m *Manager) HandleEventRecord(_ context.Context, record eventstreamevents.EventRecordData) error {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(record.Topic) == "" || record.Topic == workorchdevents.TopicWorkspaceProjectionChanged {
		return nil
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
	var previous, candidate readmodelcore.WorkspaceSnapshot
	if m != nil && m.store != nil && strings.TrimSpace(workspaceID) != "" {
		if projected, _, err := m.store.GetSnapshot(workspaceID); err == nil {
			candidate = projected
			previous = projected
		}
	}
	m.publishProjectionChangedWithDiff(ctx, workspaceID, changeKind, previous, candidate)
}

func (m *Manager) publishProjectionChangedWithDiff(ctx context.Context, workspaceID string, changeKind string, previous, candidate readmodelcore.WorkspaceSnapshot) {
	if m == nil || m.eventPorts.PublishChanged == nil || m.store == nil {
		return
	}
	status := m.store.StatusForWorkspace(workspaceID)
	if strings.TrimSpace(workspaceID) != "" && strings.TrimSpace(candidate.Workspace.ID) == "" {
		if projected, _, err := m.store.GetSnapshot(workspaceID); err == nil {
			candidate = projected
		}
	}
	runID := common.RunID(readmodelprojection.SelectDisplayRunID(candidate))
	var runtimeStateRevision uint64
	var evidenceSequence uint64
	activityRevision := int64(0)
	if runtime := candidate.TaskProgress.Runtime; runtime != nil {
		runtimeStateRevision = runtime.StateRevision
		evidenceSequence = runtime.EvidenceSequence
	}
	// ActivityRevision authority is the store-owned transaction revision.
	if feed, ok := m.store.GetActivity(workspaceID); ok {
		activityRevision = feed.ActivityRevision
	} else {
		// Snapshot may not yet have activity committed (overview-only path).
		activityRevision = 0
	}
	changedSections := readmodelprojection.DiffWorkspaceProjectionSections(previous, candidate, changeKind)
	portSections := make([]workorchport.WorkspaceProjectionSection, 0, len(changedSections))
	for _, section := range changedSections {
		portSections = append(portSections, workorchport.WorkspaceProjectionSection(section))
	}
	data := workorchdevents.WorkspaceProjectionChangedData{
		WorkspaceID:             common.WorkspaceID(workspaceID),
		RunID:                   runID,
		StreamGeneration:        status.StreamGeneration,
		GlobalRevision:          status.GlobalRevision,
		WorkspaceRevision:       status.WorkspaceRevision,
		SourceEventRevision:     status.SourceEventRevision,
		ProjectedSourceRevision: status.ProjectedSourceRevision,
		RuntimeStateRevision:    runtimeStateRevision,
		EvidenceSequence:        evidenceSequence,
		ActivityRevision:        activityRevision,
		WorkspaceTotal:          status.WorkspaceTotal,
		ChangeKind:              changeKind,
		ChangedSections:         portSections,
		ProjectionState:         status.State,
		UpdatedAt:               status.UpdatedAt,
		RebuiltAt:               status.RebuiltAt,
		StaleReason:             status.StaleReason,
		Error:                   status.Error,
	}
	m.eventPorts.PublishChanged(ctx, data)
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
