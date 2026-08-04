package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyTaskProgressCurrentStep(ret *projectioncontract.TaskProgress, snapshot readmodelcore.RunSnapshot) {
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = llmcommon.CanonicalRuntimeBlockKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		ret.Recovery.StepID = snapshot.Step.ID
	}
	ret.CurrentActionLabel = currentActionLabelFromRunSnapshot(snapshot)
	ret.DiagnosticLineage = ret.CurrentStepID
	if result := TerminalTaskProgressResult(snapshot); result != nil {
		ret.CurrentStepID = firstNonEmptyTimelineString(result.SourceRef, result.ID)
		ret.CurrentStepKind = "result/" + strings.TrimSpace(firstNonEmptyTimelineString(result.SourceType, "terminal"))
		ret.CurrentStepState = result.State
		ret.Recovery.StepID = ret.CurrentStepID
		ret.DiagnosticLineage = ret.CurrentStepID
	}
}

func currentActionLabelFromRunSnapshot(snapshot readmodelcore.RunSnapshot) string {
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
