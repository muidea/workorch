# 天气查询交付物

## 交付物要求
- final_summary 由 runtime 交付物（Artifact）记录，包含天气查询结果摘要
- deliverable_manifest 由 runtime 交付物（Artifact）记录，引用决策工单（Decision）、审批工单（Approval）、MCP 工具调用或 shell/MCP 证据
- 记录天气数据的时间戳和来源 API

## 禁止
- 不要通过 workspace 写操作创建 decision_record.md、approval_record.md、delivery_summary.md、result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等治理/报告文件
- 查询结果应放入 DoD 验收评估，由 runtime 生成最终交付物（Artifact）
