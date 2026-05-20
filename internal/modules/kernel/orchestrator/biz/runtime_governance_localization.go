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
		return "自动运行时修复尝试已耗尽。请补充缺失的处理指导，选择重新规划，或拒绝当前路径。"
	}
	return "Automatic runtime repair attempts were exhausted. Provide missing recovery guidance, choose replan, or reject the current path."
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
		return "WorkOrch runtime LLM provider 多次重试仍失败。请先在 WorkOrch 系统配置中修复或切换 runtime LLM provider，或稍后重试；不要在工单答案中粘贴 API Key。配置修复完成后再回复继续、选择重新规划，或拒绝当前路径。"
	}
	return "WorkOrch runtime LLM provider retries were exhausted. Fix or switch the runtime LLM provider in WorkOrch system configuration first, or retry later; do not paste API keys into the ticket answer. After configuration is fixed, continue, choose replan, or reject the current path."
}

func localizedContextIntegrityGateReason(language string, action string, reason string, err error) string {
	if action == "" {
		action = "unknown"
	}
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if reason == "" {
		reason = "context integrity gate failed"
	}
	if prefersChinese(language) {
		return fmt.Sprintf("runtime 上下文准入失败（%s）：%s", action, reason)
	}
	return fmt.Sprintf("runtime context gate failed (%s): %s", action, reason)
}

func localizedContextIntegrityGateQuestion(language string) string {
	if prefersChinese(language) {
		return "WorkOrch runtime 无法构造满足准入要求的 LLM 上下文。请补充缺失信息、选择重新规划，或终止当前路径；不要通过普通状态变更输出修复该 runtime 上下文问题。"
	}
	return "WorkOrch runtime cannot build an LLM context that satisfies the gate. Provide the missing information, choose replan, or terminate the path; do not fix this runtime context issue through a normal state-changing output."
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
