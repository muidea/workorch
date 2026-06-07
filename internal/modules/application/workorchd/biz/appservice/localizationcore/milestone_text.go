package localizationcore

func MilestoneTitle(language string, milestoneID string) string {
	if PrefersChinese(language) {
		switch milestoneID {
		case "run_created":
			return "运行（Run）已创建"
		case "execution_steps":
			return "执行步骤"
		case "governance":
			return "决策与审批治理"
		case "validation":
			return "验证"
		case "formal_result":
			return "结果（Result）/交付物（Artifact）"
		}
	}
	switch milestoneID {
	case "run_created":
		return "Run created"
	case "execution_steps":
		return "Execution steps"
	case "governance":
		return "Decision and approval governance"
	case "validation":
		return "Validation"
	case "formal_result":
		return "Result / Artifact"
	default:
		return milestoneID
	}
}
