package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

const (
	executionAdmissionKindWorkspaceRun = "workspace_run"

	executionAdmissionReasonSubmissionFailed = "submission_failed"
)

// executionAdmissionMetrics contains monotonic process metrics only. It does
// not retain request identity, attempt counters, recovery advice or a recent
// failure list, and therefore cannot act as an execution-admission read model.
type executionAdmissionMetrics struct {
	mu sync.Mutex

	commandCounts     map[labelPair]uint64
	admissionFailures map[labelPair]uint64
}

type labelPair struct {
	Left  string
	Right string
}

func newExecutionAdmissionMetrics() *executionAdmissionMetrics {
	return &executionAdmissionMetrics{
		commandCounts:     map[labelPair]uint64{},
		admissionFailures: map[labelPair]uint64{},
	}
}

func (m *executionAdmissionMetrics) recordCommand(kind, result string) {
	if m == nil {
		return
	}
	kind = executionAdmissionMetricLabel(kind, "unknown")
	result = executionAdmissionMetricLabel(result, "unknown")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandCounts[labelPair{Left: kind, Right: result}]++
}

func (m *executionAdmissionMetrics) recordFailure(kind, reason string) {
	if m == nil {
		return
	}
	kind = executionAdmissionMetricLabel(kind, "unknown")
	reason = executionAdmissionMetricLabel(reason, "unknown")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.admissionFailures[labelPair{Left: kind, Right: reason}]++
}

func (m *executionAdmissionMetrics) render() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("# TYPE workorch_execution_admission_commands_total counter\n")
	writeLabelPairMetrics(&b, "workorch_execution_admission_commands_total", "kind", "result", m.commandCounts)
	b.WriteString("# TYPE workorch_execution_admission_failures_total counter\n")
	writeLabelPairMetrics(&b, "workorch_execution_admission_failures_total", "kind", "reason", m.admissionFailures)
	return b.String()
}

func executionAdmissionKindFromTopic(topic string) string {
	switch strings.TrimSpace(topic) {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return executionAdmissionKindWorkspaceRun
	default:
		return "workorchd_execution_admission"
	}
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

func executionAdmissionMetricLabel(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
