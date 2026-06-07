package setupdraftcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/doditems"
)

func NormalizeDraft(draft Draft) Draft {
	draft.GoalText = strings.TrimSpace(draft.GoalText)
	draft.DoDItems = doditems.Normalize(draft.DoDItems)
	draft.Assumptions = CompactStrings(draft.Assumptions)
	draft.ClarifyingQuestions = CompactStrings(draft.ClarifyingQuestions)
	draft.Warnings = CompactStrings(draft.Warnings)
	draft.Confidence = NormalizedConfidence(draft.Confidence)
	draft.GenerationSummary = strings.TrimSpace(draft.GenerationSummary)
	if (draft.GoalText == "" || len(draft.DoDItems) == 0) && len(draft.ClarifyingQuestions) == 0 && len(draft.Warnings) == 0 {
		draft.ClarifyingQuestions = []string{"请补充目标范围、验收条件或关键约束，以便生成完整的 Goal 和 DoD 草案。"}
	}
	draft.NeedsClarification = draft.GoalText == "" || len(draft.DoDItems) == 0 || len(draft.ClarifyingQuestions) > 0
	return draft
}

func MergeDraft(base Draft, update Draft) Draft {
	base = NormalizeDraft(base)
	updateHasGoal := strings.TrimSpace(update.GoalText) != ""
	updateHasDoD := len(update.DoDItems) > 0
	updateHasAssumptions := len(update.Assumptions) > 0
	updateHasWarnings := len(update.Warnings) > 0
	updateHasSummary := strings.TrimSpace(update.GenerationSummary) != ""
	update = NormalizeDraft(update)
	merged := update
	if !updateHasGoal {
		merged.GoalText = base.GoalText
	}
	if !updateHasDoD {
		merged.DoDItems = append([]string(nil), base.DoDItems...)
	}
	if !updateHasAssumptions {
		merged.Assumptions = append([]string(nil), base.Assumptions...)
	}
	if !updateHasWarnings {
		merged.Warnings = append([]string(nil), base.Warnings...)
	}
	if !updateHasSummary {
		merged.GenerationSummary = base.GenerationSummary
	}
	merged.TemplateContextUsed = update.TemplateContextUsed || base.TemplateContextUsed
	merged.PreviousDraftApplied = update.PreviousDraftApplied || base.PreviousDraftApplied
	return NormalizeDraft(merged)
}

func RevisionBaseline(draft Draft) Draft {
	draft = NormalizeDraft(draft)
	draft.ClarifyingQuestions = nil
	return NormalizeDraft(draft)
}

func HasContent(draft Draft) bool {
	return strings.TrimSpace(draft.GoalText) != "" ||
		len(draft.DoDItems) > 0 ||
		len(draft.Assumptions) > 0 ||
		len(draft.ClarifyingQuestions) > 0 ||
		len(draft.Warnings) > 0 ||
		strings.TrimSpace(draft.GenerationSummary) != ""
}
