package definitionquality

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func Evaluate(
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	goalText string,
	dodItems []string,
) Check {
	ret := Check{
		WorkspaceID:       string(workspaceID),
		GoalID:            string(goalID),
		DoDID:             string(dodID),
		GoalText:          goalText,
		DoDItems:          append([]string(nil), dodItems...),
		ExecutionScore:    100,
		VerificationScore: 100,
	}

	goalTextNorm := normalizeText(goalText)
	dodCombinedNorm := normalizeText(strings.Join(dodItems, " "))

	measurableCount := measurableDoDItemCount(dodItems)
	applyGoalQualitySignals(&ret, goalText, goalTextNorm)
	applyDoDQualitySignals(&ret, dodItems, measurableCount)
	applyCoverageConstraintSignals(&ret, goalTextNorm+" "+dodCombinedNorm)

	ret.ExecutionScore = clampScore(ret.ExecutionScore)
	ret.VerificationScore = clampScore(ret.VerificationScore)
	ret.Executable = ret.ExecutionScore >= 60 && !ret.hasError("goal_missing")
	ret.Verifiable = ret.VerificationScore >= 60 && len(dodItems) > 0 && measurableCount > 0 && !ret.hasError("dod_missing") && !ret.hasError("dod_not_measurable")
	if ret.Executable && ret.Verifiable {
		ret.Status = "ready"
	} else {
		ret.Status = "needs_refinement"
	}
	if len(ret.Suggestions) == 0 && ret.Status != "ready" {
		ret.addSuggestion("补充 Goal / DoD 的执行边界和验收证据，再重新发起预检。")
	}
	return ret
}
