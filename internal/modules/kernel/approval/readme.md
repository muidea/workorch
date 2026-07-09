# Approval Module

## 功能定位

`kernel/approval` 是 Approval 与 approval decision result 的 owner module。它负责需要授权的 runtime action、外部访问或风险动作的审批对象生命周期。

## 拥有边界

- 拥有 Approval 的 pending/approved/rejected 状态与 owner command/query。
- Approval 表达授权结果，不表达业务 evidence 是否满足。
- 不创建 accepted Result，也不直接推进 coverage。

## 主要交互

- 被 `kernel/orchestrator`、`kernel/governance` 或 runtime block approval gate 创建 pending approval。
- 与 `kernel/run` 协作表达 `blocked_on_approval` 状态。
- approval resolution 被 `kernel/orchestrator` 或相关 block continuation 消费。
- 被 `application/workorchd`、Web/TUI/API 读取和提交 approve/reject 命令。

## 组件交互合同

- Input contract: approval command 必须包含 approvalId，并在 runtime 场景携带 workspaceId/runId/stepId/action。
- Output contract: command response 必须返回 pending/approved/rejected 状态和授权 decision payload。
- Orchestrator/block contract: approval 只授权动作执行；后续 evidence/result 仍由 orchestrator gate 判定。
- Run contract: 只有 pending approval 正式创建成功后，Run 才能进入 `blocked_on_approval`。
- Application contract: 无 scope approval 操作只能命中唯一 open approval，否则返回 conflict。

## 已知缺口

- 当前盘点未发现 Approval module 自身职责缺失；批量审批、重复 approvalId 和跨 workspace scope 仍需要持续 pairwise 回归。

## 边界约束

- runtime approval 必须携带 workspaceId/runId/stepId/action scope。
- 跨 workspace 重复 approval ID 读取必须有 scope 或返回 conflict。
- 授权通过不等于验证通过。
