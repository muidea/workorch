package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MapRuntimeProgress(snapshot readmodelcore.RunSnapshot) readmodelcore.RunProgress {
	ret := readmodelcore.RunProgress{
		State:     snapshot.Run.State,
		StepTotal: len(snapshot.Steps),
		Blocked:   snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "",
		Terminal:  IsTerminalRunState(snapshot.Run.State),
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
	}
	for _, step := range snapshot.Steps {
		switch step.State {
		case string(common.StepStateExecuting):
			ret.StepDispatched++
		case string(common.StepStateSucceeded):
			ret.StepCompleted++
		case string(common.StepStateFailed):
			ret.StepFailed++
		}
	}
	// Runtime V2 authority progress is authoritative when step projections are empty.
	// All counters use dispatch units so stepCompleted never exceeds stepTotal.
	if ret.StepTotal == 0 && snapshot.TaskProgress.RuntimeV2 != nil {
		summary := snapshot.TaskProgress.RuntimeV2
		completed := summary.CompletedDispatchCount
		active := 0
		if summary.ActiveDispatchID != "" {
			active = 1
		}
		ret.StepTotal = completed + active
		ret.StepDispatched = active
		// Approximate completed/failed dispatches from evidence polarity while
		// keeping the unit equal to dispatch count.
		failed := summary.RejectedEvidenceCount
		if failed > completed {
			failed = completed
		}
		ret.StepFailed = failed
		ret.StepCompleted = completed - failed
		if ret.StepCompleted < 0 {
			ret.StepCompleted = 0
		}
		if ret.CurrentStepID == "" {
			if summary.ActiveDispatchID != "" {
				ret.CurrentStepID = summary.ActiveDispatchID
			}
		}
		if ret.CurrentStepKind == "" && summary.ConvergenceState != "" {
			ret.CurrentStepKind = "runtime_v2/" + summary.ConvergenceState
		}
		if ret.CurrentStepState == "" {
			switch {
			case ret.Terminal && snapshot.Run.State == string(common.RunStateFailed):
				ret.CurrentStepState = "rejected"
			case ret.Terminal && snapshot.Run.State == string(common.RunStateCompleted):
				ret.CurrentStepState = "completed"
			case summary.ActiveDispatchID != "":
				ret.CurrentStepState = "executing"
			case summary.CandidateAttemptCount > 0:
				ret.CurrentStepState = "running"
			}
		}
	}
	return ret
}
