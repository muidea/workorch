package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (s *serverState) acquireRunSlot(ctx context.Context) (func(), error) {
	return s.acquireRunSlotWithPriority(ctx, 0)
}

func (s *serverState) readRunStatusWithWait(ctx context.Context, runID string, values url.Values) (appservice.RunStatus, bool, error) {
	waitState := strings.TrimSpace(values.Get("waitState"))
	timeout := durationFromMillisQuery(values, "waitTimeoutMillis", 0, 30*time.Second)
	interval := durationFromMillisQuery(values, "waitIntervalMillis", 50*time.Millisecond, time.Second)
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	if cached, ok := s.cachedRunSnapshot(ctx, runID); ok {
		status := appservice.RunStatusFromSnapshot(cached)
		if waitState == "" || status.State == waitState {
			return status, waitState == "" || status.State == waitState, nil
		}
	}
	if waitState == "" || timeout <= 0 {
		item, err := s.readRunStatus(ctx, s.config.Backend, s.config.DataRoot, runID)
		matched := waitState == "" || item.State == waitState
		if err == nil && matched {
			s.warmRunSnapshotCache(ctx, runID, item.State)
		}
		return item, matched, err
	}
	deadline := time.Now().Add(timeout)
	for {
		notify := s.runStatusWaitNotify()
		item, err := s.readRunStatus(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			return appservice.RunStatus{}, false, err
		}
		if item.State == waitState {
			s.warmRunSnapshotCache(ctx, runID, item.State)
			return item, true, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return item, false, nil
		}
		sleepFor := interval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return appservice.RunStatus{}, false, ctx.Err()
		case <-notify:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func requestRunWorkspaceContext(ctx context.Context, r *http.Request) context.Context {
	if r == nil {
		return ctx
	}
	workspaceID, _, _, ok := workspaceRunActionFromPath(r.URL.Path)
	if !ok {
		workspaceID = strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	}
	if workspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
}

func (s *serverState) warmRunSnapshotCache(ctx context.Context, runID string, state string) {
	if s == nil || strings.TrimSpace(runID) == "" || !terminalRunState(state) {
		return
	}
	if _, ok := s.cachedRunSnapshot(ctx, runID); ok {
		return
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	if _, ok := s.ensureRunSnapshotInflight(ctx, runID); ok {
		return
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if !s.scheduleBackground("run_snapshot_cache_warm", func(context.Context) {
		warmCtx, cancel := context.WithTimeout(s.backgroundContext(), runSnapshotCacheTTL)
		defer cancel()
		if workspaceID != "" {
			warmCtx = common.ContextWithRuntimeWorkspaceID(warmCtx, workspaceID)
		}
		defer s.finishRunSnapshotInflight(warmCtx, runID)
		item, err := s.readRunSnapshot(warmCtx, s.config.Backend, s.config.DataRoot, runID)
		s.completeRunSnapshotInflight(warmCtx, runID, item, err)
	}) {
		s.completeRunSnapshotInflight(ctx, runID, appservice.RunSnapshot{}, common.ErrConflict)
		s.finishRunSnapshotInflight(ctx, runID)
	}
}

func (s *serverState) readRunSnapshotCached(ctx context.Context, runID string) (appservice.RunSnapshot, error) {
	if cached, ok := s.cachedRunSnapshot(ctx, runID); ok {
		return cached, nil
	}
	if item, err, ok := s.waitRunSnapshotInflight(ctx, runID); ok {
		return item, err
	}
	item, err := s.readRunSnapshot(ctx, s.config.Backend, s.config.DataRoot, runID)
	if err != nil {
		return appservice.RunSnapshot{}, err
	}
	s.cacheRunSnapshot(item)
	return item, nil
}

func (s *serverState) readWorkspaceSnapshotCached(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if cached, ok := s.cachedRunSnapshotByWorkspace(workspaceID); ok && s.sharedSession != nil {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx, session, workspaceID, cached)
			return err
		})
		return ret, err
	}
	return s.readWorkspaceSnapshot(ctx, s.config.Backend, s.config.DataRoot, workspaceID)
}

func (s *serverState) readArtifactContentCached(ctx context.Context, artifactURI string) (appservice.ArtifactContent, error) {
	if cached, ok := s.cachedRunSnapshotByArtifactURI(artifactURI); ok && s.sharedSession != nil {
		var ret appservice.ArtifactContent
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadArtifactContentWithRunSnapshotWithSession(ctx, session, artifactURI, cached)
			return err
		})
		return ret, err
	}
	return s.readArtifactContent(ctx, s.config.Backend, s.config.DataRoot, artifactURI)
}

func (s *serverState) readRunObservabilityCached(ctx context.Context, runID string) (appservice.RunObservability, error) {
	if cached, ok := s.cachedRunSnapshot(ctx, runID); ok {
		return appservice.RunObservabilityFromSnapshot(cached), nil
	}
	if item, err, ok := s.waitRunSnapshotInflight(ctx, runID); ok {
		if err != nil {
			return appservice.RunObservability{}, err
		}
		return appservice.RunObservabilityFromSnapshot(item), nil
	}
	item, err := s.readRunObservability(ctx, s.config.Backend, s.config.DataRoot, runID)
	if err != nil {
		return appservice.RunObservability{}, err
	}
	return item, nil
}

func (s *serverState) cachedRunSnapshot(ctx context.Context, runID string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	key := runSnapshotCacheKey(ctx, runID)
	entry, ok := s.runSnapshotCache[key]
	if !ok || time.Since(entry.cachedAt) > runSnapshotCacheTTL {
		if ok {
			delete(s.runSnapshotCache, key)
		}
		if common.RuntimeWorkspaceIDFromContext(ctx) != "" {
			return appservice.RunSnapshot{}, false
		}
		return s.cachedRunSnapshotByBareRunIDLocked(runID)
	}
	return entry.snapshot, true
}

func (s *serverState) cachedRunSnapshotByBareRunIDLocked(runID string) (appservice.RunSnapshot, bool) {
	now := time.Now()
	var ret appservice.RunSnapshot
	count := 0
	for key, entry := range s.runSnapshotCache {
		if now.Sub(entry.cachedAt) > runSnapshotCacheTTL {
			delete(s.runSnapshotCache, key)
			continue
		}
		if entry.snapshot.Run.ID == runID {
			ret = entry.snapshot
			count++
		}
	}
	return ret, count == 1
}

func (s *serverState) cachedRunSnapshotByWorkspace(workspaceID string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	now := time.Now()
	for runID, entry := range s.runSnapshotCache {
		if now.Sub(entry.cachedAt) > runSnapshotCacheTTL {
			delete(s.runSnapshotCache, runID)
			continue
		}
		if entry.snapshot.Run.WorkspaceID == workspaceID {
			return entry.snapshot, true
		}
	}
	return appservice.RunSnapshot{}, false
}

func (s *serverState) ensureRunSnapshotInflight(ctx context.Context, runID string) (*runSnapshotInflight, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return nil, true
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	if s.runSnapshotInflight == nil {
		s.runSnapshotInflight = map[string]*runSnapshotInflight{}
	}
	key := runSnapshotCacheKey(ctx, runID)
	if current, ok := s.runSnapshotInflight[key]; ok {
		return current, true
	}
	current := &runSnapshotInflight{done: make(chan struct{})}
	s.runSnapshotInflight[key] = current
	return current, false
}

func (s *serverState) waitRunSnapshotInflight(ctx context.Context, runID string) (appservice.RunSnapshot, error, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return appservice.RunSnapshot{}, nil, false
	}
	s.runSnapshotCacheMu.Lock()
	key := runSnapshotCacheKey(ctx, runID)
	current, ok := s.runSnapshotInflight[key]
	s.runSnapshotCacheMu.Unlock()
	if !ok {
		return appservice.RunSnapshot{}, nil, false
	}
	select {
	case <-ctx.Done():
		return appservice.RunSnapshot{}, ctx.Err(), true
	case <-current.done:
		return current.snapshot, current.err, true
	}
}

func (s *serverState) completeRunSnapshotInflight(ctx context.Context, runID string, snapshot appservice.RunSnapshot, err error) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	key := runSnapshotCacheKey(ctx, runID)
	current, ok := s.runSnapshotInflight[key]
	if !ok {
		return
	}
	current.snapshot = snapshot
	current.err = err
	if err == nil && snapshot.Run.ID != "" && terminalRunState(snapshot.Run.State) {
		if s.runSnapshotCache == nil {
			s.runSnapshotCache = map[string]cachedRunSnapshot{}
		}
		s.runSnapshotCache[runSnapshotCacheKeyForSnapshot(snapshot)] = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
	}
	close(current.done)
}

func (s *serverState) finishRunSnapshotInflight(ctx context.Context, runID string) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	delete(s.runSnapshotInflight, runSnapshotCacheKey(ctx, runID))
}

func (s *serverState) cachedRunSnapshotByArtifactURI(artifactURI string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(artifactURI) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	now := time.Now()
	for runID, entry := range s.runSnapshotCache {
		if now.Sub(entry.cachedAt) > runSnapshotCacheTTL {
			delete(s.runSnapshotCache, runID)
			continue
		}
		for _, artifact := range entry.snapshot.Artifacts {
			if strings.TrimSpace(artifact.URI) == artifactURI {
				return entry.snapshot, true
			}
		}
	}
	return appservice.RunSnapshot{}, false
}

func (s *serverState) cacheRunSnapshot(snapshot appservice.RunSnapshot) {
	if s == nil || snapshot.Run.ID == "" || !terminalRunState(snapshot.Run.State) {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	if s.runSnapshotCache == nil {
		s.runSnapshotCache = map[string]cachedRunSnapshot{}
	}
	s.runSnapshotCache[runSnapshotCacheKeyForSnapshot(snapshot)] = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
}

func runSnapshotCacheKey(ctx context.Context, runID string) string {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return "/" + strings.TrimSpace(runID)
	}
	return string(workspaceID) + "/" + strings.TrimSpace(runID)
}

func runSnapshotCacheKeyForSnapshot(snapshot appservice.RunSnapshot) string {
	if snapshot.Run.WorkspaceID == "" {
		return "/" + snapshot.Run.ID
	}
	return snapshot.Run.WorkspaceID + "/" + snapshot.Run.ID
}

func (s *serverState) clearRunSnapshotCache() {
	if s == nil {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	clear(s.runSnapshotCache)
}

func (s *serverState) runStatusWaitNotify() <-chan struct{} {
	if s == nil {
		return closedNotifyChannel()
	}
	s.runStatusNotifyMu.Lock()
	defer s.runStatusNotifyMu.Unlock()
	if s.runStatusNotify == nil {
		s.runStatusNotify = make(chan struct{})
	}
	return s.runStatusNotify
}

func (s *serverState) notifyRunStatusChanged() {
	if s == nil {
		return
	}
	s.clearRunSnapshotCache()
	s.notifyRunStatusWaiters()
}

func (s *serverState) notifyRunStatusChangedForRun(run appservice.Run) {
	if s == nil {
		return
	}
	s.clearRunSnapshotCache()
	s.warmRunSnapshotCache(context.Background(), run.ID, run.State)
	s.notifyRunStatusWaiters()
}

func (s *serverState) notifyRunStatusWaiters() {
	if s == nil {
		return
	}
	s.runStatusNotifyMu.Lock()
	defer s.runStatusNotifyMu.Unlock()
	if s.runStatusNotify == nil {
		s.runStatusNotify = make(chan struct{})
		return
	}
	close(s.runStatusNotify)
	s.runStatusNotify = make(chan struct{})
}

func closedNotifyChannel() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func durationFromMillisQuery(values url.Values, key string, fallback time.Duration, maxValue time.Duration) time.Duration {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || millis < 0 {
		return fallback
	}
	ret := time.Duration(millis) * time.Millisecond
	if maxValue > 0 && ret > maxValue {
		return maxValue
	}
	return ret
}

type governanceRunResumeRef struct {
	WorkspaceID string
	RunID       string
}

func (s *serverState) scheduleGovernanceRunResumes(runIDs []string, priority int) {
	if s == nil || s.runContinuations == nil {
		return
	}
	s.runContinuations.scheduleGovernanceRunResumes(runIDs, priority)
}

func (s *serverState) scheduleGovernanceRunResumeRefs(refs []governanceRunResumeRef, priority int) {
	if s == nil || s.runContinuations == nil {
		return
	}
	s.runContinuations.scheduleGovernanceRunResumeRefs(refs, priority)
}

func (s *serverState) scheduleReplanContinuationIfNeeded(ctx context.Context, workspaceID string, runID string, priority int) {
	if s == nil || s.runContinuations == nil {
		return
	}
	s.runContinuations.scheduleReplanContinuationIfNeeded(ctx, workspaceID, runID, priority)
}

func runIsReplanSuccessorOf(item appservice.Run, predecessorRunID string) bool {
	predecessorRunID = strings.TrimSpace(predecessorRunID)
	if predecessorRunID == "" {
		return false
	}
	if replan, _ := item.ExecutionContext["replanContinuation"].(map[string]any); len(replan) > 0 {
		if predecessor, _ := replan["predecessorRunId"].(string); strings.TrimSpace(predecessor) == predecessorRunID {
			return true
		}
	}
	return strings.HasPrefix(item.ID, predecessorRunID+"-replan-")
}

func (s *serverState) runHasResolvedReplanDecision(ctx context.Context, workspaceID string, runID string) bool {
	if s == nil || s.listDecisions == nil {
		return false
	}
	items, err := s.listDecisions(ctx, s.config.Backend, s.config.DataRoot, workspaceID, runID)
	if err != nil {
		slog.Warn("workorch replan continuation decision lookup failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		return false
	}
	for _, item := range items {
		if item.RunID == runID &&
			item.State == string(common.DecisionTicketStateResolved) &&
			item.ResolutionType == decisioncommon.ResolutionTypeReplan {
			return true
		}
	}
	return false
}

func runIDsFromDecisions(items []appservice.Decision) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.RunID)
	}
	return ret
}

func runIDsFromApprovals(items []appservice.Approval) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.RunID)
	}
	return ret
}

func runResumeRefsFromDecisions(items []appservice.Decision) []governanceRunResumeRef {
	ret := make([]governanceRunResumeRef, 0, len(items))
	for _, item := range items {
		ret = append(ret, governanceRunResumeRef{
			WorkspaceID: item.WorkspaceID,
			RunID:       item.RunID,
		})
	}
	return ret
}

func runResumeRefsFromApprovals(items []appservice.Approval) []governanceRunResumeRef {
	ret := make([]governanceRunResumeRef, 0, len(items))
	for _, item := range items {
		ret = append(ret, governanceRunResumeRef{
			WorkspaceID: item.WorkspaceID,
			RunID:       item.RunID,
		})
	}
	return ret
}

func uniqueGovernanceRunResumeRefs(items []governanceRunResumeRef) []governanceRunResumeRef {
	seen := map[string]struct{}{}
	ret := make([]governanceRunResumeRef, 0, len(items))
	for _, item := range items {
		item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
		item.RunID = strings.TrimSpace(item.RunID)
		if item.RunID == "" {
			continue
		}
		key := governanceRunResumeKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func governanceRunResumeKey(item governanceRunResumeRef) string {
	if item.WorkspaceID == "" {
		return "/" + item.RunID
	}
	return item.WorkspaceID + "/" + item.RunID
}

func uniqueNonEmptyStrings(items []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func (s *serverState) acquireRunSlotWithPriority(ctx context.Context, priority int) (func(), error) {
	s.runQueueMu.Lock()
	if s.activeRunSlots < s.config.MaxConcurrentRuns && len(s.runQueueWaiters) == 0 {
		s.activeRunSlots++
		s.metrics.runInFlight.Add(1)
		s.runQueueMu.Unlock()
		return s.releaseRunSlot, nil
	}

	s.runQueueSequence++
	waiter := &runQueueWaiter{
		ready:    make(chan struct{}),
		priority: priority,
		sequence: s.runQueueSequence,
	}
	s.runQueueWaiters = append(s.runQueueWaiters, waiter)
	s.metrics.runQueueWaiting.Add(1)
	s.metrics.runQueuedTotal.Add(1)
	s.runQueueMu.Unlock()

	select {
	case <-waiter.ready:
		if ctx.Err() != nil {
			s.releaseRunSlot()
			return nil, ctx.Err()
		}
		return s.releaseRunSlot, nil
	case <-ctx.Done():
		if s.cancelQueuedRunWaiter(waiter.ready) {
			return nil, ctx.Err()
		}
		s.releaseRunSlot()
		return nil, ctx.Err()
	}
}

func (s *serverState) releaseRunSlot() {
	s.runQueueMu.Lock()
	if len(s.runQueueWaiters) > 0 {
		idx := s.nextRunQueueWaiterIndex()
		waiter := s.runQueueWaiters[idx]
		s.runQueueWaiters = append(s.runQueueWaiters[:idx], s.runQueueWaiters[idx+1:]...)
		s.metrics.runQueueWaiting.Add(-1)
		s.runQueueMu.Unlock()
		close(waiter.ready)
		return
	}
	if s.activeRunSlots > 0 {
		s.activeRunSlots--
		s.metrics.runInFlight.Add(-1)
	}
	s.runQueueMu.Unlock()
}

func (s *serverState) cancelQueuedRunWaiter(target chan struct{}) bool {
	s.runQueueMu.Lock()
	defer s.runQueueMu.Unlock()

	for idx, waiter := range s.runQueueWaiters {
		if waiter.ready != target {
			continue
		}
		s.runQueueWaiters = append(s.runQueueWaiters[:idx], s.runQueueWaiters[idx+1:]...)
		s.metrics.runQueueWaiting.Add(-1)
		return true
	}
	return false
}

func (s *serverState) nextRunQueueWaiterIndex() int {
	best := 0
	for idx := 1; idx < len(s.runQueueWaiters); idx++ {
		if runQueueWaiterBefore(s.runQueueWaiters[idx], s.runQueueWaiters[best]) {
			best = idx
		}
	}
	return best
}

func runQueueWaiterBefore(left *runQueueWaiter, right *runQueueWaiter) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if left.priority != right.priority {
		return left.priority > right.priority
	}
	return left.sequence < right.sequence
}
