package localizationcore

func RecoveryNextAction(language string) string {
	return "resume_run"
}

func BlockerReason(language string, kind string, state string) string {
	if PrefersChinese(language) {
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
