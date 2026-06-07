package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func applyWorkspaceProgressCurrentStep(ret *readmodelcore.TaskProgress, snapshot readmodelcore.WorkspaceSnapshot) {
	if ret == nil || snapshot.Step == nil {
		return
	}
	ret.CurrentStepID = snapshot.Step.ID
	ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
	ret.CurrentStepState = snapshot.Step.State
	if ret.Recovery.StepID == "" {
		ret.Recovery.StepID = snapshot.Step.ID
	}
}
