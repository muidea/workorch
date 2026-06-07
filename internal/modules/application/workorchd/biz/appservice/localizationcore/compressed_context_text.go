package localizationcore

func CreatedRunSummary(language string) string {
	if PrefersChinese(language) {
		return "运行（Run）已创建，尚无执行证据"
	}
	return "run created with no execution evidence yet"
}

func RetentionPolicy(language string) string {
	if PrefersChinese(language) {
		return "保留恢复点、阻塞项、里程碑、最新证据摘要及结果（Result）/交付物（Artifact）引用；需要时按源引用重新加载完整证据"
	}
	return "retain recovery, blockers, milestones, latest evidence summaries, and Result/Artifact refs; reload full evidence by source refs when needed"
}
