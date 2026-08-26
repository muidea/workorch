package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const (
	runProjectionReconciliationInterval = time.Second
	runProjectionInventoryPageLimit     = 100
	runProjectionInventoryMaxPages      = 1000
	runProjectionInventoryMaxItems      = 100000
)

// startRunProjectionReconciliation marks startup inventory dirty before Run
// commands are necessarily available. The periodic owner-local worker keeps
// retrying the current Run fact until the Workspace CAS succeeds. No retry
// state is authoritative: a fresh process reconstructs it from Run inventory.
func (m *Biz) startRunProjectionReconciliation() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.runProjectionStateMu.Lock()
	if m.runProjectionCancel != nil {
		m.runProjectionCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.runProjectionCtx = ctx
	m.runProjectionCancel = cancel
	m.runProjectionPending = true
	m.runProjectionGeneration++
	m.runProjectionLastError = nil
	m.runProjectionStateMu.Unlock()

	background := m.BackgroundRoutine()
	if background == nil {
		// Tests and explicit minimal assemblies may omit a BackgroundRoutine.
		// Perform the same bounded current-fact read synchronously; later hints
		// use the same fallback and therefore cannot leave permanent dirty state.
		m.reconcileRunProjections()
		m.reconcileWorkspaceDeletionInventory(ctx)
		return nil
	}
	if err := background.Timer(ctx, &workspaceRunProjectionReconciliationTask{owner: m}, runProjectionReconciliationInterval, 0); err != nil {
		m.stopRunProjectionReconciliation()
		return err
	}
	if err := background.AsyncFunction(func() {
		m.reconcileRunProjections()
		m.reconcileWorkspaceDeletionInventory(ctx)
	}); err != nil {
		m.stopRunProjectionReconciliation()
		return err
	}
	return nil
}

type workspaceRunProjectionReconciliationTask struct{ owner *Biz }

func (t *workspaceRunProjectionReconciliationTask) Run() {
	if t != nil && t.owner != nil {
		t.owner.heartbeatRunProjectionReconciliation()
		t.owner.runProjectionStateMu.Lock()
		ctx := t.owner.runProjectionCtx
		t.owner.runProjectionStateMu.Unlock()
		if ctx != nil && ctx.Err() == nil {
			t.owner.reconcileWorkspaceDeletionInventory(ctx)
		}
	}
}

// heartbeatRunProjectionReconciliation makes every periodic tick an
// unconditional current-fact inventory pass. Notifications and process-local
// pending state are only latency hints: a fact inserted behind a cursor while
// a non-snapshot paginated scan is in progress is discovered by the next full
// scan from cursor zero even when its notification was lost.
func (m *Biz) heartbeatRunProjectionReconciliation() {
	if m == nil {
		return
	}
	m.runProjectionStateMu.Lock()
	if m.runProjectionCtx == nil || m.runProjectionCtx.Err() != nil {
		m.runProjectionStateMu.Unlock()
		return
	}
	m.runProjectionPending = true
	m.runProjectionGeneration++
	m.runProjectionStateMu.Unlock()
	m.reconcileRunProjections()
}

func (m *Biz) stopRunProjectionReconciliation() {
	if m == nil {
		return
	}
	m.runProjectionStateMu.Lock()
	cancel := m.runProjectionCancel
	m.runProjectionCancel = nil
	m.runProjectionCtx = nil
	m.runProjectionPending = false
	m.runProjectionGeneration++
	m.runProjectionStateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Wait for a current inventory/reducer pass before repositories are cleared.
	m.runProjectionWorkMu.Lock()
	m.runProjectionWorkMu.Unlock()
	// The same owner-local worker also drives persisted deletion journals. Wait
	// for an in-flight cross-owner fact/reducer pass before Teardown clears the
	// Workspace repositories.
	m.deletionWorkMu.Lock()
	m.deletionWorkMu.Unlock()
}

func (m *Biz) requestRunProjectionReconciliation(cause error) {
	if m == nil {
		return
	}
	m.runProjectionStateMu.Lock()
	if m.runProjectionCtx == nil || m.runProjectionCtx.Err() != nil {
		m.runProjectionStateMu.Unlock()
		return
	}
	m.runProjectionPending = true
	m.runProjectionGeneration++
	if cause != nil {
		m.runProjectionLastError = cause
	}
	m.runProjectionStateMu.Unlock()

	background := m.BackgroundRoutine()
	if background == nil {
		m.reconcileRunProjections()
		return
	}
	if err := background.AsyncFunction(func() { m.reconcileRunProjections() }); err != nil {
		m.runProjectionStateMu.Lock()
		m.runProjectionLastError = err
		m.runProjectionStateMu.Unlock()
		// A stopped/rejected scheduler must not strand a valid owner fact.
		// No projection lock is held here, so the bounded synchronous fallback
		// cannot recurse or deadlock with notification/reconciliation work.
		m.reconcileRunProjections()
	}
}

func (m *Biz) reconcileRunProjections() {
	if m == nil {
		return
	}
	m.runProjectionWorkMu.Lock()
	defer m.runProjectionWorkMu.Unlock()

	m.runProjectionStateMu.Lock()
	ctx := m.runProjectionCtx
	generation := m.runProjectionGeneration
	pending := m.runProjectionPending
	m.runProjectionStateMu.Unlock()
	if !pending || ctx == nil || ctx.Err() != nil {
		return
	}

	err := m.reconcileCurrentRunProjectionFacts(ctx)
	m.runProjectionStateMu.Lock()
	defer m.runProjectionStateMu.Unlock()
	if m.runProjectionCtx != ctx || ctx.Err() != nil {
		return
	}
	if err != nil {
		m.runProjectionPending = true
		m.runProjectionLastError = err
		return
	}
	if m.runProjectionGeneration == generation {
		m.runProjectionPending = false
		m.runProjectionLastError = nil
	}
}

func (m *Biz) reconcileCurrentRunProjectionFacts(ctx context.Context) error {
	if m == nil || m.EventHub() == nil || m.workspaces == nil {
		return common.ErrInvalidArgument
	}
	cursor := ""
	visited := map[string]struct{}{}
	total := 0
	var replayErr error
	for page := 0; page < runProjectionInventoryMaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		response, err := m.queryRunLifecycleFactInventory(ctx, runevents.QueryRunLifecycleFactInventoryCommand{
			Cursor: cursor,
			Limit:  runProjectionInventoryPageLimit,
		})
		if err != nil {
			return err
		}
		if err := response.Validate(); err != nil {
			return err
		}
		total += len(response.Items)
		if total > runProjectionInventoryMaxItems {
			return fmt.Errorf("%w: Run lifecycle fact inventory exceeds item limit", common.ErrConflict)
		}
		for _, fact := range response.Items {
			if err := m.applyRunStateChangedData(ctx, fact); err != nil {
				// A deleted Workspace has no projection aggregate to reconcile.
				if errors.Is(err, common.ErrNotFound) {
					continue
				}
				// One broken Workspace must not prevent current facts for later
				// workspaces in the same bounded inventory from converging. Preserve
				// the error so the pass remains dirty and retries after the scan.
				if replayErr == nil {
					replayErr = fmt.Errorf("Workspace %s Run %s projection: %w", fact.WorkspaceID, fact.RunID, err)
				}
			}
		}
		next := strings.TrimSpace(response.NextCursor)
		if next == "" {
			return replayErr
		}
		if next == cursor {
			return fmt.Errorf("%w: Run lifecycle fact inventory cursor did not advance", common.ErrConflict)
		}
		if _, duplicate := visited[next]; duplicate {
			return fmt.Errorf("%w: Run lifecycle fact inventory cursor cycle", common.ErrConflict)
		}
		visited[next] = struct{}{}
		cursor = next
	}
	return fmt.Errorf("%w: Run lifecycle fact inventory exceeds page limit", common.ErrConflict)
}

func (m *Biz) queryRunLifecycleFactInventory(ctx context.Context, command runevents.QueryRunLifecycleFactInventoryCommand) (runevents.QueryRunLifecycleFactInventoryResult, error) {
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandLifecycleFactInventory,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "run_owner", SubjectID: "lifecycle_fact_inventory",
	}))
	result := m.SendEvent(event)
	if result == nil {
		return runevents.QueryRunLifecycleFactInventoryResult{}, fmt.Errorf("Run lifecycle fact inventory returned no response")
	}
	if err := result.Error(); err != nil {
		return runevents.QueryRunLifecycleFactInventoryResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return runevents.QueryRunLifecycleFactInventoryResult{}, err
	}
	response, ok := value.(runevents.QueryRunLifecycleFactInventoryResult)
	if !ok {
		return runevents.QueryRunLifecycleFactInventoryResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

// applyRunStateChangedNotification serializes the lossy delivery hint with
// current-fact replay and rejects callbacks that arrive after owner shutdown.
// The reconciler already holds runProjectionWorkMu while applying inventory.
func (m *Biz) applyRunStateChangedNotification(ctx context.Context, data runevents.RunStateChangedData) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.runProjectionWorkMu.Lock()
	defer m.runProjectionWorkMu.Unlock()
	m.runProjectionStateMu.Lock()
	ownerCtx := m.runProjectionCtx
	m.runProjectionStateMu.Unlock()
	if ownerCtx == nil || ownerCtx.Err() != nil {
		return common.ErrInvalidArgument
	}
	return m.applyRunStateChangedData(ctx, data)
}

func (m *Biz) applyRunStateChangedData(ctx context.Context, data runevents.RunStateChangedData) error {
	if m == nil || m.workspaces == nil {
		return common.ErrInvalidArgument
	}
	if err := data.Validate(); err != nil {
		return err
	}
	switch data.State {
	case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
	default:
		return common.ErrInvalidArgument
	}
	boundCtx, err := common.BindRuntimeWorkspaceContext(ctx, data.WorkspaceID)
	if err != nil {
		return err
	}
	fact := workspaceRunProjectionFact{
		WorkspaceID: data.WorkspaceID, WorkspaceRunGeneration: data.WorkspaceRunGeneration,
		RunID: data.RunID, RunRevision: data.RunRevision,
		SourceOwner: runevents.CommandDestination, CausationID: data.CausationID, RunState: data.State,
		RuntimeStateRevision: data.RuntimeStateRevision, RuntimeStateRef: data.RuntimeStateRef,
	}
	// The cross-owner Run fact is translated once into the Workspace owner's
	// private internal event. Only that dispatcher/reducer can mutate the
	// Workspace lifecycle projection.
	_, err = m.submitWorkspaceLifecycleInternalEvent(boundCtx, workspaceLifecycleInternalEvent{
		Kind: workspaceLifecycleInternalEventRunProjectionObserved, RunProjection: &fact,
	})
	return err
}
