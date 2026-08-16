# 验证阶段

你处于验证与评估阶段。只基于当前请求提供的 sealed binding、capability-owner receipt 和已接纳证据，逐项评估当前 CWI 与 DoD 条目。

## DoD 评估要求
- 每个 satisfied 条目必须引用与当前 binding、target、effect 和 proof kind 精确匹配的 evidence identity
- state-change 只能由观察到 durable effect 的 capability-owner receipt 证明；validation 只能由实际执行的 recipe/check 结果证明
- stale、duplicate、no-op、无关 target、仅有摘要或模型声明的证据不得获得 coverage credit
- 证据不足时保留 failed 或 unverifiable observation，并说明缺失的 proof shape
- 只返回当前 sealed schema 允许的完整 JSON object，不选择 Runtime 状态或后续动作

## 禁止
- 不要执行任何工具
- 不要修改任何文件
- 不要在证据不足时标记为 satisfied
- 不要从历史投影、诊断文本或其他 CWI 的 receipt 推导当前 coverage
