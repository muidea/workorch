package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
)

func detectSystemLocale() string {
	return dialoglanguage.FromEnvironment()
}

func languageNameFromLocale(locale string) string {
	return dialoglanguage.NormalizeTag(locale)
}

func prefersChinese(language string) bool {
	return dialoglanguage.PrefersChinese(language)
}

func defaultDialogLanguage() string {
	return detectSystemLocale()
}

func defaultDialogLanguageFromContext(ctx context.Context) string {
	if cfg, err := loadConfigFromContext(ctx); err == nil {
		if lang := dialoglanguage.NormalizeTag(cfg.Server.DefaultDialogLanguage); lang != "" {
			return lang
		}
	}
	return defaultDialogLanguage()
}

func localizedMilestoneTitle(language string, milestoneID string) string {
	if prefersChinese(language) {
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

func localizedCreatedRunSummary(language string) string {
	if prefersChinese(language) {
		return "运行（Run）已创建，尚无执行证据"
	}
	return "run created with no execution evidence yet"
}

func localizedRetentionPolicy(language string) string {
	if prefersChinese(language) {
		return "保留恢复点、阻塞项、里程碑、最新证据摘要及结果（Result）/交付物（Artifact）引用；需要时按源引用重新加载完整证据"
	}
	return "retain recovery, blockers, milestones, latest evidence summaries, and Result/Artifact refs; reload full evidence by source refs when needed"
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
			return "审批已通过；恢复运行（Run）执行"
		case kind == "approval" && state == "rejected":
			return "审批已拒绝"
		case kind == "decision" && state == "resolved":
			return "决策已解决；恢复运行（Run）执行"
		default:
			return "阻塞项已解决；恢复运行（Run）执行"
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
