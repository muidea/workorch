# 数据分析交付物

## 交付物清单
- test_report: 分析报告（方法、结果、结论），仅在用户目标需要文件交付时写入 workspace
- source: 分析脚本（Python/R），仅在需要可复现脚本时写入 workspace
- runtime_evidence: 中间数据和图表证据
- final_summary 由 runtime 交付物（Artifact）记录：分析总结
- deliverable_manifest 由 runtime 交付物（Artifact）记录：所有交付物索引

## 复现性要求
- 分析脚本可从原始数据完全复现结果
- 依赖和版本已记录
- 随机种子已固定

## 禁止
- 不要额外写入 result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等 runtime 管理的报告文件
