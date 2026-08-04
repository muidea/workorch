package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

const (
	runWatchdogInterval       = time.Duration(0)
	schedulerDiagnosticSource = "workorchd/scheduler"
	schedulerDiagnosticLimit  = 64
	schedulerDetailLimit      = 512

	schedulerKindWorkspaceActivation = "workspace_activation"
	schedulerKindWorkspaceRun        = "workspace_run"
	schedulerKindWorkspaceRestart    = "workspace_restart"
	schedulerKindRunContinuation     = "run_continuation"

	schedulerDiagnosticStateNotScheduled   = "not_scheduled"
	schedulerDiagnosticStateAccepted       = "accepted"
	schedulerDiagnosticStateRunning        = "running"
	schedulerDiagnosticStateFailed         = "failed"
	schedulerDiagnosticStateCompleted      = "completed"
	schedulerDiagnosticStateNoActionNeeded = "no_action_needed"

	schedulerReasonEnqueueFailed                           = "scheduler_enqueue_failed"
	schedulerReasonBackgroundFailed                        = "background_execution_failed"
	schedulerReasonWorkspaceActivationFailed               = "workspace_activation_failed"
	schedulerReasonOwnerCommandFailed                      = "owner_command_failed"
	schedulerReasonRunSlotUnavailable                      = "run_slot_unavailable"
	schedulerReasonServiceShuttingDown                     = "service_shutting_down"
	schedulerReasonWorkspaceNotEligible                    = "workspace_not_eligible"
	schedulerReasonRunNotEligible                          = "run_not_eligible"
	schedulerReasonOwnerReadbackNotAdvanced                = "owner_readback_not_advanced"
	schedulerReasonServiceRestartedPendingWorkNotRecovered = "service_restarted_pending_scheduler_work_not_recovered"

	schedulerRecoverabilityManualRetry = "manual_retry_required"
	schedulerRecoverabilityNoAction    = "no_action_needed"
	schedulerRecoverabilityInspect     = "inspect_required"
)

type schedulerDiagnosticInput struct {
	WorkspaceID        string
	RunID              string
	Kind               string
	State              string
	Reason             string
	Detail             string
	Readback           *projectioncontract.ContinuationReadbackDiagnostic
	Recoverability     string
	RecommendedActions []string
	DedupeKey          string
}

type schedulerDiagnosticsStore struct {
	mu sync.Mutex

	recent   []projectioncontract.ContinuationDiagnostic
	attempts map[string]int

	commandCounts          map[labelPair]uint64
	commandEnqueueFailures map[labelPair]uint64
	continuationFailures   map[labelPair]uint64
	continuationNoActions  map[labelPair]uint64
	continuationCompleted  map[labelPair]uint64

	watchdogTimerRegistrations uint64
	watchdogTicks              uint64
	watchdogLatestTickAt       time.Time
	watchdogLatestDuration     time.Duration
	watchdogLatestError        string
	watchdogLastErrorAt        time.Time
	watchdogStaleRecoveries    map[labelPair]uint64
	watchdogStaleRecoveryRuns  uint64
}

type labelPair struct {
	Left  string
	Right string
}

func newSchedulerDiagnosticsStore() *schedulerDiagnosticsStore {
	return &schedulerDiagnosticsStore{
		attempts:                map[string]int{},
		commandCounts:           map[labelPair]uint64{},
		commandEnqueueFailures:  map[labelPair]uint64{},
		continuationFailures:    map[labelPair]uint64{},
		continuationNoActions:   map[labelPair]uint64{},
		continuationCompleted:   map[labelPair]uint64{},
		watchdogStaleRecoveries: map[labelPair]uint64{},
	}
}

func (s *schedulerDiagnosticsStore) recordCommand(kind, result string) {
	if s == nil {
		return
	}
	kind = schedulerLabel(kind, "unknown")
	result = schedulerLabel(result, "unknown")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandCounts[labelPair{Left: kind, Right: result}]++
}

func (s *schedulerDiagnosticsStore) recordCommandEnqueueFailure(kind, reason string) {
	if s == nil {
		return
	}
	kind = schedulerLabel(kind, "unknown")
	reason = schedulerLabel(reason, schedulerReasonEnqueueFailed)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandEnqueueFailures[labelPair{Left: kind, Right: reason}]++
}

func (s *schedulerDiagnosticsStore) recordContinuation(input schedulerDiagnosticInput) projectioncontract.ContinuationDiagnostic {
	if s == nil {
		return projectioncontract.ContinuationDiagnostic{}
	}
	item := projectioncontract.ContinuationDiagnostic{
		WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
		RunID:              strings.TrimSpace(input.RunID),
		Kind:               schedulerLabel(input.Kind, "unknown"),
		State:              schedulerLabel(input.State, schedulerDiagnosticStateFailed),
		Reason:             schedulerLabel(input.Reason, ""),
		Detail:             truncateSchedulerDetail(input.Detail),
		Readback:           cloneContinuationReadbackDiagnostic(input.Readback),
		Recoverability:     strings.TrimSpace(input.Recoverability),
		RecommendedActions: compactSchedulerActions(input.RecommendedActions),
		DedupeKey:          strings.TrimSpace(input.DedupeKey),
		OccurredAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Source:             schedulerDiagnosticSource,
	}
	if item.Recoverability == "" {
		item.Recoverability = schedulerRecoverabilityFor(item.State, item.Reason)
	}
	if item.DedupeKey == "" {
		item.DedupeKey = schedulerDedupeKey(item.WorkspaceID, item.RunID, item.Kind, item.Reason)
	}
	if len(item.RecommendedActions) == 0 {
		item.RecommendedActions = schedulerRecommendedActions(item.Kind, item.Reason, item.Recoverability)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if item.DedupeKey != "" {
		s.attempts[item.DedupeKey]++
		item.Attempt = s.attempts[item.DedupeKey]
	}
	s.recent = append(s.recent, item)
	if len(s.recent) > schedulerDiagnosticLimit {
		copy(s.recent, s.recent[len(s.recent)-schedulerDiagnosticLimit:])
		s.recent = s.recent[:schedulerDiagnosticLimit]
	}
	pair := labelPair{Left: item.Kind, Right: item.Reason}
	switch item.State {
	case schedulerDiagnosticStateFailed:
		s.continuationFailures[pair]++
	case schedulerDiagnosticStateNoActionNeeded:
		s.continuationNoActions[pair]++
	case schedulerDiagnosticStateCompleted:
		s.continuationCompleted[pair]++
	}
	return item
}

func (s *schedulerDiagnosticsStore) recordWatchdogTimerRegistration() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogTimerRegistrations++
}

func (s *schedulerDiagnosticsStore) recordWatchdogTick(start time.Time, duration time.Duration, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogTicks++
	if start.IsZero() {
		start = time.Now().UTC()
	}
	s.watchdogLatestTickAt = start.UTC()
	s.watchdogLatestDuration = duration
	if err != nil {
		s.watchdogLatestError = truncateSchedulerDetail(err.Error())
		s.watchdogLastErrorAt = time.Now().UTC()
	} else {
		s.watchdogLatestError = ""
	}
}

func (s *schedulerDiagnosticsStore) recordStaleRuntimeGapRecovery(gap, safety string, recoveredRuns int) {
	if s == nil || recoveredRuns <= 0 {
		return
	}
	gap = schedulerLabel(gap, "stale_dispatched")
	safety = schedulerLabel(safety, "runtime_recovery_policy")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogStaleRecoveries[labelPair{Left: gap, Right: safety}] += uint64(recoveredRuns)
	s.watchdogStaleRecoveryRuns += uint64(recoveredRuns)
}

func (s *schedulerDiagnosticsStore) snapshot(workspaceID, runID string) *projectioncontract.SchedulerDiagnostics {
	if s == nil {
		return nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)

	s.mu.Lock()
	defer s.mu.Unlock()

	var recent []projectioncontract.ContinuationDiagnostic
	for i := len(s.recent) - 1; i >= 0; i-- {
		item := s.recent[i]
		if schedulerDiagnosticMatches(item, workspaceID, runID) {
			recent = append(recent, cloneContinuationDiagnostic(item))
		}
	}
	ret := &projectioncontract.SchedulerDiagnostics{
		Watchdog: projectioncontract.WatchdogDiagnostics{
			TimerRegistrations: s.watchdogTimerRegistrations,
			LatestTickAt:       formatSchedulerTime(s.watchdogLatestTickAt),
			LatestDurationMs:   int64(s.watchdogLatestDuration / time.Millisecond),
			LatestError:        s.watchdogLatestError,
			StaleRecoveryRuns:  s.watchdogStaleRecoveryRuns,
		},
		RecentContinuations: recent,
	}
	if len(recent) > 0 {
		latest := recent[0]
		ret.LatestContinuation = &latest
		ret.ContinuationFailureCounts = continuationCountsFromRecent(recent, schedulerDiagnosticStateFailed)
		ret.ContinuationNoActionCounts = continuationCountsFromRecent(recent, schedulerDiagnosticStateNoActionNeeded)
		ret.ContinuationCompletedCounts = continuationCountsFromRecent(recent, schedulerDiagnosticStateCompleted)
	}
	if ret.Watchdog.LatestTickAt == "" && len(recent) == 0 && workspaceID != "" {
		return nil
	}
	return ret
}

func (s *schedulerDiagnosticsStore) renderMetrics() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	b.WriteString("# TYPE workorch_scheduler_commands_total counter\n")
	writeLabelPairMetrics(&b, "workorch_scheduler_commands_total", "kind", "result", s.commandCounts)
	b.WriteString("# TYPE workorch_scheduler_command_enqueue_failures_total counter\n")
	writeLabelPairMetrics(&b, "workorch_scheduler_command_enqueue_failures_total", "kind", "reason", s.commandEnqueueFailures)
	b.WriteString("# TYPE workorch_scheduler_continuation_failures_total counter\n")
	writeLabelPairMetrics(&b, "workorch_scheduler_continuation_failures_total", "kind", "reason", s.continuationFailures)
	b.WriteString("# TYPE workorch_scheduler_continuation_no_action_total counter\n")
	writeLabelPairMetrics(&b, "workorch_scheduler_continuation_no_action_total", "kind", "reason", s.continuationNoActions)
	b.WriteString("# TYPE workorch_watchdog_timer_registrations_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_watchdog_timer_registrations_total %d\n", s.watchdogTimerRegistrations))
	b.WriteString("# TYPE workorch_watchdog_interval_seconds gauge\n")
	b.WriteString(fmt.Sprintf("workorch_watchdog_interval_seconds %.0f\n", runWatchdogInterval.Seconds()))
	b.WriteString("# TYPE workorch_watchdog_ticks_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_watchdog_ticks_total %d\n", s.watchdogTicks))
	b.WriteString("# TYPE workorch_watchdog_last_error_timestamp_seconds gauge\n")
	b.WriteString(fmt.Sprintf("workorch_watchdog_last_error_timestamp_seconds %d\n", schedulerUnixSeconds(s.watchdogLastErrorAt)))
	b.WriteString("# TYPE workorch_watchdog_stale_runtime_gap_recoveries_total counter\n")
	writeLabelPairMetrics(&b, "workorch_watchdog_stale_runtime_gap_recoveries_total", "gap", "safety", s.watchdogStaleRecoveries)
	return b.String()
}

func schedulerDiagnosticMatches(item projectioncontract.ContinuationDiagnostic, workspaceID, runID string) bool {
	if workspaceID != "" && strings.TrimSpace(item.WorkspaceID) != workspaceID {
		return false
	}
	if runID != "" && strings.TrimSpace(item.RunID) != runID {
		return false
	}
	return true
}

func continuationCountsFromRecent(items []projectioncontract.ContinuationDiagnostic, state string) []projectioncontract.ContinuationDiagnosticCount {
	counts := map[labelPair]uint64{}
	for _, item := range items {
		if item.State != state {
			continue
		}
		counts[labelPair{Left: item.Kind, Right: item.Reason}]++
	}
	if len(counts) == 0 {
		return nil
	}
	keys := sortedLabelPairs(counts)
	ret := make([]projectioncontract.ContinuationDiagnosticCount, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, projectioncontract.ContinuationDiagnosticCount{
			Kind:   key.Left,
			Reason: key.Right,
			Count:  counts[key],
		})
	}
	return ret
}

func writeLabelPairMetrics(b *strings.Builder, name, leftLabel, rightLabel string, values map[labelPair]uint64) {
	keys := sortedLabelPairs(values)
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("%s{%s=\"%s\",%s=\"%s\"} %d\n",
			name,
			leftLabel,
			prometheusLabelValue(key.Left),
			rightLabel,
			prometheusLabelValue(key.Right),
			values[key],
		))
	}
}

func sortedLabelPairs(values map[labelPair]uint64) []labelPair {
	keys := make([]labelPair, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Left == keys[j].Left {
			return keys[i].Right < keys[j].Right
		}
		return keys[i].Left < keys[j].Left
	})
	return keys
}

func cloneContinuationDiagnostic(item projectioncontract.ContinuationDiagnostic) projectioncontract.ContinuationDiagnostic {
	item.RecommendedActions = append([]string(nil), item.RecommendedActions...)
	item.Readback = cloneContinuationReadbackDiagnostic(item.Readback)
	return item
}

func cloneContinuationReadbackDiagnostic(input *projectioncontract.ContinuationReadbackDiagnostic) *projectioncontract.ContinuationReadbackDiagnostic {
	if input == nil {
		return nil
	}
	ret := *input
	return &ret
}

func compactSchedulerActions(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(input))
	for _, item := range input {
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

func schedulerRecommendedActions(kind, reason, recoverability string) []string {
	if recoverability == schedulerRecoverabilityNoAction {
		return nil
	}
	switch kind {
	case schedulerKindWorkspaceRestart:
		return []string{"workspace_restart"}
	case schedulerKindWorkspaceActivation:
		if reason == schedulerReasonWorkspaceActivationFailed {
			// Bootstrap compilation happens before a Run exists. Resume cannot
			// make an incomplete profile/definition evidence contract executable;
			// surface the repair boundary and the only valid subsequent command.
			return []string{"repair_profile_or_definition", "workspace_restart"}
		}
		return []string{"run_resume"}
	case schedulerKindRunContinuation:
		return []string{"run_resume"}
	default:
		if reason == schedulerReasonRunSlotUnavailable || reason == schedulerReasonServiceShuttingDown {
			return []string{"run_resume", "workspace_restart"}
		}
		return nil
	}
}

func schedulerRecoverabilityFor(state, reason string) string {
	if state == schedulerDiagnosticStateNoActionNeeded {
		return schedulerRecoverabilityNoAction
	}
	switch reason {
	case schedulerReasonWorkspaceNotEligible, schedulerReasonRunNotEligible:
		return schedulerRecoverabilityNoAction
	case schedulerReasonWorkspaceActivationFailed, schedulerReasonOwnerCommandFailed, schedulerReasonBackgroundFailed, schedulerReasonOwnerReadbackNotAdvanced:
		return schedulerRecoverabilityInspect
	default:
		return schedulerRecoverabilityManualRetry
	}
}

func schedulerDedupeKey(workspaceID, runID, kind, reason string) string {
	parts := []string{strings.TrimSpace(workspaceID), strings.TrimSpace(runID), strings.TrimSpace(kind), strings.TrimSpace(reason)}
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			ret = append(ret, part)
		}
	}
	return strings.Join(ret, "/")
}

func schedulerLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func truncateSchedulerDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) <= schedulerDetailLimit {
		return value
	}
	runes := []rune(value)
	return string(runes[:schedulerDetailLimit])
}

func formatSchedulerTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func schedulerUnixSeconds(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
