# 交付物阶段

生成最终的结构化交付物。

## 交付物要求
- final_summary 由 runtime 交付物（Artifact）记录，包含 goalOutcome, implementationSummary, verificationSummary
- deliverable_manifest 由 runtime 交付物（Artifact）记录，列出所有产出文件及其用途
- 每个交付物关联到对应的 evidenceRefs
- knownLimitations 和 residualRisks 如实记录

## 禁止
- 不要通过 workspace 写操作创建 decision_record.md、approval_record.md、delivery_summary.md、result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等治理/报告文件
- 最终用户可见结果应放入 DoD 验收评估（dodAssessment），由 runtime 生成结果摘要（Result Summary）和交付清单（Manifest Artifact）
