package readmodelprojection

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeExecutionGapRequestNotAccepted         = "request_not_accepted"
	runtimeExecutionGapAcceptedNoProgress         = "accepted_no_progress"
	runtimeExecutionGapAcceptedInProgress         = "accepted_in_progress"
	runtimeExecutionGapRuntimeTerminalUnfinalized = "runtime_terminal_not_finalized"
	runtimeExecutionGapWorkerLost                 = "worker_lost"
)

func MapRuntimeExecutionState(steps []StepView, checkpoints []readmodelcore.RuntimeCheckpoint, currentStep *StepView) *readmodelcore.RuntimeExecutionState {
	if len(steps) == 0 && currentStep == nil {
		return nil
	}
	step := currentStep
	if step == nil {
		step = LatestNonTerminalSnapshotStep(steps)
	}
	if step == nil {
		return nil
	}
	state := &readmodelcore.RuntimeExecutionState{
		StepID:            step.ID,
		StepKind:          CanonicalStepKind(step.Kind),
		StepState:         step.State,
		ContractViolation: runtimeExecutionContractViolationFromStep(step),
	}
	for _, checkpoint := range orderedRuntimeCheckpoints(checkpoints) {
		if checkpoint.CurrentStepID != step.ID {
			continue
		}
		applyRuntimeExecutionCheckpoint(state, checkpoint)
	}
	state.ResourceHealth = RuntimeResourceHealthFromSteps(steps)
	deriveRuntimeExecutionGap(state)
	if state.RequestID == "" && state.AcceptedAt == "" && state.LastProgressAt == "" && state.DoneAt == "" && state.FailedAt == "" && state.FinalizedAt == "" && state.Gap == "" && state.ContractViolation == nil && len(state.ResourceHealth) == 0 {
		return nil
	}
	return state
}

func orderedRuntimeCheckpoints(items []readmodelcore.RuntimeCheckpoint) []readmodelcore.RuntimeCheckpoint {
	if len(items) <= 1 {
		return append([]readmodelcore.RuntimeCheckpoint(nil), items...)
	}
	ret := append([]readmodelcore.RuntimeCheckpoint(nil), items...)
	sortRuntimeCheckpoints(ret)
	return ret
}

func sortRuntimeCheckpoints(items []readmodelcore.RuntimeCheckpoint) {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].OccurredAt != items[right].OccurredAt {
			return items[left].OccurredAt < items[right].OccurredAt
		}
		return items[left].ID < items[right].ID
	})
}

func applyRuntimeExecutionCheckpoint(state *readmodelcore.RuntimeExecutionState, checkpoint readmodelcore.RuntimeCheckpoint) {
	if state == nil {
		return
	}
	event := strings.TrimSpace(readModelStringValue(checkpoint.Data["event"]))
	if event == "" {
		return
	}
	applyRuntimeExecutionRecoveryPolicy(state, checkpoint)
	switch event {
	case "runtime_execute_accepted":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.QueueKey = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["queueKey"]), state.QueueKey)
		state.AcceptedAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["acceptedAt"]), checkpoint.OccurredAt, state.AcceptedAt)
	case "runtime_step_done":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.DoneAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["occurredAt"]), checkpoint.OccurredAt, state.DoneAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_step_result_available":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.DoneAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["completedAt"]), checkpoint.OccurredAt, state.DoneAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_step_failed":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.FailedAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["occurredAt"]), checkpoint.OccurredAt, state.FailedAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_worker_started":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.QueueKey = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["queueKey"]), state.QueueKey)
		state.WorkerID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["workerId"]), state.WorkerID)
		state.WorkerStartedAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["startedAt"]), checkpoint.OccurredAt, state.WorkerStartedAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_worker_heartbeat":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.QueueKey = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["queueKey"]), state.QueueKey)
		state.WorkerID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["workerId"]), state.WorkerID)
		state.WorkerHeartbeatAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["heartbeatAt"]), checkpoint.OccurredAt, state.WorkerHeartbeatAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_worker_stopped":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.QueueKey = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["queueKey"]), state.QueueKey)
		state.WorkerID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["workerId"]), state.WorkerID)
		state.WorkerStoppedAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["stoppedAt"]), checkpoint.OccurredAt, state.WorkerStoppedAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "runtime_worker_lost":
		state.RequestID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["requestId"]), state.RequestID)
		state.QueueKey = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["queueKey"]), state.QueueKey)
		state.WorkerID = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["workerId"]), state.WorkerID)
		state.WorkerLostAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["lostAt"]), checkpoint.OccurredAt, state.WorkerLostAt)
		applyRuntimeExecutionProgress(state, checkpoint, event)
	case "step_completed", "step_failed":
		state.FinalizedAt = checkpoint.OccurredAt
		state.FinalizeEvent = event
	default:
		if strings.HasPrefix(event, "runtime_") || strings.Contains(event, "progress") {
			applyRuntimeExecutionProgress(state, checkpoint, event)
		}
	}
}

func applyRuntimeExecutionRecoveryPolicy(state *readmodelcore.RuntimeExecutionState, checkpoint readmodelcore.RuntimeCheckpoint) {
	if state == nil || checkpoint.Data == nil {
		return
	}
	if rawPolicy, ok := checkpoint.Data["runtimeRecoveryPolicy"]; ok {
		policy := runtimemodel.RuntimeRecoveryPolicyFromValue(rawPolicy)
		state.RuntimeRecoveryPolicy = policy.ToMap()
		if state.RecoverySafety == "" {
			state.RecoverySafety = policy.WatchdogRecoverySafety()
		}
	}
	if safety := strings.TrimSpace(readModelStringValue(checkpoint.Data["recoverySafety"])); safety != "" {
		state.RecoverySafety = safety
	}
	if violation := runtimeExecutionContractViolationFromValue(checkpoint.Data["contractViolation"]); violation != nil {
		state.ContractViolation = violation
	}
}

func applyRuntimeExecutionProgress(state *readmodelcore.RuntimeExecutionState, checkpoint readmodelcore.RuntimeCheckpoint, event string) {
	state.LastProgressAt = firstNonEmptyTimelineString(readModelStringValue(checkpoint.Data["occurredAt"]), checkpoint.OccurredAt, state.LastProgressAt)
	state.LastProgressEvent = event
	if stage := strings.TrimSpace(readModelStringValue(checkpoint.Data["stage"])); stage != "" {
		state.LastProgressStage = stage
	}
	if summary := strings.TrimSpace(readModelStringValue(checkpoint.Data["summary"])); summary != "" {
		state.LastProgressSummary = summary
	}
	if requestID := strings.TrimSpace(readModelStringValue(checkpoint.Data["requestId"])); requestID != "" {
		state.RequestID = requestID
	}
}

func runtimeExecutionContractViolationFromStep(step *StepView) *runtimemodel.ContractViolation {
	if step == nil {
		return nil
	}
	if violation := runtimeExecutionContractViolationFromValue(step.Input["contractViolationRecovery"]); violation != nil {
		return violation
	}
	if violation := runtimeExecutionContractViolationFromValue(step.Output["contractViolation"]); violation != nil {
		return violation
	}
	return nil
}

func runtimeExecutionContractViolationFromValue(value any) *runtimemodel.ContractViolation {
	switch typed := value.(type) {
	case nil:
		return nil
	case runtimemodel.ContractViolation:
		ret := typed
		if runtimeExecutionContractViolationEmpty(ret) {
			return nil
		}
		return &ret
	case *runtimemodel.ContractViolation:
		if typed == nil || runtimeExecutionContractViolationEmpty(*typed) {
			return nil
		}
		ret := *typed
		return &ret
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	var ret runtimemodel.ContractViolation
	if err := json.Unmarshal(data, &ret); err != nil || runtimeExecutionContractViolationEmpty(ret) {
		return nil
	}
	return &ret
}

func runtimeExecutionContractViolationEmpty(item runtimemodel.ContractViolation) bool {
	return strings.TrimSpace(item.ID) == "" &&
		strings.TrimSpace(item.ViolatedRule) == "" &&
		strings.TrimSpace(item.FailureSignature) == "" &&
		strings.TrimSpace(item.Recoverability) == ""
}

func deriveRuntimeExecutionGap(state *readmodelcore.RuntimeExecutionState) {
	if state == nil {
		return
	}
	terminalStep := state.StepState == "completed" || state.StepState == "failed"
	runtimeTerminal := state.DoneAt != "" || state.FailedAt != ""
	if terminalStep || state.FinalizedAt != "" {
		state.Pending = false
		state.Gap = ""
		return
	}
	state.Pending = true
	switch {
	case runtimeTerminal:
		state.Gap = runtimeExecutionGapRuntimeTerminalUnfinalized
	case state.WorkerLostAt != "":
		state.Gap = runtimeExecutionGapWorkerLost
	case state.AcceptedAt == "":
		state.Gap = runtimeExecutionGapRequestNotAccepted
	case state.LastProgressAt == "":
		state.Gap = runtimeExecutionGapAcceptedNoProgress
	default:
		state.Gap = runtimeExecutionGapAcceptedInProgress
	}
}
