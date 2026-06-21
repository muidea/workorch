package governancecore

func NormalizeRuntimeRepairRecoveryAction(value string) string {
	text := NormalizeRepairResolutionText(value)
	switch {
	case text == "":
		return ""
	case ContainsAnyRepairResolutionCue(text, []string{
		"replan", "replanning", "重新规划", "重新计划", "重做计划", "换方案", "重构计划",
	}):
		return "replan"
	case ContainsAnyRepairResolutionCue(text, []string{
		"terminate", "stop", "abort", "cancel", "reject", "decline", "refuse", "deny",
		"终止", "停止", "取消", "拒绝", "驳回", "否决", "不继续", "不执行",
	}):
		return "terminate"
	case ContainsAnyRepairResolutionCue(text, []string{
		"retry_same_step", "retrysamestep", "retry", "rerun", "resume", "proceed", "goahead", "again",
		"重试", "重新尝试", "再次尝试", "继续", "继续执行", "重新执行", "恢复执行", "再试", "同意", "确认", "可以", "好的",
	}):
		return "retry_same_step"
	case ContainsAnyRepairResolutionCue(text, []string{
		"provide_information", "provideinfo", "information", "guidance", "instruction", "context",
		"补充信息", "提供信息", "实现指导", "修复指导", "上下文", "说明",
	}):
		return "provide_information"
	default:
		return ""
	}
}
