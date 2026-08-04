package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyWorkspaceProgressCurrentStep(ret *projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshot) {
	if ret == nil || snapshot.Step == nil {
		return
	}
	ret.CurrentStepID = snapshot.Step.ID
	ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
	ret.CurrentStepState = snapshot.Step.State
	ret.CurrentActionLabel = currentActionLabelFromWorkspaceSnapshot(snapshot)
	ret.DiagnosticLineage = ret.CurrentStepID
	if ret.Recovery.StepID == "" {
		ret.Recovery.StepID = snapshot.Step.ID
	}
}

func currentActionLabelFromWorkspaceSnapshot(snapshot readmodelcore.WorkspaceSnapshot) string {
	action := ""
	if snapshot.GapBaseline != nil {
		action = strings.TrimSpace(snapshot.GapBaseline.NextAction.Kind)
	}
	focus := ""
	if snapshot.Acceptance.CurrentFocus != nil {
		focus = strings.TrimSpace(snapshot.Acceptance.CurrentFocus.ID)
	}
	if focus == "" && snapshot.GapBaseline != nil {
		focus = strings.TrimSpace(stringValue(snapshot.GapBaseline.CurrentFocus, "id"))
	}
	switch {
	case action != "" && focus != "":
		return action + " " + focus
	case action != "":
		return action
	case focus != "":
		return "focus " + focus
	default:
		return ""
	}
}
