package definitionquality

var actionKeywords = []string{
	"implement", "add", "fix", "update", "refactor", "generate", "migrate", "optimize", "review", "document",
	"实现", "新增", "修复", "更新", "重构", "生成", "迁移", "优化", "审查", "编写",
}

var measurableDoDKeywords = []string{
	"test", "build", "lint", "diff", "artifact", "report", "evidence", "metric", "assert", "verify", "command", "output",
	"测试", "构建", "编译", "静态检查", "校验", "验证", "证据", "报告", "交付物", "输出", "命令",
}

var scopeBoundaryKeywords = []string{
	"scope", "boundary", "exclude", "only", "not include", "out of scope",
	"范围", "边界", "仅", "只", "不包含", "不修改", "排除",
}

var riskConstraintKeywords = []string{
	"risk", "limitation", "dependency", "assumption", "constraint",
	"风险", "限制", "依赖", "假设", "约束",
}
