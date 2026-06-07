package localizationcore

func StepCompletionSummary(language string) string {
	if PrefersChinese(language) {
		return "runtime 步骤已完成"
	}
	return "runtime step completed"
}

func WorkspaceProgressText(language string, key string) string {
	if PrefersChinese(language) {
		switch key {
		case "no_active_run":
			return "工作空间尚无活跃运行（Run）"
		case "active_run":
			return "工作空间存在活跃运行（Run）投影"
		case "workspace_defined":
			return "工作空间（Workspace）已定义"
		case "runs":
			return "运行（Run）"
		case "results":
			return "结果（Result）"
		case "artifacts":
			return "交付物（Artifact）"
		case "context_summary":
			return "无活跃运行（Run）的工作空间上下文包"
		case "workspace":
			return "工作空间（Workspace）"
		case "task_progress":
			return "任务进度"
		}
	}
	switch key {
	case "no_active_run":
		return "workspace has no active Run"
	case "active_run":
		return "workspace has an active Run projection"
	case "workspace_defined":
		return "Workspace defined"
	case "runs":
		return "Runs"
	case "results":
		return "Results"
	case "artifacts":
		return "Artifacts"
	case "context_summary":
		return "workspace context bundle without active Run"
	case "workspace":
		return "Workspace"
	case "task_progress":
		return "Task progress"
	default:
		return key
	}
}
