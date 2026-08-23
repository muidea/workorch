package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: run_outcome_result.go

func applyRunOutcomeTerminalResult(ret *projectioncontract.RunDisplayOutcome, result projectioncontract.Result) {
	if result.State == string(common.ResultStateAccepted) {
		ret.State = "accepted"
		ret.Accepted = true
	} else {
		ret.State = "failed"
		ret.Reason = result.Summary
	}
	ret.Terminal = true
	ret.ResultID = result.ID
	ret.Summary = result.Summary
	ret.EvidenceRefs = compactUniqueStrings(result.EvidenceRefs)
	ret.ArtifactRefs = compactUniqueStrings(result.ArtifactRefs)
}

// source: runtime_evidence_state_accumulators.go

func accumulateToolCallEvidenceStates(ret *projectioncontract.RuntimeEvidenceSummary, items []projectioncontract.ToolCall) {
	for _, item := range items {
		switch item.State {
		case string(common.ToolCallStateRunning):
			ret.ToolCallRunning++
		case string(common.ToolCallStateCompleted):
			ret.ToolCallCompleted++
		case string(common.ToolCallStateFailed):
			ret.ToolCallFailed++
		}
	}
}

func accumulatePatchEvidenceStates(ret *projectioncontract.RuntimeEvidenceSummary, items []projectioncontract.Patch) {
	for _, item := range items {
		switch item.State {
		case string(common.PatchStateApplied):
			ret.PatchApplied++
		case string(common.PatchStateFailed):
			ret.PatchFailed++
		}
	}
}

func accumulateValidationEvidenceStates(ret *projectioncontract.RuntimeEvidenceSummary, items []projectioncontract.ValidationResult) {
	for _, item := range items {
		switch item.State {
		case string(common.ValidationResultStatePassed):
			ret.ValidationPassed++
		case string(common.ValidationResultStateFailed):
			ret.ValidationFailed++
		case string(common.ValidationResultStateWarning):
			ret.ValidationWarning++
		}
	}
}

// source: snapshot_step_candidates.go

func latestNonTerminalSnapshotStep(steps []projectioncontract.StepView) *projectioncontract.StepView {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		if steps[idx].State == string(common.StepStateExecuting) || steps[idx].State == string(common.StepStatePending) {
			return &steps[idx]
		}
	}
	return nil
}

func terminalSnapshotStep(steps []projectioncontract.StepView) *projectioncontract.StepView {
	terminals := terminalSnapshotStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == string(common.StepStateExecuting) {
			return step
		}
	}
	return terminals[len(terminals)-1]
}

// source: task_decomposition_state.go

func taskNodeStateFromStep(state string) string {
	switch state {
	case string(common.StepStateSucceeded):
		return "completed"
	case string(common.StepStateFailed):
		return "failed"
	case string(common.StepStateExecuting):
		return "running"
	default:
		if state != "" {
			return state
		}
		return "pending"
	}
}

func taskDecompositionState(item projectioncontract.TaskDecomposition) string {
	if item.NodeFailed > 0 {
		return "failed"
	}
	if item.NodeTotal > 0 && item.NodeCompleted == item.NodeTotal {
		return "completed"
	}
	if item.NodeCompleted > 0 || item.NodeRunning > 0 {
		return "running"
	}
	return "pending"
}
