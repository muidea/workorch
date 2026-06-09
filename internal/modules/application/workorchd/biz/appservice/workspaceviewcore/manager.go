package workspaceviewcore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Manager struct {
	loader   Loader
	hub      events.Hub
	schedule Scheduler
	store    *Store

	mu                 sync.Mutex
	unsubscribers      []func()
	started            bool
	closed             bool
	rebuildAllInFlight bool
	rebuildAllPending  bool
	rebuilding         map[string]struct{}
	rebuildPending     map[string]struct{}
}

func NewManager(options ManagerOptions) *Manager {
	return &Manager{
		loader:         options.Loader,
		hub:            options.Hub,
		schedule:       options.Schedule,
		store:          NewStore(),
		rebuilding:     map[string]struct{}{},
		rebuildPending: map[string]struct{}{},
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
		if m.hub != nil {
			m.unsubscribers = events.SubscribeWorkorch(m.hub, func(ctx context.Context, event events.WorkorchEvent) error {
				return m.handleEvent(ctx, event.Envelope())
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

func (m *Manager) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if m == nil || m.store == nil {
		return
	}
	m.store.UpsertWorkspace(workspace)
	m.publishProjectionChanged(context.Background(), workspace.ID, "overview")
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

func (m *Manager) rebuildAll(ctx context.Context) error {
	m.store.setBuilding()
	workspaces, err := m.loader.ListWorkspaces(ctx)
	if err != nil {
		m.store.setFailed(err)
		return err
	}
	snapshots := make(map[string]readmodelcore.WorkspaceSnapshot, len(workspaces))
	var degraded error
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.ID) == "" {
			continue
		}
		snapshot, err := m.loader.ReadWorkspaceSnapshot(ctx, workspace.ID)
		if err != nil {
			degraded = errors.Join(degraded, fmt.Errorf("%s: %w", workspace.ID, err))
			continue
		}
		snapshots[workspace.ID] = snapshot
	}
	if m.isClosed() {
		return fmt.Errorf("%w: workspace projection manager closed", common.ErrConflict)
	}
	m.store.ReplaceAll(workspaces, snapshots, degraded)
	m.publishProjectionChanged(ctx, "", "rebuild")
	return nil
}

func (m *Manager) RebuildWorkspace(ctx context.Context, workspaceID string) error {
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
	snapshot, err := m.loader.ReadWorkspaceSnapshot(ctx, workspaceID)
	if err != nil {
		workspace, readErr := m.loader.ReadWorkspace(ctx, workspaceID)
		if readErr == nil {
			m.store.UpsertWorkspace(workspace)
			m.publishProjectionChanged(ctx, workspaceID, "overview")
		}
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
	if _, ok := m.rebuilding[workspaceID]; ok {
		m.rebuildPending[workspaceID] = struct{}{}
		m.mu.Unlock()
		return
	}
	m.rebuilding[workspaceID] = struct{}{}
	m.mu.Unlock()
	if !m.submit("workspace_projection_rebuild", func(ctx context.Context) {
		if err := m.RebuildWorkspace(ctx, workspaceID); err != nil && !m.isClosed() {
			slog.Warn("workorch workspace projection rebuild failed", "workspaceID", workspaceID, "err", err)
		}
		reschedule := false
		m.mu.Lock()
		if _, ok := m.rebuildPending[workspaceID]; ok && !m.closed {
			delete(m.rebuildPending, workspaceID)
			reschedule = true
		}
		delete(m.rebuilding, workspaceID)
		m.mu.Unlock()
		if reschedule {
			m.ScheduleRebuildWorkspace(workspaceID)
		}
	}) {
		m.mu.Lock()
		delete(m.rebuilding, workspaceID)
		delete(m.rebuildPending, workspaceID)
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
	reschedule := false
	m.mu.Lock()
	if m.rebuildAllPending && !m.closed {
		m.rebuildAllPending = false
		reschedule = true
	}
	m.rebuildAllInFlight = false
	m.mu.Unlock()
	if reschedule {
		m.ScheduleRebuildAll()
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

func (m *Manager) handleEvent(_ context.Context, envelope events.Envelope) error {
	if m == nil {
		return nil
	}
	if envelope.Topic == workorchdevents.TopicWorkspaceProjectionChanged {
		return nil
	}
	if strings.TrimSpace(envelope.WorkspaceID) != "" {
		m.store.MarkStale(envelope.WorkspaceID, envelope.Revision)
		if m.isRebuildAllInFlight() {
			m.ScheduleRebuildAll()
			return nil
		}
		m.ScheduleRebuildWorkspace(envelope.WorkspaceID)
		return nil
	}
	switch strings.TrimSpace(envelope.SubjectType) {
	case "workspace":
		m.store.MarkStale("", envelope.Revision)
		m.ScheduleRebuildAll()
	}
	return nil
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
	if m == nil || m.hub == nil || m.store == nil {
		return
	}
	status := m.store.StatusForWorkspace(workspaceID)
	payload := map[string]any{
		"type":              "workspace_projection_changed",
		"workspaceId":       workspaceID,
		"globalRevision":    status.GlobalRevision,
		"workspaceRevision": status.WorkspaceRevision,
		"workspaceTotal":    status.WorkspaceTotal,
		"changeKind":        changeKind,
		"projectionState":   status.State,
		"updatedAt":         status.UpdatedAt,
	}
	if status.Error != "" {
		payload["error"] = status.Error
	}
	envelope := events.Envelope{
		Topic:        workorchdevents.TopicWorkspaceProjectionChanged,
		SourceModule: "workorchd.workspace_projection",
		SubjectType:  "workspace_projection",
		SubjectID:    workspaceID,
		WorkspaceID:  workspaceID,
		Payload:      payload,
	}
	var err error
	if asyncHub, ok := m.hub.(events.AsyncHub); ok {
		err = asyncHub.PublishAsync(ctx, envelope)
	} else {
		err = m.hub.Publish(ctx, envelope)
	}
	if err != nil {
		slog.Warn("workorch workspace projection change event publish failed", "workspaceID", workspaceID, "changeKind", changeKind, "err", err)
	}
}
