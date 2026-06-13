package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

const runWatchdogInterval = 30 * time.Second

type runContinuationScheduler struct {
	state *serverState

	recoveryMu             sync.Mutex
	startupRecoveryStarted bool
	periodicRecoveryTimer  bool
	staleRecoveryRunning   bool

	replanMu      sync.Mutex
	replanStarted map[string]struct{}

	governanceMu      sync.Mutex
	governanceStarted map[string]struct{}
}

func newRunContinuationScheduler(state *serverState) *runContinuationScheduler {
	return &runContinuationScheduler{
		state:             state,
		replanStarted:     map[string]struct{}{},
		governanceStarted: map[string]struct{}{},
	}
}

func (s *runContinuationScheduler) scheduleStartupRecovery() {
	if s == nil || s.state == nil {
		return
	}
	if !s.markStartupRecoveryStarted() {
		return
	}
	state := s.state
	state.scheduleBackground("startup_recovery", func(ctx context.Context) {
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		if err := s.recoverStuckRunningRuns(appservice.ContextWithEventHub(ctx, state.eventHub)); err != nil {
			slog.Warn("workorch startup run recovery failed", "err", err)
		}
	})
	s.schedulePeriodicStaleDispatchedRecovery()
}

func (s *runContinuationScheduler) schedulePeriodicStaleDispatchedRecovery() {
	if s == nil || s.state == nil || s.state.backgroundRoutine == nil {
		return
	}
	if !s.markPeriodicRecoveryTimerStarted() {
		return
	}
	state := s.state
	err := state.backgroundRoutine.Timer(
		state.backgroundContext(),
		backgroundFuncTask{fn: func() {
			ctx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
			if err := s.recoverStaleDispatchedRuns(ctx); err != nil {
				slog.Warn("workorch stale dispatched run recovery failed", "err", err)
			}
			if err := s.recoverResolvedGovernanceBlockedRuns(ctx); err != nil {
				slog.Warn("workorch resolved governance blocked run recovery failed", "err", err)
			}
		}},
		runWatchdogInterval,
		0,
	)
	if err != nil {
		s.clearPeriodicRecoveryTimerStarted()
		slog.Warn("workorch stale dispatched run recovery timer failed", "err", err)
	}
}

func (s *runContinuationScheduler) recoverStuckRunningRuns(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	state := s.state
	var recovered []appservice.Run
	var err error
	if state.hasSharedSession() {
		err = state.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var recoverErr error
			recovered, recoverErr = appservice.RecoverStuckRunningRunsWithSession(ctx, session)
			return recoverErr
		})
	} else {
		recovered, err = appservice.RecoverStuckRunningRunsWithBackend(ctx, state.config.Backend, state.config.DataRoot)
	}
	if err != nil {
		return err
	}
	if len(recovered) > 0 {
		runIDs := make([]string, 0, len(recovered))
		for _, item := range recovered {
			runIDs = append(runIDs, item.ID)
		}
		slog.Warn("workorch recovered stuck running runs", "runIDs", runIDs)
	}
	if err := s.recoverResolvedGovernanceBlockedRuns(ctx); err != nil {
		slog.Warn("workorch resolved governance blocked run recovery failed", "err", err)
	}
	s.recoverReplanContinuations(ctx)
	return nil
}

func (s *runContinuationScheduler) recoverStaleDispatchedRuns(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	if !s.markStaleRecoveryRunning() {
		return nil
	}
	defer s.clearStaleRecoveryRunning()

	state := s.state
	release, err := state.acquireRunSlotWithPriority(ctx, 0)
	if err != nil {
		state.metrics.runCanceled.Add(1)
		return err
	}
	defer release()

	var recovered []appservice.Run
	minAge := state.config.StaleDispatchedStepMinAge
	if minAge <= 0 {
		minAge = appservice.DefaultStaleDispatchedStepMinAge
	}
	if state.hasSharedSession() {
		err = state.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var recoverErr error
			recovered, recoverErr = appservice.RecoverStaleDispatchedRunsWithSession(ctx, session, minAge, time.Now().UTC())
			return recoverErr
		})
	} else {
		recovered, err = appservice.RecoverStaleDispatchedRunsWithBackend(ctx, state.config.Backend, state.config.DataRoot, minAge, time.Now().UTC())
	}
	if err != nil {
		return err
	}
	if len(recovered) == 0 {
		return nil
	}
	runIDs := make([]string, 0, len(recovered))
	for _, item := range recovered {
		runIDs = append(runIDs, item.ID)
		state.notifyRunStatusChangedForRun(item)
	}
	slog.Warn("workorch recovered stale dispatched runs", "runIDs", runIDs)
	return nil
}

func (s *runContinuationScheduler) recoverResolvedGovernanceBlockedRuns(ctx context.Context) error {
	if s == nil || s.state == nil || s.state.listWorkspaces == nil || s.state.listRuns == nil {
		return nil
	}
	state := s.state
	workspaces, err := state.listWorkspaces(ctx, state.config.Backend, state.config.DataRoot)
	if err != nil {
		return err
	}
	var refs []governanceRunResumeRef
	for _, workspace := range workspaces {
		workspaceID := strings.TrimSpace(workspace.ID)
		if workspaceID == "" {
			continue
		}
		runs, err := state.listRuns(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID)), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			slog.Warn("workorch governance blocked run recovery run scan failed", "workspaceID", workspaceID, "err", err)
			continue
		}
		for _, run := range runs {
			if ref, ok := s.resolvedGovernanceResumeRefForRun(ctx, workspaceID, run); ok {
				refs = append(refs, ref)
			}
		}
	}
	if len(refs) == 0 {
		return nil
	}
	s.scheduleGovernanceRunResumeRefs(refs, 0)
	return nil
}

func (s *runContinuationScheduler) resolvedGovernanceResumeRefForRun(ctx context.Context, workspaceID string, run appservice.Run) (governanceRunResumeRef, bool) {
	if s == nil || s.state == nil {
		return governanceRunResumeRef{}, false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(run.WorkspaceID)
	}
	runID := strings.TrimSpace(run.ID)
	if workspaceID == "" || runID == "" {
		return governanceRunResumeRef{}, false
	}
	switch common.RunState(run.State) {
	case common.RunStateBlockedOnDecision:
		if strings.TrimSpace(run.BlockingTicketID) == "" || !s.runHasResolvedDecision(ctx, workspaceID, runID, run.BlockingTicketID) {
			return governanceRunResumeRef{}, false
		}
	case common.RunStateBlockedOnApproval:
		if strings.TrimSpace(run.BlockingApprovalID) == "" || !s.runHasResolvedApproval(ctx, workspaceID, runID, run.BlockingApprovalID) {
			return governanceRunResumeRef{}, false
		}
	default:
		return governanceRunResumeRef{}, false
	}
	return governanceRunResumeRef{WorkspaceID: workspaceID, RunID: runID}, true
}

func (s *runContinuationScheduler) runHasResolvedDecision(ctx context.Context, workspaceID string, runID string, ticketID string) bool {
	if s == nil || s.state == nil || s.state.listDecisions == nil {
		return false
	}
	items, err := s.state.listDecisions(ctx, s.state.config.Backend, s.state.config.DataRoot, workspaceID, runID)
	if err != nil {
		slog.Warn("workorch governance blocked run recovery decision lookup failed", "workspaceID", workspaceID, "runID", runID, "ticketID", ticketID, "err", err)
		return false
	}
	for _, item := range items {
		if item.ID == ticketID &&
			item.WorkspaceID == workspaceID &&
			item.RunID == runID &&
			item.State == string(common.DecisionTicketStateResolved) {
			return true
		}
	}
	return false
}

func (s *runContinuationScheduler) runHasResolvedApproval(ctx context.Context, workspaceID string, runID string, approvalID string) bool {
	if s == nil || s.state == nil || s.state.listApprovals == nil {
		return false
	}
	items, err := s.state.listApprovals(ctx, s.state.config.Backend, s.state.config.DataRoot, workspaceID, runID)
	if err != nil {
		slog.Warn("workorch governance blocked run recovery approval lookup failed", "workspaceID", workspaceID, "runID", runID, "approvalID", approvalID, "err", err)
		return false
	}
	for _, item := range items {
		if item.ID == approvalID &&
			item.WorkspaceID == workspaceID &&
			item.RunID == runID &&
			(item.State == string(common.ApprovalStateApproved) || item.State == string(common.ApprovalStateRejected) || item.State == string(common.ApprovalStateResolved)) {
			return true
		}
	}
	return false
}

func (s *runContinuationScheduler) markStartupRecoveryStarted() bool {
	if s == nil {
		return false
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if s.startupRecoveryStarted {
		return false
	}
	s.startupRecoveryStarted = true
	return true
}

func (s *runContinuationScheduler) markPeriodicRecoveryTimerStarted() bool {
	if s == nil {
		return false
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if s.periodicRecoveryTimer {
		return false
	}
	s.periodicRecoveryTimer = true
	return true
}

func (s *runContinuationScheduler) clearPeriodicRecoveryTimerStarted() {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	s.periodicRecoveryTimer = false
}

func (s *runContinuationScheduler) markStaleRecoveryRunning() bool {
	if s == nil {
		return false
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if s.staleRecoveryRunning {
		return false
	}
	s.staleRecoveryRunning = true
	return true
}

func (s *runContinuationScheduler) clearStaleRecoveryRunning() {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	s.staleRecoveryRunning = false
}

type backgroundFuncTask struct {
	fn func()
}

var _ magictask.Task = backgroundFuncTask{}

func (t backgroundFuncTask) Run() {
	if t.fn != nil {
		t.fn()
	}
}

func (s *runContinuationScheduler) recoverReplanContinuations(ctx context.Context) {
	if s == nil || s.state == nil || s.state.listWorkspaces == nil || s.state.listRuns == nil {
		return
	}
	state := s.state
	workspaces, err := state.listWorkspaces(ctx, state.config.Backend, state.config.DataRoot)
	if err != nil {
		slog.Warn("workorch replan continuation workspace scan failed", "err", err)
		return
	}
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.ID) == "" ||
			!workspaceNeedsReplanContinuationRecovery(workspace) {
			continue
		}
		runs, err := state.listRuns(ctx, state.config.Backend, state.config.DataRoot, workspace.ID)
		if err != nil {
			slog.Warn("workorch replan continuation run scan failed", "workspaceID", workspace.ID, "err", err)
			continue
		}
		for _, run := range runs {
			if run.WorkspaceID != workspace.ID || run.State != string(common.RunStateSuperseded) {
				continue
			}
			if activeRunID := strings.TrimSpace(workspace.ActiveRunID); activeRunID != "" && run.ID != activeRunID {
				continue
			}
			s.scheduleReplanContinuationIfNeeded(ctx, workspace.ID, run.ID, run.Priority)
		}
	}
}

func workspaceNeedsReplanContinuationRecovery(workspace appservice.Workspace) bool {
	state := common.WorkspaceState(strings.TrimSpace(workspace.State))
	activeRunID := strings.TrimSpace(workspace.ActiveRunID)
	if state == common.WorkspaceStateFailed && activeRunID == "" {
		return true
	}
	if activeRunID == "" {
		return false
	}
	switch state {
	case common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked:
		return true
	default:
		return false
	}
}

func (s *runContinuationScheduler) scheduleGovernanceRunResumeRefs(refs []governanceRunResumeRef, priority int) {
	if s == nil || s.state == nil {
		return
	}
	state := s.state
	refs = uniqueGovernanceRunResumeRefs(refs)
	for _, ref := range refs {
		ref := ref
		key := governanceRunResumeKey(ref)
		if !s.markGovernanceResumeStarted(key) {
			slog.Debug("workorch governance resume already scheduled", "workspaceID", ref.WorkspaceID, "runID", ref.RunID)
			continue
		}
		if !state.scheduleBackground("governance_run_resume", func(context.Context) {
			defer s.clearGovernanceResumeStarted(key)
			var resumedRun appservice.Run
			defer func() {
				if resumedRun.ID != "" {
					state.notifyRunStatusChangedForRun(resumedRun)
					return
				}
				state.notifyRunStatusChanged()
			}()
			ctx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
			state.metrics.runResumeRequests.Add(1)
			release, err := state.acquireRunSlotWithPriority(ctx, priority)
			if err != nil {
				state.metrics.runCanceled.Add(1)
				slog.Warn("workorch governance resume queue failed", "workspaceID", ref.WorkspaceID, "runID", ref.RunID, "err", err)
				return
			}
			defer release()
			if ref.WorkspaceID != "" {
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(ref.WorkspaceID))
			}
			item, err := state.resumeRun(ctx, state.config.Backend, state.config.DataRoot, ref.RunID)
			if err != nil {
				slog.Warn("workorch governance resume failed", "workspaceID", ref.WorkspaceID, "runID", ref.RunID, "err", err)
				return
			}
			state.metrics.observeRunOutcome(item)
			if item.State == string(common.RunStateSuperseded) {
				s.scheduleReplanContinuationIfNeeded(ctx, item.WorkspaceID, item.ID, priority)
			}
			resumedRun = item
		}) {
			s.clearGovernanceResumeStarted(key)
		}
	}
}

func (s *runContinuationScheduler) markGovernanceResumeStarted(key string) bool {
	if s == nil {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	if s.governanceStarted == nil {
		s.governanceStarted = map[string]struct{}{}
	}
	if _, ok := s.governanceStarted[key]; ok {
		return false
	}
	s.governanceStarted[key] = struct{}{}
	return true
}

func (s *runContinuationScheduler) clearGovernanceResumeStarted(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	delete(s.governanceStarted, key)
}

func (s *runContinuationScheduler) scheduleReplanContinuationIfNeeded(ctx context.Context, workspaceID string, runID string, priority int) {
	if s == nil || s.state == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return
	}
	state := s.state
	if !state.runHasResolvedReplanDecision(ctx, workspaceID, runID) {
		return
	}
	if s.workspaceHasReplanSuccessor(ctx, workspaceID, runID) {
		return
	}
	key := workspaceID + "/" + runID
	if !s.markReplanContinuationStarted(key) {
		return
	}

	if !state.scheduleBackground("replan_continuation", func(context.Context) {
		runCtx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
		release, err := state.acquireRunSlotWithPriority(runCtx, priority)
		if err != nil {
			s.clearReplanContinuationStarted(key)
			state.metrics.runCanceled.Add(1)
			slog.Warn("workorch replan continuation queue failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			return
		}
		defer release()
		activation, err := state.activateWorkspace(runCtx, state.config.Backend, state.config.DataRoot, workspaceID, appservice.ActivateWorkspaceExecutionCommand{Priority: priority})
		if err != nil {
			s.clearReplanContinuationStarted(key)
			slog.Warn("workorch replan continuation activation failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			state.notifyRunStatusChanged()
			return
		}
		state.metrics.observeRunOutcome(activation.Run)
		state.notifyRunStatusChangedForRun(activation.Run)
	}) {
		s.clearReplanContinuationStarted(key)
	}
}

func (s *runContinuationScheduler) markReplanContinuationStarted(key string) bool {
	if s == nil {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.replanMu.Lock()
	defer s.replanMu.Unlock()
	if s.replanStarted == nil {
		s.replanStarted = map[string]struct{}{}
	}
	if _, ok := s.replanStarted[key]; ok {
		return false
	}
	s.replanStarted[key] = struct{}{}
	return true
}

func (s *runContinuationScheduler) clearReplanContinuationStarted(key string) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.replanMu.Lock()
	defer s.replanMu.Unlock()
	delete(s.replanStarted, key)
}

func (s *runContinuationScheduler) workspaceHasReplanSuccessor(ctx context.Context, workspaceID string, runID string) bool {
	if s == nil || s.state == nil || s.state.listRuns == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return false
	}
	state := s.state
	items, err := state.listRuns(ctx, state.config.Backend, state.config.DataRoot, workspaceID)
	if err != nil {
		slog.Warn("workorch replan continuation successor scan failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		return false
	}
	for _, item := range items {
		if item.WorkspaceID != workspaceID || item.ID == runID {
			continue
		}
		if runIsReplanSuccessorOf(item, runID) {
			return true
		}
	}
	return false
}
