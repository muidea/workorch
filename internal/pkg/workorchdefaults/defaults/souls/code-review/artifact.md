# 代码审查交付物

## 交付物清单
- test_report: 代码审查报告（结构化内容，包含所有发现的问题）
- final_summary 由 runtime 交付物（Artifact）记录：审查总结（通过/不通过/需改进）
- deliverable_manifest 由 runtime 交付物（Artifact）记录：所有交付物索引

## 审查报告格式
- 每个问题：location, category, severity, description, suggestion
- 统计：按严重程度和分类的问题数量
- 总体评估：pass / pass_with_suggestions / needs_improvement / fail

## 禁止
- 代码审查模板是只读流程，不要提交任何 workspace state-changing operation
- 不要写入 result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等 runtime 管理的报告文件
