package workspaceviewcore

import (
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func (s *Store) RunHeartbeatMetrics(now time.Time) Metrics {
	if s == nil {
		return Metrics{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ret Metrics
	for _, entry := range s.workspaces {
		if entry == nil || !entry.HasSnapshot {
			continue
		}
		addConvergenceMetrics(&ret, entry.Snapshot.Acceptance)
		heartbeat := entry.Snapshot.RunHeartbeat
		if strings.TrimSpace(heartbeat.RunID) == "" || !activeRunHeartbeatState(heartbeat.State) {
			continue
		}
		ret.ActiveRunHeartbeatTotal++
		if heartbeat.Stalled || strings.TrimSpace(heartbeat.ActivityState) == "stalled" {
			ret.StalledRunTotal++
		}
		if heartbeat.IneffectiveRetry || strings.TrimSpace(heartbeat.ActivityState) == "ineffective_retry" {
			ret.IneffectiveRetryRunTotal++
		}
		if age := runHeartbeatAgeSeconds(heartbeat, now); age > ret.ActiveRunHeartbeatAgeSecondsMax {
			ret.ActiveRunHeartbeatAgeSecondsMax = age
		}
	}
	return ret
}

func addConvergenceMetrics(ret *Metrics, acceptance projectioncontract.AcceptanceProjection) {
	if ret == nil {
		return
	}
	ret.ConvergenceActiveSlices += acceptance.Summary.ActiveSlices
	if acceptance.CurrentFocus != nil && activeCoverageFocusState(acceptance.CurrentFocus.State) {
		ret.ConvergenceOpenFocuses++
	}
	if acceptance.Budget != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.Budget.RepairAttempts
		ret.ConvergenceNoDeltaRoundsTotal += acceptance.Budget.NoCoverageDeltaRounds
		if convergenceBudgetEscalated(acceptance.Budget.Decision) {
			ret.ConvergenceBudgetEscalations++
		}
		return
	}
	if acceptance.CurrentFocus != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.CurrentFocus.Attempt
	}
}

func activeCoverageFocusState(state string) bool {
	state = strings.TrimSpace(state)
	return state == "" || state == "active" || state == "pending" || state == "blocked"
}

func convergenceBudgetEscalated(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "", "resume":
		return false
	default:
		return true
	}
}

func activeRunHeartbeatState(state string) bool {
	switch common.RunState(strings.TrimSpace(state)) {
	case common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval:
		return false
	default:
		return true
	}
}

func runHeartbeatAgeSeconds(heartbeat projectioncontract.RunHeartbeat, now time.Time) int64 {
	for _, candidate := range []string{
		heartbeat.LatestActivityAt,
		heartbeat.LatestEffectiveProgressAt,
		heartbeat.LatestEventAt,
		heartbeat.LatestLLMUsageAt,
		heartbeat.LatestResultAt,
		heartbeat.LatestArtifactAt,
		heartbeat.LatestPatchAt,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, candidate)
		if err != nil {
			continue
		}
		age := now.Sub(parsed.UTC()).Seconds()
		if age < 0 {
			return 0
		}
		return int64(age)
	}
	return 0
}
