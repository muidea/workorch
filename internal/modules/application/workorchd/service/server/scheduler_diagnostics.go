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
	schedulerDiagnosticSource = "workorchd/scheduler"
	schedulerDiagnosticLimit  = 64
	schedulerDetailLimit      = 512

	schedulerKindWorkspaceActivation = "workspace_activation"
	schedulerKindWorkspaceRun        = "workspace_run"

	schedulerDiagnosticOutcomeAdmissionRejected = "admission_rejected"

	schedulerReasonWorkspaceActivationFailed = "workspace_activation_failed"

	schedulerRecoverabilityInspect = "inspect_required"
)

type schedulerDiagnosticInput struct {
	WorkspaceID        string
	RunID              string
	Kind               string
	Outcome            string
	Reason             string
	Detail             string
	Recoverability     string
	RecommendedActions []string
	DedupeKey          string
}

type schedulerDiagnosticsStore struct {
	mu sync.Mutex

	recent   []projectioncontract.ExecutionAdmissionFailureDiagnostic
	attempts map[string]int

	commandCounts     map[labelPair]uint64
	admissionFailures map[labelPair]uint64
}

type labelPair struct {
	Left  string
	Right string
}

func newSchedulerDiagnosticsStore() *schedulerDiagnosticsStore {
	return &schedulerDiagnosticsStore{
		attempts:          map[string]int{},
		commandCounts:     map[labelPair]uint64{},
		admissionFailures: map[labelPair]uint64{},
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

func (s *schedulerDiagnosticsStore) recordAdmissionFailure(input schedulerDiagnosticInput) projectioncontract.ExecutionAdmissionFailureDiagnostic {
	if s == nil {
		return projectioncontract.ExecutionAdmissionFailureDiagnostic{}
	}
	item := projectioncontract.ExecutionAdmissionFailureDiagnostic{
		WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
		RunID:              strings.TrimSpace(input.RunID),
		Kind:               schedulerLabel(input.Kind, "unknown"),
		Outcome:            schedulerLabel(input.Outcome, schedulerDiagnosticOutcomeAdmissionRejected),
		Reason:             schedulerLabel(input.Reason, ""),
		Detail:             truncateSchedulerDetail(input.Detail),
		Recoverability:     strings.TrimSpace(input.Recoverability),
		RecommendedActions: compactSchedulerActions(input.RecommendedActions),
		DedupeKey:          strings.TrimSpace(input.DedupeKey),
		OccurredAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Source:             schedulerDiagnosticSource,
	}
	if item.DedupeKey == "" {
		item.DedupeKey = schedulerDedupeKey(item.WorkspaceID, item.RunID, item.Kind, item.Reason)
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
	s.admissionFailures[pair]++
	return item
}

func (s *schedulerDiagnosticsStore) snapshot(workspaceID, runID string) *projectioncontract.SchedulerDiagnostics {
	if s == nil {
		return nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)

	s.mu.Lock()
	defer s.mu.Unlock()

	var recent []projectioncontract.ExecutionAdmissionFailureDiagnostic
	for i := len(s.recent) - 1; i >= 0; i-- {
		item := s.recent[i]
		if schedulerDiagnosticMatches(item, workspaceID, runID) {
			recent = append(recent, cloneExecutionAdmissionFailureDiagnostic(item))
		}
	}
	ret := &projectioncontract.SchedulerDiagnostics{RecentAdmissionFailures: recent}
	if len(recent) > 0 {
		latest := recent[0]
		ret.LatestAdmissionFailure = &latest
		ret.AdmissionFailureCounts = admissionFailureCountsFromRecent(recent)
	}
	if len(recent) == 0 && workspaceID != "" {
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
	b.WriteString("# TYPE workorch_execution_admission_commands_total counter\n")
	writeLabelPairMetrics(&b, "workorch_execution_admission_commands_total", "kind", "result", s.commandCounts)
	b.WriteString("# TYPE workorch_execution_admission_failures_total counter\n")
	writeLabelPairMetrics(&b, "workorch_execution_admission_failures_total", "kind", "reason", s.admissionFailures)
	return b.String()
}

func schedulerDiagnosticMatches(item projectioncontract.ExecutionAdmissionFailureDiagnostic, workspaceID, runID string) bool {
	if workspaceID != "" && strings.TrimSpace(item.WorkspaceID) != workspaceID {
		return false
	}
	if runID != "" && strings.TrimSpace(item.RunID) != runID {
		return false
	}
	return true
}

func admissionFailureCountsFromRecent(items []projectioncontract.ExecutionAdmissionFailureDiagnostic) []projectioncontract.ExecutionAdmissionFailureCount {
	counts := map[labelPair]uint64{}
	for _, item := range items {
		if item.Outcome != schedulerDiagnosticOutcomeAdmissionRejected {
			continue
		}
		counts[labelPair{Left: item.Kind, Right: item.Reason}]++
	}
	if len(counts) == 0 {
		return nil
	}
	keys := sortedLabelPairs(counts)
	ret := make([]projectioncontract.ExecutionAdmissionFailureCount, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, projectioncontract.ExecutionAdmissionFailureCount{
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

func cloneExecutionAdmissionFailureDiagnostic(item projectioncontract.ExecutionAdmissionFailureDiagnostic) projectioncontract.ExecutionAdmissionFailureDiagnostic {
	item.RecommendedActions = append([]string(nil), item.RecommendedActions...)
	return item
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
