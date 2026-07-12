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
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
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
		cached = s.attachSchedulerDiagnosticsToRunSnapshot(ctx, cached)
		status := appservice.RunStatusFromSnapshot(cached)
		if waitState == "" || status.State == waitState {
			return status, waitState == "" || status.State == waitState, nil
		}
	}
	if waitState == "" || timeout <= 0 {
		item, err := s.readRunStatus(ctx, s.config.Backend, s.config.DataRoot, runID)
		item = s.attachSchedulerDiagnosticsToRunStatus(ctx, item)
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
		item = s.attachSchedulerDiagnosticsToRunStatus(ctx, item)
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
		return s.attachSchedulerDiagnosticsToRunSnapshot(ctx, cached), nil
	}
	if item, err, ok := s.waitRunSnapshotInflight(ctx, runID); ok {
		return s.attachSchedulerDiagnosticsToRunSnapshot(ctx, item), err
	}
	item, err := s.readRunSnapshot(ctx, s.config.Backend, s.config.DataRoot, runID)
	if err != nil {
		return appservice.RunSnapshot{}, err
	}
	s.cacheRunSnapshot(item)
	return s.attachSchedulerDiagnosticsToRunSnapshot(ctx, item), nil
}

func (s *serverState) readWorkspaceSnapshotCached(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if cached, ok := s.cachedRunSnapshotByWorkspace(workspaceID); ok && s.hasSharedSession() {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx, session, workspaceID, cached)
			return err
		})
		return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(ret), err
	}
	ret, err := s.readWorkspaceSnapshot(ctx, s.config.Backend, s.config.DataRoot, workspaceID)
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(ret), err
}

func (s *serverState) readWorkspaceDetailSnapshotCached(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if cached, ok := s.cachedRunSnapshotByWorkspace(workspaceID); ok && s.hasSharedSession() {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceDetailSnapshotWithRunSnapshotWithSession(ctx, session, workspaceID, cached)
			return err
		})
		return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(ret), err
	}
	ret, err := s.readWorkspaceDetailSnapshot(ctx, s.config.Backend, s.config.DataRoot, workspaceID)
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(ret), err
}

func (s *serverState) readArtifactContentCached(ctx context.Context, artifactURI string) (appservice.ArtifactContent, error) {
	if cached, ok := s.cachedRunSnapshotByArtifactURI(artifactURI); ok && s.hasSharedSession() {
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

func (s *serverState) schedulerDiagnosticsFor(workspaceID string, runID string) *appservice.SchedulerDiagnostics {
	if s == nil || s.schedulerDiagnostics == nil {
		return nil
	}
	return s.schedulerDiagnostics.snapshot(workspaceID, runID)
}

func (s *serverState) attachSchedulerDiagnosticsToRunStatus(ctx context.Context, item appservice.RunStatus) appservice.RunStatus {
	workspaceID := strings.TrimSpace(item.WorkspaceID)
	if workspaceID == "" {
		workspaceID = string(common.RuntimeWorkspaceIDFromContext(ctx))
	}
	item.SchedulerDiagnostics = s.schedulerDiagnosticsFor(workspaceID, item.ID)
	return item
}

func (s *serverState) attachSchedulerDiagnosticsToRunSnapshot(ctx context.Context, item appservice.RunSnapshot) appservice.RunSnapshot {
	workspaceID := strings.TrimSpace(item.Run.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(item.Workspace.ID)
	}
	if workspaceID == "" {
		workspaceID = string(common.RuntimeWorkspaceIDFromContext(ctx))
	}
	item.SchedulerDiagnostics = s.schedulerDiagnosticsFor(workspaceID, item.Run.ID)
	return item
}

func (s *serverState) attachSchedulerDiagnosticsToWorkspaceSnapshot(item appservice.WorkspaceSnapshot) appservice.WorkspaceSnapshot {
	workspaceID := strings.TrimSpace(item.Workspace.ID)
	var runID string
	if item.Run != nil {
		runID = strings.TrimSpace(item.Run.ID)
	}
	item.SchedulerDiagnostics = s.schedulerDiagnosticsFor(workspaceID, runID)
	return item
}

func (s *serverState) readRunObservabilityCached(ctx context.Context, runID string) (appservice.RunObservability, error) {
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
	entry := s.runSnapshotEntries[key]
	cached, ok := freshRunSnapshotCachedEntry(entry, time.Now())
	if !ok {
		s.clearRunSnapshotCachedEntryLocked(key, entry)
		if common.RuntimeWorkspaceIDFromContext(ctx) != "" {
			return appservice.RunSnapshot{}, false
		}
		return s.cachedRunSnapshotByBareRunIDLocked(runID)
	}
	return cached.snapshot, true
}

func (s *serverState) cachedRunSnapshotByBareRunIDLocked(runID string) (appservice.RunSnapshot, bool) {
	now := time.Now()
	var ret appservice.RunSnapshot
	count := 0
	for key, entry := range s.runSnapshotEntries {
		cached, ok := freshRunSnapshotCachedEntry(entry, now)
		if !ok {
			s.clearRunSnapshotCachedEntryLocked(key, entry)
			continue
		}
		if cached.snapshot.Run.ID == runID {
			ret = cached.snapshot
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
	var ret appservice.RunSnapshot
	count := 0
	for key, entry := range s.runSnapshotEntries {
		cached, ok := freshRunSnapshotCachedEntry(entry, now)
		if !ok {
			s.clearRunSnapshotCachedEntryLocked(key, entry)
			continue
		}
		if cached.snapshot.Run.WorkspaceID == workspaceID {
			ret = cached.snapshot
			count++
		}
	}
	return ret, count == 1
}

func (s *serverState) ensureRunSnapshotInflight(ctx context.Context, runID string) (*runSnapshotInflight, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return nil, true
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	key := runSnapshotCacheKey(ctx, runID)
	entry := s.ensureRunSnapshotEntryLocked(key)
	if entry.inflight != nil {
		return entry.inflight, true
	}
	current := &runSnapshotInflight{done: make(chan struct{})}
	entry.inflight = current
	return current, false
}

func (s *serverState) waitRunSnapshotInflight(ctx context.Context, runID string) (appservice.RunSnapshot, error, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return appservice.RunSnapshot{}, nil, false
	}
	s.runSnapshotCacheMu.Lock()
	key := runSnapshotCacheKey(ctx, runID)
	entry := s.runSnapshotEntries[key]
	var current *runSnapshotInflight
	if entry != nil {
		current = entry.inflight
	}
	s.runSnapshotCacheMu.Unlock()
	if current == nil {
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
	entry := s.runSnapshotEntries[key]
	if entry == nil || entry.inflight == nil {
		return
	}
	current := entry.inflight
	current.snapshot = snapshot
	current.err = err
	if err == nil && snapshot.Run.ID != "" && terminalRunState(snapshot.Run.State) {
		cacheKey := runSnapshotCacheKeyForSnapshot(snapshot)
		cacheEntry := s.ensureRunSnapshotEntryLocked(cacheKey)
		cacheEntry.cached = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
		cacheEntry.hasCache = true
	}
	close(current.done)
}

func (s *serverState) finishRunSnapshotInflight(ctx context.Context, runID string) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	key := runSnapshotCacheKey(ctx, runID)
	entry := s.runSnapshotEntries[key]
	if entry == nil {
		return
	}
	entry.inflight = nil
	if !entry.hasCache {
		delete(s.runSnapshotEntries, key)
	}
}

func (s *serverState) cachedRunSnapshotByArtifactURI(artifactURI string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(artifactURI) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	now := time.Now()
	for key, entry := range s.runSnapshotEntries {
		cached, ok := freshRunSnapshotCachedEntry(entry, now)
		if !ok {
			s.clearRunSnapshotCachedEntryLocked(key, entry)
			continue
		}
		for _, artifact := range cached.snapshot.Artifacts {
			if strings.TrimSpace(artifact.URI) == artifactURI {
				return cached.snapshot, true
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
	entry := s.ensureRunSnapshotEntryLocked(runSnapshotCacheKeyForSnapshot(snapshot))
	entry.cached = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
	entry.hasCache = true
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
	for key, entry := range s.runSnapshotEntries {
		if entry == nil {
			delete(s.runSnapshotEntries, key)
			continue
		}
		entry.cached = cachedRunSnapshot{}
		entry.hasCache = false
		if entry.inflight == nil {
			delete(s.runSnapshotEntries, key)
		}
	}
}

func (s *serverState) ensureRunSnapshotEntryLocked(key string) *runSnapshotCacheEntry {
	if s.runSnapshotEntries == nil {
		s.runSnapshotEntries = map[string]*runSnapshotCacheEntry{}
	}
	entry := s.runSnapshotEntries[key]
	if entry == nil {
		entry = &runSnapshotCacheEntry{}
		s.runSnapshotEntries[key] = entry
	}
	return entry
}

func (s *serverState) clearRunSnapshotCachedEntryLocked(key string, entry *runSnapshotCacheEntry) {
	if entry == nil {
		return
	}
	entry.cached = cachedRunSnapshot{}
	entry.hasCache = false
	if entry.inflight == nil {
		delete(s.runSnapshotEntries, key)
	}
}

func freshRunSnapshotCachedEntry(entry *runSnapshotCacheEntry, now time.Time) (cachedRunSnapshot, bool) {
	if entry == nil || !entry.hasCache {
		return cachedRunSnapshot{}, false
	}
	if now.Sub(entry.cached.cachedAt) > runSnapshotCacheTTL {
		return cachedRunSnapshot{}, false
	}
	return entry.cached, true
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
	WorkspaceID         string
	RunID               string
	Intent              string
	Reason              string
	RequestID           string
	DecisionID          string
	ResolutionID        string
	AuthoritySnapshotID string
}

func (s *serverState) scheduleGovernanceRunResumeRefs(refs []governanceRunResumeRef, priority int) bool {
	if s == nil || s.runContinuations == nil {
		return false
	}
	return s.runContinuations.scheduleGovernanceRunResumeRefs(refs, priority)
}

func (s *serverState) scheduleReplanContinuationIfNeeded(ctx context.Context, workspaceID string, runID string, priority int) bool {
	if s == nil || s.runContinuations == nil {
		return false
	}
	return s.runContinuations.scheduleReplanContinuationIfNeeded(ctx, workspaceID, runID, priority)
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
	return false
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
		workspaceID := strings.TrimSpace(item.WorkspaceID)
		runID := strings.TrimSpace(item.RunID)
		decisionID := strings.TrimSpace(item.ID)
		if workspaceID == "" || runID == "" || decisionID == "" {
			continue
		}
		intent := "resume"
		reason := "decision_resolved"
		if strings.TrimSpace(item.ResolutionType) == decisioncommon.ResolutionTypeReplan {
			intent = "replan"
			reason = "decision_replan_resolved"
		}
		ret = append(ret, governanceRunResumeRef{
			WorkspaceID: workspaceID,
			RunID:       runID,
			Intent:      intent,
			Reason:      reason,
			RequestID:   governanceRunResumeRequestID("decision", decisionID, intent),
			DecisionID:  decisionID,
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
			Intent:      "resume",
			Reason:      "approval_resolved",
			RequestID:   governanceRunResumeRequestID("approval", item.ID, "resume"),
		})
	}
	return ret
}

func governanceRunResumeRequestID(kind string, id string, intent string) string {
	parts := []string{"governance", strings.TrimSpace(kind), strings.TrimSpace(id), strings.TrimSpace(intent)}
	for idx, part := range parts {
		if part == "" {
			parts[idx] = "none"
		}
	}
	return strings.Join(parts, "-")
}

func uniqueGovernanceRunResumeRefs(items []governanceRunResumeRef) []governanceRunResumeRef {
	seen := map[string]struct{}{}
	ret := make([]governanceRunResumeRef, 0, len(items))
	for _, item := range items {
		item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
		item.RunID = strings.TrimSpace(item.RunID)
		item.Intent = strings.TrimSpace(item.Intent)
		item.Reason = strings.TrimSpace(item.Reason)
		item.RequestID = strings.TrimSpace(item.RequestID)
		item.DecisionID = strings.TrimSpace(item.DecisionID)
		item.ResolutionID = strings.TrimSpace(item.ResolutionID)
		item.AuthoritySnapshotID = strings.TrimSpace(item.AuthoritySnapshotID)
		if item.Intent == "" {
			item.Intent = "resume"
		}
		if item.WorkspaceID == "" || item.RunID == "" {
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
	intent := strings.TrimSpace(item.Intent)
	if intent == "" {
		intent = "resume"
	}
	workspaceID := strings.TrimSpace(item.WorkspaceID)
	runID := strings.TrimSpace(item.RunID)
	if workspaceID == "" || runID == "" {
		return ""
	}
	decisionID := strings.TrimSpace(item.DecisionID)
	resolutionID := strings.TrimSpace(item.ResolutionID)
	authoritySnapshotID := strings.TrimSpace(item.AuthoritySnapshotID)
	if intent == "resume" && (decisionID != "" || resolutionID != "" || authoritySnapshotID != "") {
		return strings.Join([]string{intent, workspaceID, runID, firstNonEmptyServerValue(decisionID, "decision-unknown"), firstNonEmptyServerValue(resolutionID, "resolution-unknown"), firstNonEmptyServerValue(authoritySnapshotID, "authority-unknown")}, "/")
	}
	if decisionID != "" {
		return intent + "/" + workspaceID + "/" + runID + "/" + decisionID
	}
	return intent + "/" + workspaceID + "/" + runID
}

func firstNonEmptyServerValue(value string, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
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
