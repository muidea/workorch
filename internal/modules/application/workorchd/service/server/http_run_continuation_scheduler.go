package server

import (
	"context"
	"errors"
	"fmt"
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

	recoveryMu            sync.Mutex
	periodicRecoveryTimer bool
	staleRecoveryRunning  bool

	replanMu      sync.Mutex
	replanStarted map[string]struct{}

	restartMu      sync.Mutex
	restartStarted map[string]struct{}
	restartPending map[string]workspaceRestartRequest

	runMu      sync.Mutex
	runStarted map[string]struct{}
	runPending map[string]runContinuationRequest

	governanceMu      sync.Mutex
	governanceStarted map[string]struct{}
	governancePending map[string]governanceRunResumeRef
}

func newRunContinuationScheduler(state *serverState) *runContinuationScheduler {
	return &runContinuationScheduler{
		state:             state,
		replanStarted:     map[string]struct{}{},
		restartStarted:    map[string]struct{}{},
		restartPending:    map[string]workspaceRestartRequest{},
		runStarted:        map[string]struct{}{},
		runPending:        map[string]runContinuationRequest{},
		governanceStarted: map[string]struct{}{},
		governancePending: map[string]governanceRunResumeRef{},
	}
}

func (s *runContinuationScheduler) scheduleWatchdog() {
	if s == nil || s.state == nil {
		return
	}
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
			started := time.Now().UTC()
			ctx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
			err := s.recoverStaleDispatchedRuns(ctx)
			if state.schedulerDiagnostics != nil {
				state.schedulerDiagnostics.recordWatchdogTick(started, time.Since(started), err)
			}
			if err != nil {
				slog.Warn("workorch stale dispatched run recovery failed", "err", err)
			}
		}},
		runWatchdogInterval,
		0,
	)
	if err != nil {
		s.clearPeriodicRecoveryTimerStarted()
		slog.Warn("workorch stale dispatched run recovery timer failed", "err", err)
		return
	}
	if state.schedulerDiagnostics != nil {
		state.schedulerDiagnostics.recordWatchdogTimerRegistration()
	}
}

func (s *runContinuationScheduler) recoverStuckRunningRuns(ctx context.Context) error {
	if err := s.recoverStuckRunningRunsOnly(ctx); err != nil {
		return err
	}
	if err := s.recoverResolvedGovernanceBlockedRuns(ctx); err != nil {
		slog.Warn("workorch resolved governance blocked run recovery failed", "err", err)
	}
	s.recoverReplanContinuations(ctx)
	return nil
}

func (s *runContinuationScheduler) recoverStartupInactiveRunningWorkspaces(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	state := s.state
	ctx = state.eventContext(ctx)
	var recovered []appservice.Run
	var err error
	if state.hasSharedSession() {
		err = state.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var recoverErr error
			recovered, recoverErr = appservice.RecoverStartupInactiveRunningWorkspacesWithSession(ctx, session)
			if recoverErr != nil {
				return fmt.Errorf("recover startup inactive workspaces with shared session: %w", recoverErr)
			}
			return nil
		})
	} else {
		recovered, err = appservice.RecoverStartupInactiveRunningWorkspacesWithBackend(ctx, state.config.Backend, state.config.DataRoot)
		if err != nil {
			err = fmt.Errorf("recover startup inactive workspaces with backend: %w", err)
		}
	}
	if err != nil {
		return err
	}
	if len(recovered) > 0 {
		runIDs := make([]string, 0, len(recovered))
		for _, item := range recovered {
			runIDs = append(runIDs, item.ID)
			if state.schedulerDiagnostics != nil {
				state.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
					WorkspaceID:    item.WorkspaceID,
					RunID:          item.ID,
					Kind:           schedulerKindRunContinuation,
					State:          schedulerDiagnosticStateNotScheduled,
					Reason:         schedulerReasonServiceRestartedPendingWorkNotRecovered,
					Detail:         "previous process-local execution is not recoverable after service restart; submit /run resume or an equivalent explicit resume command",
					Recoverability: schedulerRecoverabilityManualRetry,
					RecommendedActions: []string{
						"run_resume",
						"workspace_restart",
					},
				})
			}
		}
		slog.Warn("workorch recovered startup inactive running workspaces", "runIDs", runIDs)
	}
	return nil
}

func (s *runContinuationScheduler) recoverStuckRunningRunsOnly(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	state := s.state
	ctx = state.eventContext(ctx)
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
	ctx = state.eventContext(ctx)
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
	if state.schedulerDiagnostics != nil {
		state.schedulerDiagnostics.recordStaleRuntimeGapRecovery("stale_dispatched", "runtime_recovery_policy", len(recovered))
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
	return governanceRunResumeRef{WorkspaceID: workspaceID, RunID: runID, Intent: "resume", Reason: "governance_recovery"}, true
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

func (s *runContinuationScheduler) recordContinuation(input schedulerDiagnosticInput) {
	if s == nil || s.state == nil || s.state.schedulerDiagnostics == nil {
		return
	}
	s.state.schedulerDiagnostics.recordContinuation(input)
}

func (s *runContinuationScheduler) recordContinuationFailed(workspaceID, runID, kind, reason string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	s.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: workspaceID,
		RunID:       runID,
		Kind:        kind,
		State:       schedulerDiagnosticStateFailed,
		Reason:      reason,
		Detail:      detail,
		DedupeKey:   schedulerDedupeKey(workspaceID, runID, kind, reason),
	})
}

func (s *runContinuationScheduler) recordContinuationNoAction(workspaceID, runID, kind, reason string, detail string) {
	s.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		Kind:           kind,
		State:          schedulerDiagnosticStateNoActionNeeded,
		Reason:         reason,
		Detail:         detail,
		Recoverability: schedulerRecoverabilityNoAction,
		DedupeKey:      schedulerDedupeKey(workspaceID, runID, kind, reason),
	})
}

func (s *runContinuationScheduler) recordContinuationCompleted(workspaceID, runID, kind string) {
	s.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		Kind:           kind,
		State:          schedulerDiagnosticStateCompleted,
		Recoverability: schedulerRecoverabilityNoAction,
		DedupeKey:      schedulerDedupeKey(workspaceID, runID, kind, schedulerDiagnosticStateCompleted),
	})
}

func (s *runContinuationScheduler) recordSchedulerEnqueueFailure(workspaceID, runID, kind, detail string) {
	if s == nil || s.state == nil || s.state.schedulerDiagnostics == nil {
		return
	}
	s.state.schedulerDiagnostics.recordCommandEnqueueFailure(kind, schedulerReasonEnqueueFailed)
	s.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		Kind:           kind,
		State:          schedulerDiagnosticStateFailed,
		Reason:         schedulerReasonEnqueueFailed,
		Detail:         detail,
		Recoverability: schedulerRecoverabilityManualRetry,
		DedupeKey:      schedulerDedupeKey(workspaceID, runID, kind, schedulerReasonEnqueueFailed),
	})
}

func schedulerRunSlotFailureReason(ctx context.Context, err error) string {
	if err == nil {
		return schedulerReasonRunSlotUnavailable
	}
	if ctx != nil && ctx.Err() != nil {
		return schedulerReasonServiceShuttingDown
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return schedulerReasonServiceShuttingDown
	}
	return schedulerReasonRunSlotUnavailable
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

func (s *runContinuationScheduler) scheduleGovernanceRunResumeRefs(refs []governanceRunResumeRef, priority int) bool {
	if s == nil || s.state == nil {
		return false
	}
	refs = uniqueGovernanceRunResumeRefs(refs)
	if len(refs) == 0 {
		return true
	}
	s.enqueueGovernanceResumeRefs(refs)
	return s.drainGovernanceResumeQueue(priority)
}

func (s *runContinuationScheduler) enqueueGovernanceResumeRefs(refs []governanceRunResumeRef) {
	if s == nil {
		return
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	if s.governancePending == nil {
		s.governancePending = map[string]governanceRunResumeRef{}
	}
	for _, ref := range refs {
		key := governanceRunResumeKey(ref)
		if key == "" {
			continue
		}
		s.governancePending[key] = ref
	}
}

func (s *runContinuationScheduler) drainGovernanceResumeQueue(priority int) bool {
	if s == nil || s.state == nil {
		return false
	}
	state := s.state
	for {
		key, ref, ok := s.nextPendingGovernanceResume()
		if !ok {
			return true
		}
		resumeKey, resumeRef := key, ref
		if !state.scheduleBackground("governance_run_resume", func(context.Context) {
			completePending := false
			defer func() {
				if completePending {
					s.clearGovernanceResumePending(resumeKey)
				}
				s.clearGovernanceResumeStarted(resumeKey)
			}()
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
				s.recordContinuationFailed(resumeRef.WorkspaceID, resumeRef.RunID, schedulerKindGovernanceContinuation, schedulerRunSlotFailureReason(ctx, err), err)
				slog.Warn("workorch governance resume queue failed", "workspaceID", resumeRef.WorkspaceID, "runID", resumeRef.RunID, "err", err)
				return
			}
			defer release()
			if resumeRef.WorkspaceID != "" {
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(resumeRef.WorkspaceID))
			}
			item, err := state.resumeRun(ctx, state.config.Backend, state.config.DataRoot, resumeRef.RunID)
			if err != nil {
				if !s.governanceResumeRefStillNeedsWork(ctx, resumeRef) {
					completePending = true
					s.recordContinuationNoAction(resumeRef.WorkspaceID, resumeRef.RunID, schedulerKindGovernanceContinuation, schedulerReasonRunNotEligible, "governance continuation no longer needs resume")
				} else {
					s.recordContinuationFailed(resumeRef.WorkspaceID, resumeRef.RunID, schedulerKindGovernanceContinuation, schedulerReasonOwnerCommandFailed, err)
				}
				slog.Warn("workorch governance resume failed", "workspaceID", resumeRef.WorkspaceID, "runID", resumeRef.RunID, "err", err)
				return
			}
			completePending = true
			state.metrics.observeRunOutcome(item)
			s.recordContinuationCompleted(resumeRef.WorkspaceID, item.ID, schedulerKindGovernanceContinuation)
			if item.State == string(common.RunStateSuperseded) {
				s.scheduleReplanContinuationIfNeeded(ctx, item.WorkspaceID, item.ID, priority)
			}
			resumedRun = item
		}) {
			s.clearGovernanceResumeStarted(resumeKey)
			s.recordSchedulerEnqueueFailure(resumeRef.WorkspaceID, resumeRef.RunID, schedulerKindGovernanceContinuation, "governance continuation work item was not queued")
			return false
		}
	}
}

func (s *runContinuationScheduler) nextPendingGovernanceResume() (string, governanceRunResumeRef, bool) {
	if s == nil {
		return "", governanceRunResumeRef{}, false
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	if s.governanceStarted == nil {
		s.governanceStarted = map[string]struct{}{}
	}
	for key, ref := range s.governancePending {
		if _, ok := s.governanceStarted[key]; ok {
			continue
		}
		s.governanceStarted[key] = struct{}{}
		return key, ref, true
	}
	return "", governanceRunResumeRef{}, false
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

func (s *runContinuationScheduler) clearGovernanceResumePending(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	delete(s.governancePending, key)
}

func (s *runContinuationScheduler) governanceResumeRefStillNeedsWork(ctx context.Context, ref governanceRunResumeRef) bool {
	if s == nil || s.state == nil || s.state.listRuns == nil {
		return true
	}
	workspaceID := strings.TrimSpace(ref.WorkspaceID)
	runID := strings.TrimSpace(ref.RunID)
	if workspaceID == "" || runID == "" {
		return false
	}
	runs, err := s.state.listRuns(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID)), s.state.config.Backend, s.state.config.DataRoot, workspaceID)
	if err != nil {
		slog.Warn("workorch governance resume pending run scan failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		return true
	}
	for _, run := range runs {
		if run.WorkspaceID == workspaceID && run.ID == runID {
			_, ok := s.resolvedGovernanceResumeRefForRun(ctx, workspaceID, run)
			return ok
		}
	}
	return false
}

func (s *runContinuationScheduler) scheduleReplanContinuationIfNeeded(ctx context.Context, workspaceID string, runID string, priority int) bool {
	if s == nil || s.state == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return false
	}
	state := s.state
	if !state.runHasResolvedReplanDecision(ctx, workspaceID, runID) {
		return true
	}
	if s.workspaceHasReplanSuccessor(ctx, workspaceID, runID) {
		s.recordContinuationNoAction(workspaceID, runID, schedulerKindReplanContinuation, schedulerReasonReplanSuccessorExists, "replan successor already exists")
		return true
	}
	key := workspaceID + "/" + runID
	if !s.markReplanContinuationStarted(key) {
		return true
	}

	if !state.scheduleBackground("replan_continuation", func(context.Context) {
		runCtx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
		release, err := state.acquireRunSlotWithPriority(runCtx, priority)
		if err != nil {
			s.clearReplanContinuationStarted(key)
			state.metrics.runCanceled.Add(1)
			s.recordContinuationFailed(workspaceID, runID, schedulerKindReplanContinuation, schedulerRunSlotFailureReason(runCtx, err), err)
			slog.Warn("workorch replan continuation queue failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			return
		}
		defer release()
		activation, err := state.resumeWorkspaceExecution(runCtx, state.config.Backend, state.config.DataRoot, workspaceID, appservice.ActivateWorkspaceExecutionCommand{Priority: priority})
		if err != nil {
			s.clearReplanContinuationStarted(key)
			s.recordContinuationFailed(workspaceID, runID, schedulerKindReplanContinuation, schedulerReasonOwnerCommandFailed, err)
			slog.Warn("workorch replan continuation activation failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			state.notifyRunStatusChanged()
			return
		}
		state.metrics.observeRunOutcome(activation.Run)
		s.recordContinuationCompleted(workspaceID, activation.Run.ID, schedulerKindReplanContinuation)
		state.notifyRunStatusChangedForRun(activation.Run)
	}) {
		s.clearReplanContinuationStarted(key)
		s.recordSchedulerEnqueueFailure(workspaceID, runID, schedulerKindReplanContinuation, "replan continuation work item was not queued")
		return false
	}
	return true
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

func (s *runContinuationScheduler) scheduleWorkspaceRestart(req workspaceRestartRequest) bool {
	req.Command.Restart = true
	return s.scheduleWorkspaceExecution("workspace_restart", req)
}

func (s *runContinuationScheduler) scheduleWorkspaceActivation(req workspaceRestartRequest) bool {
	req.Command.Restart = false
	return s.scheduleWorkspaceExecution("workspace_activation", req)
}

func (s *runContinuationScheduler) scheduleWorkspaceExecution(taskName string, req workspaceRestartRequest) bool {
	if s == nil || s.state == nil {
		return false
	}
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	if req.WorkspaceID == "" {
		return false
	}
	req.Command.Priority = req.Priority
	key := strings.TrimSpace(taskName) + "/" + req.WorkspaceID
	s.enqueueWorkspaceExecution(key, req)
	return s.drainWorkspaceExecutionQueue()
}

func (s *runContinuationScheduler) enqueueWorkspaceExecution(key string, req workspaceRestartRequest) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	if s.restartPending == nil {
		s.restartPending = map[string]workspaceRestartRequest{}
	}
	s.restartPending[key] = req
}

func (s *runContinuationScheduler) drainWorkspaceExecutionQueue() bool {
	if s == nil || s.state == nil {
		return false
	}
	state := s.state
	for {
		key, req, ok := s.nextPendingWorkspaceExecution()
		if !ok {
			return true
		}
		taskName := workspaceExecutionTaskNameFromKey(key)
		if taskName == "" {
			taskName = "workspace_execution"
		}
		kind := schedulerKindWorkspaceActivation
		if taskName == "workspace_restart" {
			kind = schedulerKindWorkspaceRestart
		}
		queued := state.scheduleBackground(taskName, func(context.Context) {
			defer s.clearWorkspaceRestartStarted(key)
			runCtx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
			runCtx = common.ContextWithRuntimeWorkspaceID(runCtx, common.WorkspaceID(req.WorkspaceID))
			release, err := state.acquireRunSlotWithPriority(runCtx, req.Priority)
			if err != nil {
				state.metrics.runCanceled.Add(1)
				s.recordContinuationFailed(req.WorkspaceID, "", kind, schedulerRunSlotFailureReason(runCtx, err), err)
				slog.Warn("workorch workspace execution queue failed", "task", taskName, "workspaceID", req.WorkspaceID, "err", err)
				return
			}
			defer release()
			activation, err := state.resumeWorkspaceExecution(runCtx, state.config.Backend, state.config.DataRoot, req.WorkspaceID, req.Command)
			if err != nil {
				slog.Warn("workorch workspace execution activation failed", "task", taskName, "workspaceID", req.WorkspaceID, "err", err)
				state.notifyRunStatusChanged()
				if !s.workspaceExecutionStillNeedsWork(runCtx, req) {
					s.recordContinuationNoAction(req.WorkspaceID, "", kind, schedulerReasonWorkspaceNotEligible, "workspace execution no longer needs activation")
					s.clearWorkspaceExecutionPending(key)
				} else {
					s.recordContinuationFailed(req.WorkspaceID, "", kind, schedulerReasonOwnerCommandFailed, err)
				}
				return
			}
			s.clearWorkspaceExecutionPending(key)
			state.metrics.observeRunOutcome(activation.Run)
			s.recordContinuationCompleted(req.WorkspaceID, activation.Run.ID, kind)
			if activation.Run.State == string(common.RunStateSuperseded) {
				s.scheduleReplanContinuationIfNeeded(runCtx, activation.Run.WorkspaceID, activation.Run.ID, req.Priority)
			}
			state.notifyRunStatusChangedForRun(activation.Run)
		})
		if !queued {
			s.clearWorkspaceRestartStarted(key)
			s.recordSchedulerEnqueueFailure(req.WorkspaceID, "", kind, taskName+" work item was not queued")
			return false
		}
	}
}

func workspaceExecutionTaskNameFromKey(key string) string {
	key = strings.TrimSpace(key)
	idx := strings.Index(key, "/")
	if idx <= 0 {
		return key
	}
	return key[:idx]
}

func (s *runContinuationScheduler) nextPendingWorkspaceExecution() (string, workspaceRestartRequest, bool) {
	if s == nil {
		return "", workspaceRestartRequest{}, false
	}
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	if s.restartStarted == nil {
		s.restartStarted = map[string]struct{}{}
	}
	for key, req := range s.restartPending {
		if _, ok := s.restartStarted[key]; ok {
			continue
		}
		s.restartStarted[key] = struct{}{}
		return key, req, true
	}
	return "", workspaceRestartRequest{}, false
}

func (s *runContinuationScheduler) clearWorkspaceRestartStarted(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	delete(s.restartStarted, key)
}

func (s *runContinuationScheduler) clearWorkspaceExecutionPending(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	delete(s.restartPending, key)
}

func (s *runContinuationScheduler) workspaceExecutionStillNeedsWork(ctx context.Context, req workspaceRestartRequest) bool {
	if s == nil || s.state == nil || s.state.readWorkspace == nil {
		return true
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		return false
	}
	workspace, err := s.state.readWorkspace(ctx, s.state.config.Backend, s.state.config.DataRoot, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return false
		}
		slog.Warn("workorch workspace execution pending workspace lookup failed", "workspaceID", workspaceID, "err", err)
		return true
	}
	if req.Command.Restart {
		return true
	}
	return strings.TrimSpace(workspace.ActiveRunID) == ""
}

func (s *runContinuationScheduler) scheduleRunContinuation(req runContinuationRequest) bool {
	if s == nil || s.state == nil {
		return false
	}
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.RunID = strings.TrimSpace(req.RunID)
	req.Action = strings.TrimSpace(req.Action)
	if req.WorkspaceID == "" || req.RunID == "" || req.Action == "" {
		return false
	}
	key := req.WorkspaceID + "/" + req.RunID + "/" + req.Action
	s.enqueueRunContinuation(key, req)
	return s.drainRunContinuationQueue()
}

func (s *runContinuationScheduler) enqueueRunContinuation(key string, req runContinuationRequest) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.runPending == nil {
		s.runPending = map[string]runContinuationRequest{}
	}
	s.runPending[key] = req
}

func (s *runContinuationScheduler) drainRunContinuationQueue() bool {
	if s == nil || s.state == nil {
		return false
	}
	state := s.state
	for {
		key, req, ok := s.nextPendingRunContinuation()
		if !ok {
			return true
		}
		queued := state.scheduleBackground("run_continuation", func(context.Context) {
			defer s.clearRunContinuationStarted(key)
			runCtx := appservice.ContextWithEventHub(state.backgroundContext(), state.eventHub)
			runCtx = common.ContextWithRuntimeWorkspaceID(runCtx, common.WorkspaceID(req.WorkspaceID))
			release, err := state.acquireRunSlotWithPriority(runCtx, req.Priority)
			if err != nil {
				state.metrics.runCanceled.Add(1)
				s.recordContinuationFailed(req.WorkspaceID, req.RunID, schedulerKindRunContinuation, schedulerRunSlotFailureReason(runCtx, err), err)
				slog.Warn("workorch run continuation queue failed", "workspaceID", req.WorkspaceID, "runID", req.RunID, "action", req.Action, "err", err)
				return
			}
			defer release()
			var item appservice.Run
			switch req.Action {
			case "start":
				state.metrics.runStartRequests.Add(1)
				item, err = state.startRun(runCtx, state.config.Backend, state.config.DataRoot, req.RunID)
			case "resume":
				state.metrics.runResumeRequests.Add(1)
				item, err = state.resumeRun(runCtx, state.config.Backend, state.config.DataRoot, req.RunID, req.ExecutionContextPatch)
			default:
				err = common.ErrInvalidArgument
			}
			if err != nil {
				slog.Warn("workorch run continuation failed", "workspaceID", req.WorkspaceID, "runID", req.RunID, "action", req.Action, "err", err)
				state.notifyRunStatusChanged()
				if !s.runContinuationStillNeedsWork(runCtx, req) {
					s.recordContinuationNoAction(req.WorkspaceID, req.RunID, schedulerKindRunContinuation, schedulerReasonRunNotEligible, "run continuation no longer needs work")
					s.clearRunContinuationPending(key)
				} else {
					s.recordContinuationFailed(req.WorkspaceID, req.RunID, schedulerKindRunContinuation, schedulerReasonOwnerCommandFailed, err)
				}
				return
			}
			s.clearRunContinuationPending(key)
			state.metrics.observeRunOutcome(item)
			s.recordContinuationCompleted(req.WorkspaceID, item.ID, schedulerKindRunContinuation)
			if item.State == string(common.RunStateSuperseded) {
				s.scheduleReplanContinuationIfNeeded(runCtx, item.WorkspaceID, item.ID, req.Priority)
			}
			state.notifyRunStatusChangedForRun(item)
		})
		if !queued {
			s.clearRunContinuationStarted(key)
			s.recordSchedulerEnqueueFailure(req.WorkspaceID, req.RunID, schedulerKindRunContinuation, "run continuation work item was not queued")
			return false
		}
	}
}

func (s *runContinuationScheduler) nextPendingRunContinuation() (string, runContinuationRequest, bool) {
	if s == nil {
		return "", runContinuationRequest{}, false
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.runStarted == nil {
		s.runStarted = map[string]struct{}{}
	}
	for key, req := range s.runPending {
		if _, ok := s.runStarted[key]; ok {
			continue
		}
		s.runStarted[key] = struct{}{}
		return key, req, true
	}
	return "", runContinuationRequest{}, false
}

func (s *runContinuationScheduler) clearRunContinuationStarted(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.runStarted, key)
}

func (s *runContinuationScheduler) clearRunContinuationPending(key string) {
	if s == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.runPending, key)
}

func (s *runContinuationScheduler) runContinuationStillNeedsWork(ctx context.Context, req runContinuationRequest) bool {
	if s == nil || s.state == nil || s.state.listRuns == nil {
		return true
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	runID := strings.TrimSpace(req.RunID)
	if workspaceID == "" || runID == "" {
		return false
	}
	runs, err := s.state.listRuns(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID)), s.state.config.Backend, s.state.config.DataRoot, workspaceID)
	if err != nil {
		slog.Warn("workorch run continuation pending run scan failed", "workspaceID", workspaceID, "runID", runID, "action", req.Action, "err", err)
		return true
	}
	for _, run := range runs {
		if run.WorkspaceID != workspaceID || run.ID != runID {
			continue
		}
		switch strings.TrimSpace(req.Action) {
		case "start":
			return common.RunState(run.State) == common.RunStateCreated
		case "resume":
			switch common.RunState(run.State) {
			case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
				return true
			default:
				return false
			}
		default:
			return false
		}
	}
	return false
}
