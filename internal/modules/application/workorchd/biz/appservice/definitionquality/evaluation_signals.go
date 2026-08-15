package definitionquality

import "strings"

func measurableDoDItemCount(items []string) int {
	var ret int
	for _, item := range items {
		itemNorm := normalizeText(item)
		if containsAny(itemNorm, measurableDoDKeywords) {
			ret++
		}
	}
	return ret
}

func applyGoalQualitySignals(check *Check, goalText string, goalTextNorm string) {
	if strings.TrimSpace(goalText) == "" {
		check.ExecutionScore -= 70
		check.addIssue("goal_missing", "error", "goal", "缺少 Goal，无法判断执行目标。", "补充一个明确、单一的目标描述。")
	}
	if runeCount(goalText) > 0 && runeCount(goalText) < 8 {
		check.ExecutionScore -= 15
		check.addIssue("goal_too_short", "warning", "goal", "Goal 过短，可能缺少上下文。", "补充目标对象、范围和期望结果。")
	}
	if looksQuestionOnly(goalTextNorm) && !containsAny(goalTextNorm, actionKeywords) {
		check.ExecutionScore -= 10
		check.addIssue("goal_ambiguous", "warning", "goal", "Goal 更像问题陈述，执行动作不够明确。", "在 Goal 中补充要执行的动作和期望交付。")
	}
}

func applyDoDQualitySignals(check *Check, dodItems []string, measurableCount int) {
	if len(dodItems) == 0 {
		check.ExecutionScore -= 30
		check.VerificationScore -= 80
		check.addIssue("dod_missing", "error", "dod", "缺少 DoD，无法验证完成标准。", "补充可验证的 DoD 条目。")
	}
	if len(dodItems) > 0 && len(dodItems) < 2 {
		check.VerificationScore -= 15
		check.addIssue("dod_too_few_items", "warning", "dod", "DoD 条目较少，可能无法覆盖验收范围。", "增加至少一条验证性条目（例如测试、输出或证据要求）。")
	}
	for _, item := range dodItems {
		if runeCount(item) < 6 {
			check.VerificationScore -= 5
			check.addIssue("dod_item_too_short", "warning", "dod_item", "DoD 条目过短，可验证性不足。", "补充可观察结果或验证方式。")
		}
	}

	if len(dodItems) > 0 && measurableCount == 0 {
		check.VerificationScore -= 50
		check.addIssue("dod_not_measurable", "error", "dod", "DoD 缺少可验证信号（命令、输出、证据或度量）。", "补充 build/test/lint、交付物或 evidenceRefs 要求。")
		check.addMissingConstraint("可验证证据")
	}
	if len(dodItems) > 0 && measurableCount > 0 && measurableCount*2 < len(dodItems) {
		check.VerificationScore -= 20
		check.addIssue("dod_measurability_low", "warning", "dod", "DoD 中可验证条目占比偏低。", "将更多条目改为可执行、可验证的形式。")
	}
}

func applyCoverageConstraintSignals(check *Check, combinedText string) {
	if !containsAny(combinedText, scopeBoundaryKeywords) {
		check.ExecutionScore -= 10
		check.addMissingConstraint("范围边界")
		check.addSuggestion("补充范围边界，说明包含与不包含内容。")
	}
	if !containsAny(combinedText, riskConstraintKeywords) {
		check.VerificationScore -= 10
		check.addMissingConstraint("风险与限制")
		check.addSuggestion("补充风险、限制或已知依赖假设，便于验收判定。")
	}
}
