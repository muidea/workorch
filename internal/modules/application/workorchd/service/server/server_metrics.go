package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Metrics are process-local observations for health and diagnostics. They are
// never used as an owner-state source or as a transition trigger.

type serverMetrics struct {
	healthzRequests              atomic.Uint64
	readyzRequests               atomic.Uint64
	metricsRequests              atomic.Uint64
	workspaceCreateRequests      atomic.Uint64
	workspaceDeleteRequests      atomic.Uint64
	workspaceListRequests        atomic.Uint64
	workspaceGetRequests         atomic.Uint64
	workspaceSnapshotRequests    atomic.Uint64
	goalDefineRequests           atomic.Uint64
	goalListRequests             atomic.Uint64
	goalGetRequests              atomic.Uint64
	dodDefineRequests            atomic.Uint64
	dodListRequests              atomic.Uint64
	dodGetRequests               atomic.Uint64
	goalDoDQualityCheckRequests  atomic.Uint64
	runCreateRequests            atomic.Uint64
	runListRequests              atomic.Uint64
	runGetRequests               atomic.Uint64
	runSnapshotRequests          atomic.Uint64
	runStatusRequests            atomic.Uint64
	runObservabilityRequests     atomic.Uint64
	artifactContentRequests      atomic.Uint64
	runTaskProgressRequests      atomic.Uint64
	runContextBundleRequests     atomic.Uint64
	runTaskDecompositionRequests atomic.Uint64
	eventListRequests            atomic.Uint64
	runStartRequests             atomic.Uint64
	runResumeRequests            atomic.Uint64
	workspaceActivationRequests  atomic.Uint64
	decisionListRequests         atomic.Uint64
	decisionGetRequests          atomic.Uint64
	decisionResolveRequests      atomic.Uint64
	approvalListRequests         atomic.Uint64
	approvalGetRequests          atomic.Uint64
	approvalApproveRequests      atomic.Uint64
	approvalRejectRequests       atomic.Uint64
	soulCreateRequests           atomic.Uint64
	soulUpdateRequests           atomic.Uint64
	soulDeleteRequests           atomic.Uint64
	soulListRequests             atomic.Uint64
	soulGetRequests              atomic.Uint64
	skillCreateRequests          atomic.Uint64
	skillListRequests            atomic.Uint64
	skillGetRequests             atomic.Uint64
	mcpServerCreateRequests      atomic.Uint64
	mcpServerListRequests        atomic.Uint64
	mcpServerGetRequests         atomic.Uint64
	mcpToolListRequests          atomic.Uint64
	mcpResourceListRequests      atomic.Uint64
	mcpPromptListRequests        atomic.Uint64
	mcpCapabilityCreateRequests  atomic.Uint64
	mcpCapabilityUpdateRequests  atomic.Uint64
	mcpCapabilityListRequests    atomic.Uint64
	mcpCapabilityGetRequests     atomic.Uint64
	memoryCreateRequests         atomic.Uint64
	memoryListRequests           atomic.Uint64
	configurationReadRequests    atomic.Uint64
	configurationUpdateRequests  atomic.Uint64
	runSucceeded                 atomic.Uint64
	runFailed                    atomic.Uint64
	runRejected                  atomic.Uint64
	runCanceled                  atomic.Uint64
	runInFlight                  atomic.Int64
	runQueueWaiting              atomic.Int64
	runQueuedTotal               atomic.Uint64
	convergenceRepairAttempts    atomic.Uint64
	runtimeMetricsMu             sync.Mutex
	observedTerminalStates       map[string]struct{}
	observedRepairAttempts       map[string]int
	observedRuntimeDrives        map[string]runtimeDriveMetric
	schedulerDiagnostics         *schedulerDiagnosticsStore
}

// runtimeDriveMetric is a process-local metric projection of an immutable
// Orchestrator observation. It is deliberately not an owner-state cache.
type runtimeDriveMetric struct {
	WorkspaceID        string
	RunID              string
	Counts             map[string]uint64
	AggregateRevision  uint64
	RecoveryGeneration uint64
	FailureCategory    string
	LeaseState         string
	Source             string
}

// observeRunState is an SM-4 observer only. The Run owner/eventstream own the
// source state; this process-local method deduplicates terminal snapshots and
// derives monotonic repair deltas without resuming or writing any owner state.
func (m *serverMetrics) observeRunState(workspaceID, runID, state string, runtimeRevision uint64, repairAttempts int) {
	if m == nil || strings.TrimSpace(runID) == "" {
		return
	}
	state = strings.TrimSpace(state)
	m.runtimeMetricsMu.Lock()
	defer m.runtimeMetricsMu.Unlock()
	if m.observedTerminalStates == nil {
		m.observedTerminalStates = make(map[string]struct{})
	}
	if m.observedRepairAttempts == nil {
		m.observedRepairAttempts = make(map[string]int)
	}
	runKey := strings.TrimSpace(workspaceID) + "/" + strings.TrimSpace(runID)
	if repairAttempts > m.observedRepairAttempts[runKey] {
		m.convergenceRepairAttempts.Add(uint64(repairAttempts - m.observedRepairAttempts[runKey]))
		m.observedRepairAttempts[runKey] = repairAttempts
	}
	if !terminalRunState(state) {
		return
	}
	terminalKey := fmt.Sprintf("%s/%s/%d", runKey, state, runtimeRevision)
	if _, observed := m.observedTerminalStates[terminalKey]; observed {
		return
	}
	m.observedTerminalStates[terminalKey] = struct{}{}
	switch state {
	case string(common.RunStateCompleted):
		m.runSucceeded.Add(1)
	case string(common.RunStateFailed):
		m.runFailed.Add(1)
	case string(common.RunStateCancelled):
		m.runCanceled.Add(1)
	case string(common.RunStateSuperseded):
		m.runRejected.Add(1)
	}
}

func (m *serverMetrics) observeRuntimeDrive(data orchestratorevents.RuntimeDriveObservedData) {
	if m == nil || data.Validate() != nil {
		return
	}
	data = data.Normalized()
	m.runtimeMetricsMu.Lock()
	defer m.runtimeMetricsMu.Unlock()
	if m.observedRuntimeDrives == nil {
		m.observedRuntimeDrives = make(map[string]runtimeDriveMetric)
	}
	key := string(data.WorkspaceID) + "/" + string(data.RunID)
	metric := m.observedRuntimeDrives[key]
	if metric.Counts == nil {
		metric = runtimeDriveMetric{WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), Counts: make(map[string]uint64)}
	}
	metric.Counts[string(data.Kind)]++
	metric.AggregateRevision = data.AggregateRevision
	metric.RecoveryGeneration = data.RecoveryGeneration
	metric.FailureCategory = data.FailureCategory
	metric.LeaseState = string(data.LeaseState)
	metric.Source = data.Source
	m.observedRuntimeDrives[key] = metric
}

func (m *serverMetrics) runtimeDriveMetricSnapshot() []runtimeDriveMetric {
	if m == nil {
		return nil
	}
	m.runtimeMetricsMu.Lock()
	defer m.runtimeMetricsMu.Unlock()
	ret := make([]runtimeDriveMetric, 0, len(m.observedRuntimeDrives))
	for _, metric := range m.observedRuntimeDrives {
		copy := metric
		copy.Counts = make(map[string]uint64, len(metric.Counts))
		for kind, count := range metric.Counts {
			copy.Counts[kind] = count
		}
		ret = append(ret, copy)
	}
	sort.Slice(ret, func(left, right int) bool {
		if ret[left].WorkspaceID == ret[right].WorkspaceID {
			return ret[left].RunID < ret[right].RunID
		}
		return ret[left].WorkspaceID < ret[right].WorkspaceID
	})
	return ret
}

func terminalRunState(state string) bool {
	switch state {
	case string(common.RunStateCompleted), string(common.RunStateFailed), string(common.RunStateCancelled), string(common.RunStateSuperseded):
		return true
	default:
		return false
	}
}
