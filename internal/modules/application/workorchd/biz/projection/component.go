// Package projection owns the application read-model projection lifecycle.
// It is deliberately separate from the HTTP transport: projection writes are
// derived observations and never issue Runtime/Run/Workspace commands.
package projection

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

const rebuildCoalesceDelay = 100 * time.Millisecond

// Component is the single Biz-owned projection manager. The wrapped manager
// remains the only mutable projection store; this component adds lifecycle
// ownership and bounded background scheduling around it.
type Component struct {
	manager    *workspaceviewcore.Manager
	background task.BackgroundRoutine
	ctx        context.Context
	cancel     context.CancelFunc

	mu      sync.RWMutex
	closed  bool
	pending sync.WaitGroup
}

func New(loader workspaceviewcore.Loader, background task.BackgroundRoutine, ports workspaceviewcore.EventPorts) *Component {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Component{background: background, ctx: ctx, cancel: cancel}
	c.manager = workspaceviewcore.NewManager(workspaceviewcore.ManagerOptions{
		Loader:               loader,
		Schedule:             c.schedule,
		EventPorts:           ports,
		RebuildCoalesceDelay: rebuildCoalesceDelay,
	})
	return c
}

func (c *Component) SetEventPorts(ports workspaceviewcore.EventPorts) {
	if c == nil || c.manager == nil {
		return
	}
	c.manager.SetEventPorts(ports)
}

func (c *Component) Start(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	return c.manager.Start(ctx)
}

func (c *Component) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.CloseContext(ctx)
}

// CloseContext prevents new projection work and waits for already accepted
// callbacks without shutting down the shared framework routine.
func (c *Component) CloseContext(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		if c.cancel != nil {
			c.cancel()
		}
	}
	c.mu.Unlock()
	if c.manager != nil {
		c.manager.Close()
	}
	done := make(chan struct{})
	go func() {
		c.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Component) Status() projectioncontract.ProjectionStatus {
	if c == nil || c.manager == nil {
		return projectioncontract.ProjectionStatus{State: workspaceviewcore.StateFailed, Error: "workspace projection is not configured"}
	}
	return c.manager.Status()
}

func (c *Component) Metrics() workspaceviewcore.Metrics {
	if c == nil || c.manager == nil {
		return workspaceviewcore.Metrics{}
	}
	return c.manager.Metrics()
}

func (c *Component) ReconcileSourceHighWatermarks(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	return c.manager.ReconcileSourceHighWatermarks(ctx)
}

func (c *Component) ListWorkspaces() ([]appmodelcore.Workspace, projectioncontract.ProjectionStatus, error) {
	if c == nil || c.manager == nil {
		return nil, projectioncontract.ProjectionStatus{State: workspaceviewcore.StateFailed}, errors.New("workspace projection is not configured")
	}
	return c.manager.ListWorkspaces()
}

func (c *Component) ReadWorkspaceSnapshotWithContext(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if c == nil || c.manager == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: workspaceviewcore.StateFailed}, errors.New("workspace projection is not configured")
	}
	return c.manager.ReadWorkspaceSnapshotWithContext(ctx, workspaceID)
}

func (c *Component) ReadWorkspaceSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if c == nil || c.manager == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: workspaceviewcore.StateFailed}, errors.New("workspace projection is not configured")
	}
	return c.manager.ReadWorkspaceSummarySnapshot(workspaceID)
}

func (c *Component) ReadWorkspaceActivity(workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool) {
	if c == nil || c.manager == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	return c.manager.ReadWorkspaceActivity(workspaceID)
}

func (c *Component) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if c != nil && c.manager != nil {
		c.manager.UpsertWorkspace(workspace)
	}
}

func (c *Component) UpsertSnapshot(snapshot readmodelcore.WorkspaceSnapshotState) {
	if c != nil && c.manager != nil {
		c.manager.UpsertSnapshot(snapshot)
	}
}

func (c *Component) ApplyRunCommandAccepted(update workspaceviewcore.RunCommandAcceptedUpdate) bool {
	return c != nil && c.manager != nil && c.manager.ApplyRunCommandAccepted(update)
}

func (c *Component) ApplyRunCommandRejected(update workspaceviewcore.RunCommandRejectedUpdate) bool {
	return c != nil && c.manager != nil && c.manager.ApplyRunCommandRejected(update)
}

func (c *Component) DeleteWorkspace(workspaceID string) {
	if c != nil && c.manager != nil {
		c.manager.DeleteWorkspace(workspaceID)
	}
}

func (c *Component) RebuildAll(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	return c.manager.RebuildAll(ctx)
}

func (c *Component) RebuildWorkspace(ctx context.Context, workspaceID string) error {
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	return c.manager.RebuildWorkspace(ctx, workspaceID)
}

// RebuildWorkspacesForDecisions refreshes each affected workspace once. The
// input is a governance observation; this component only updates the derived
// read model and never resolves the decision itself.
func (c *Component) RebuildWorkspacesForDecisions(ctx context.Context, items []projectioncontract.Decision) error {
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.WorkspaceID == "" {
			continue
		}
		if _, ok := seen[item.WorkspaceID]; ok {
			continue
		}
		seen[item.WorkspaceID] = struct{}{}
		ids = append(ids, item.WorkspaceID)
	}
	return c.rebuildWorkspaceIDs(ctx, ids)
}

// RebuildWorkspacesForApprovals is the approval counterpart of
// RebuildWorkspacesForDecisions. Deduplication belongs to the projection
// component so transport handlers do not own projection scheduling policy.
func (c *Component) RebuildWorkspacesForApprovals(ctx context.Context, items []projectioncontract.Approval) error {
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.WorkspaceID == "" {
			continue
		}
		if _, ok := seen[item.WorkspaceID]; ok {
			continue
		}
		seen[item.WorkspaceID] = struct{}{}
		ids = append(ids, item.WorkspaceID)
	}
	return c.rebuildWorkspaceIDs(ctx, ids)
}

func (c *Component) rebuildWorkspaceIDs(ctx context.Context, workspaceIDs []string) error {
	if len(workspaceIDs) == 0 {
		return nil
	}
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	var errs []error
	for _, workspaceID := range workspaceIDs {
		if err := c.RebuildWorkspace(ctx, workspaceID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Component) HandleEventRecord(ctx context.Context, record eventstreamevents.EventRecordData) error {
	if c == nil || c.manager == nil {
		return errors.New("workspace projection is not configured")
	}
	return c.manager.HandleEventRecord(ctx, record)
}

func (c *Component) HandleRuntimeDriveObserved(data orchestratorevents.RuntimeDriveObservedData) {
	if c != nil && c.manager != nil {
		c.manager.HandleRuntimeDriveObserved(data)
	}
}

func (c *Component) schedule(name string, fn func(context.Context)) bool {
	if c == nil || fn == nil || c.background == nil {
		return false
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.pending.Add(1)
	ctx := c.ctx
	c.mu.Unlock()
	if err := c.background.AsyncFunction(func() {
		defer c.pending.Done()
		c.mu.RLock()
		closed := c.closed
		c.mu.RUnlock()
		if closed {
			return
		}
		fn(ctx)
	}); err != nil {
		c.pending.Done()
		return false
	}
	_ = name // retained for manager diagnostics and future task metrics
	return true
}
