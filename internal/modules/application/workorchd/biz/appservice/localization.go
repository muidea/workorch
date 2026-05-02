package appservice

import (
	"os"
	"strings"
)

func detectSystemLocale() string {
	for _, env := range []string{"LANG", "LC_ALL", "LC_MESSAGES"} {
		if locale := os.Getenv(env); locale != "" {
			if lang := languageNameFromLocale(locale); lang != "" {
				return lang
			}
		}
	}
	return ""
}

func languageNameFromLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	locale = strings.SplitN(locale, ".", 2)[0]
	if strings.HasPrefix(locale, "zh") {
		return "zh-CN"
	}
	if strings.HasPrefix(locale, "ja") {
		return "ja"
	}
	if strings.HasPrefix(locale, "ko") {
		return "ko"
	}
	return ""
}

func prefersChinese(language string) bool {
	lang := strings.ToLower(strings.TrimSpace(language))
	return lang == "zh-cn" || lang == "zh" || lang == "chinese" || strings.HasPrefix(lang, "zh-")
}

func localizedMilestoneTitle(language string, milestoneID string) string {
	if prefersChinese(language) {
		switch milestoneID {
		case "run_created":
			return "Run 已创建"
		case "execution_steps":
			return "执行步骤"
		case "governance":
			return "决策与审批治理"
		case "validation":
			return "验证"
		case "formal_result":
			return "正式交付物"
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
		return "Formal result"
	default:
		return milestoneID
	}
}

func localizedCreatedRunSummary(language string) string {
	if prefersChinese(language) {
		return "Run 已创建，尚无执行证据"
	}
	return "run created with no execution evidence yet"
}

func localizedRetentionPolicy(language string) string {
	if prefersChinese(language) {
		return "保留恢复点、阻塞项、里程碑、最新证据摘要及正式交付物引用；需要时按源引用重新加载完整证据"
	}
	return "retain recovery, blockers, milestones, latest evidence summaries, and formal result refs; reload full evidence by source refs when needed"
}

func localizedRecoveryNextAction(language string) string {
	if prefersChinese(language) {
		return "继续执行_run"
	}
	return "continue_run"
}

func localizedBlockerReason(language string, kind string, state string) string {
	if prefersChinese(language) {
		switch {
		case kind == "approval" && state == "approved":
			return "审批已通过；恢复 Run 执行"
		case kind == "approval" && state == "rejected":
			return "审批已拒绝"
		case kind == "decision" && state == "resolved":
			return "决策已解决；恢复 Run 执行"
		default:
			return "阻塞项已解决；恢复 Run 执行"
		}
	}
	switch {
	case kind == "approval" && state == "approved":
		return "approval was resolved; resume run execution"
	case kind == "approval" && state == "rejected":
		return "approval was rejected"
	case kind == "decision" && state == "resolved":
		return "decision was resolved; resume run execution"
	default:
		return "blocker was resolved; resume run execution"
	}
}

func localizedStepCompletionSummary(language string) string {
	if prefersChinese(language) {
		return "runtime 步骤已完成"
	}
	return "runtime step completed"
}
