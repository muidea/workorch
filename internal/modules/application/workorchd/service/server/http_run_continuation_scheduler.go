package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runContinuationScheduler struct {
	state *serverState

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
}

func (s *runContinuationScheduler) recoverStuckRunningRuns(ctx context.Context) error {
	if s == nil || s.state == nil {
		return nil
	}
	state := s.state
	var recovered []appservice.Run
	var err error
	if state.sharedSession != nil {
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
	s.recoverReplanContinuations(ctx)
	return nil
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
			strings.TrimSpace(workspace.ActiveRunID) != "" ||
			workspace.State != string(common.WorkspaceStateFailed) {
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
			s.scheduleReplanContinuationIfNeeded(ctx, workspace.ID, run.ID, run.Priority)
		}
	}
}

func (s *runContinuationScheduler) scheduleGovernanceRunResumes(runIDs []string, priority int) {
	refs := make([]governanceRunResumeRef, 0, len(runIDs))
	for _, runID := range runIDs {
		refs = append(refs, governanceRunResumeRef{RunID: runID})
	}
	s.scheduleGovernanceRunResumeRefs(refs, priority)
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

func (s *runContinuationScheduler) markGovernanceResumeStarted(runID string) bool {
	if s == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	if s.governanceStarted == nil {
		s.governanceStarted = map[string]struct{}{}
	}
	if _, ok := s.governanceStarted[runID]; ok {
		return false
	}
	s.governanceStarted[runID] = struct{}{}
	return true
}

func (s *runContinuationScheduler) clearGovernanceResumeStarted(runID string) {
	if s == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	delete(s.governanceStarted, runID)
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
