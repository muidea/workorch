package runports

import (
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeExecutionGap string

const (
	runtimeExecutionGapUnknown              runtimeExecutionGap = ""
	runtimeExecutionGapRequestNotAccepted   runtimeExecutionGap = "request_not_accepted"
	runtimeExecutionGapAcceptedNoProgress   runtimeExecutionGap = "accepted_no_progress"
	runtimeExecutionGapAcceptedInProgress   runtimeExecutionGap = "accepted_in_progress"
	runtimeExecutionGapTerminalNotFinalized runtimeExecutionGap = "runtime_terminal_not_finalized"
	runtimeExecutionGapWorkerLost           runtimeExecutionGap = "worker_lost"
	runtimeExecutionGapFinalized            runtimeExecutionGap = "finalized"
)

type runtimeExecutionGapState struct {
	Gap                runtimeExecutionGap
	LatestStepTime     string
	RequestID          string
	QueueKey           string
	HasTerminalResult  bool
	HasWorkerLost      bool
	HasWorkerHeartbeat bool
	HasWorkerStopped   bool
	RecoveryPolicy     runtimemodel.RuntimeRecoveryPolicy
	RecoverySafety     string
}

func classifyRuntimeExecutionGapForStep(step *runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint) runtimeExecutionGapState {
	if step == nil {
		return runtimeExecutionGapState{}
	}
	state := runtimeExecutionGapState{Gap: runtimeExecutionGapRequestNotAccepted}
	accepted := false
	progressed := false
	runtimeTerminal := false
	finalized := false
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.CurrentStepID != step.ID {
			continue
		}
		if checkpoint.OccurredAt != "" &&
			(state.LatestStepTime == "" || checkpoint.OccurredAt > state.LatestStepTime) {
			state.LatestStepTime = checkpoint.OccurredAt
		}
		if requestID := strings.TrimSpace(runtimeCheckpointString(checkpoint.Data["requestId"])); requestID != "" {
			state.RequestID = requestID
		}
		if queueKey := strings.TrimSpace(runtimeCheckpointString(checkpoint.Data["queueKey"])); queueKey != "" {
			state.QueueKey = queueKey
		}
		if rawPolicy, ok := checkpoint.Data["runtimeRecoveryPolicy"]; ok {
			state.RecoveryPolicy = runtimemodel.RuntimeRecoveryPolicyFromValue(rawPolicy)
			state.RecoverySafety = state.RecoveryPolicy.WatchdogRecoverySafety()
		}
		if safety := strings.TrimSpace(runtimeCheckpointString(checkpoint.Data["recoverySafety"])); safety != "" {
			state.RecoverySafety = safety
		}
		event := strings.TrimSpace(runtimeCheckpointString(checkpoint.Data["event"]))
		switch event {
		case "runtime_execute_accepted":
			accepted = true
		case "runtime_step_done", "runtime_step_failed":
			accepted = true
			progressed = true
			runtimeTerminal = true
		case "runtime_step_result_available":
			accepted = true
			progressed = true
			runtimeTerminal = true
			state.HasTerminalResult = true
		case "runtime_worker_started":
			accepted = true
			progressed = true
		case "runtime_worker_heartbeat":
			accepted = true
			progressed = true
			state.HasWorkerHeartbeat = true
		case "runtime_worker_stopped":
			accepted = true
			progressed = true
			state.HasWorkerStopped = true
		case "runtime_worker_lost":
			accepted = true
			progressed = true
			state.HasWorkerLost = true
		case "step_completed", "step_failed":
			finalized = true
		default:
			if strings.HasPrefix(event, "runtime_") || strings.Contains(event, "progress") {
				accepted = true
				progressed = true
			}
		}
	}
	switch {
	case finalized:
		state.Gap = runtimeExecutionGapFinalized
	case runtimeTerminal:
		state.Gap = runtimeExecutionGapTerminalNotFinalized
	case state.HasWorkerLost:
		state.Gap = runtimeExecutionGapWorkerLost
	case progressed:
		state.Gap = runtimeExecutionGapAcceptedInProgress
	case accepted:
		state.Gap = runtimeExecutionGapAcceptedNoProgress
	default:
		state.Gap = runtimeExecutionGapRequestNotAccepted
	}
	return state
}

func runtimeGapAllowsAutomaticRecovery(state runtimeExecutionGapState) bool {
	switch state.Gap {
	case runtimeExecutionGapTerminalNotFinalized:
		return state.HasTerminalResult
	case runtimeExecutionGapWorkerLost, runtimeExecutionGapAcceptedNoProgress, runtimeExecutionGapAcceptedInProgress:
		return state.RecoveryPolicy.AllowsWatchdogExternalRetry()
	default:
		return false
	}
}

func runtimeGapRecoverySafetyClassification(state runtimeExecutionGapState) string {
	switch state.Gap {
	case runtimeExecutionGapTerminalNotFinalized:
		if state.HasTerminalResult {
			return "terminal_result_evidence"
		}
	case runtimeExecutionGapWorkerLost, runtimeExecutionGapAcceptedNoProgress, runtimeExecutionGapAcceptedInProgress:
		if safety := strings.TrimSpace(state.RecoverySafety); safety != "" {
			return safety
		}
		return state.RecoveryPolicy.WatchdogRecoverySafety()
	}
	if safety := strings.TrimSpace(state.RecoverySafety); safety != "" {
		return safety
	}
	return state.RecoveryPolicy.WatchdogRecoverySafety()
}

func runtimeCheckpointString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
