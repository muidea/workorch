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
		case string(common.StepStateDispatched):
			ret.StepDispatched++
		case string(common.StepStateCompleted):
			ret.StepCompleted++
		case string(common.StepStateFailed):
			ret.StepFailed++
		}
	}
	return ret
}
