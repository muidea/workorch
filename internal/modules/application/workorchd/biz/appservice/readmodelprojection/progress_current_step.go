package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func applyTaskProgressCurrentStep(ret *readmodelcore.TaskProgress, snapshot readmodelcore.RunSnapshot) {
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		ret.Recovery.StepID = snapshot.Step.ID
	}
	if result := TerminalTaskProgressResult(snapshot); result != nil {
		ret.CurrentStepID = firstNonEmptyTimelineString(result.SourceRef, result.ID)
		ret.CurrentStepKind = "result/" + strings.TrimSpace(firstNonEmptyTimelineString(result.SourceType, "terminal"))
		ret.CurrentStepState = result.State
		ret.Recovery.StepID = ret.CurrentStepID
	}
}
