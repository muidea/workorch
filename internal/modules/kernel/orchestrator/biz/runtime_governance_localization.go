package orchestrator

import "fmt"

func localizedRuntimeRepairExhaustionReason(language string, attempts int, err error) string {
	reason := fmt.Sprintf("runtime repair attempts exhausted after %d attempts", attempts)
	if prefersChinese(language) {
		reason = fmt.Sprintf("runtime 自动修复尝试已耗尽（共 %d 次）", attempts)
	}
	if err == nil {
		return reason
	}
	if prefersChinese(language) {
		return fmt.Sprintf("%s：%s", reason, err.Error())
	}
	return fmt.Sprintf("%s: %s", reason, err.Error())
}

func localizedRuntimeRepairExhaustionQuestion(language string) string {
	if prefersChinese(language) {
		return "自动运行时修复尝试已耗尽。请补充缺失的实现指导，选择重新规划，或拒绝当前路径。"
	}
	return "Automatic runtime repair attempts were exhausted. Provide missing implementation guidance, choose replan, or reject the current path."
}

func localizedDuplicateRuntimeFailureReason(language string, signature string, err error) string {
	reason := fmt.Sprintf("duplicate runtime failure signature detected: %s", signature)
	if prefersChinese(language) {
		reason = fmt.Sprintf("检测到重复的运行时失败签名：%s", signature)
	}
	if err == nil {
		return reason
	}
	if prefersChinese(language) {
		return fmt.Sprintf("%s：%s", reason, err.Error())
	}
	return fmt.Sprintf("%s: %s", reason, err.Error())
}

func localizedDuplicateRuntimeFailureQuestion(language string) string {
	if prefersChinese(language) {
		return "同一运行时失败在修复后再次发生。请提供不同的处理指导，选择重新规划，或拒绝当前路径。"
	}
	return "The same runtime failure happened again after repair. Provide different guidance, choose replan, or reject the current path."
}

func localizedProviderRetryExhaustionReason(language string, attempts int, err error) string {
	reason := fmt.Sprintf("LLM provider retry attempts exhausted after %d attempts", attempts)
	if prefersChinese(language) {
		reason = fmt.Sprintf("LLM provider 重试尝试已耗尽（共 %d 次）", attempts)
	}
	if err == nil {
		return reason
	}
	if prefersChinese(language) {
		return fmt.Sprintf("%s：%s", reason, err.Error())
	}
	return fmt.Sprintf("%s: %s", reason, err.Error())
}

func localizedProviderRetryExhaustionQuestion(language string) string {
	if prefersChinese(language) {
		return "LLM provider 多次重试仍失败。请补充恢复指导、稍后继续、选择重新规划，或拒绝当前路径。"
	}
	return "LLM provider retries were exhausted. Provide recovery guidance, continue later, choose replan, or reject the current path."
}

func localizedPatchApprovalReason(language string) string {
	if prefersChinese(language) {
		return "修改工作区文件前需要审批补丁。"
	}
	return "Patch requires approval before workspace files can be modified."
}

func localizedPatchApprovalAction(language string) string {
	if prefersChinese(language) {
		return "审批应用待处理的工作区补丁。"
	}
	return "Approve applying the pending workspace patch."
}

func localizedPatchRevertApprovalReason(language string) string {
	if prefersChinese(language) {
		return "回滚补丁并修改工作区文件前需要审批。"
	}
	return "Patch revert requires approval before workspace files can be modified."
}

func localizedPatchRevertApprovalAction(language string) string {
	if prefersChinese(language) {
		return "审批回滚待处理的工作区补丁。"
	}
	return "Approve reverting the pending workspace patch."
}
