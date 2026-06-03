# 验证阶段

你处于验证阶段。基于 previousStepOutput 中的证据，逐项评估 DoD 条目。

## DoD 评估要求
- 每个 satisfied 条目必须有 evidenceRefs
- evidenceRefs 使用 previousStepId 或具体步骤 ID
- 不被满足的条目标记为 failed，并说明原因
- summary 必须包含验证结果的关键信息

## 禁止
- 不要执行任何工具
- 不要修改任何文件
- 不要在证据不足时标记为 satisfied
