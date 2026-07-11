# Decision Stateful Block

## 功能说明

`blocks/decision` 管理 DecisionTicket 与 DecisionResolution 聚合，负责 ticket 的创建、查询、更新、解决、supersede 和 resolution 生命周期。

## 组件交互及合同

- Ticket 与 Resolution command/query 由 `blocks/decision/pkg/events` 定义，并返回 `DecisionCommandResult`。
- Ticket 状态变化发布 `TicketChangedData`；解决成功发布 `TicketResolvedData`。
- `kernel/governance` 和 `application/workorchd` 通过 EventHub 创建、查询或解决 ticket。
- `kernel/run` 与 `kernel/orchestrator` 使用 ticket scope 表达和恢复 `blocked_on_decision`。
- Decision 需要记录 Orchestrator evidence 时，发送本 block 定义的 `RecordDecisionEvidenceCommand`；Orchestrator 幂等写入自己的 EvidenceRecord 后返回 `RecordDecisionEvidenceResult`。

## 相关约束

- Ticket、Resolution 和 evidence command 必须携带 workspace/run/ticket scope，跨 workspace 重复 ID 不得误解析。
- 本 block 只访问 DecisionTicket 与 DecisionResolution repository，不得直接访问 EvidenceRecord、Run 或 Workspace repository。
- Resolution 必须符合 ticket 当前状态和允许的 resolution type；重复解决必须幂等或返回冲突。
- Decision 只表达治理选择，不直接修改 Run、Result 或 Orchestrator 收敛状态。
