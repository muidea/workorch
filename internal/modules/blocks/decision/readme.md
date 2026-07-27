# Decision Stateful Block

## 功能说明

`blocks/decision` 管理 DecisionTicket 与 DecisionResolution 聚合，负责 ticket 的创建、查询、更新、解决、supersede 和 resolution 生命周期。

## 组件交互及合同

- Ticket 与 Resolution command/query 由 `blocks/decision/pkg/events` 定义，并返回 `DecisionCommandResult`。
- Ticket 状态变化发布 `TicketChangedData`；解决成功发布 `TicketResolvedData`。
- `kernel/governance` 和 `application/workorchd` 通过 EventHub 创建、查询或解决 ticket。
- `blocks/run` 与 `kernel/orchestrator` 使用 ticket scope 表达治理事实；Orchestrator 不恢复 Run，Run owner消费其 immutable wait/continuation fact 后在自身 reducer 中迁移 `blocked_on_decision`。
- Decision 需要记录 Orchestrator evidence 时，发送本 block 定义的 `RecordDecisionEvidenceCommand`；Orchestrator 幂等写入自己的 EvidenceRecord 后返回 `RecordDecisionEvidenceResult`。
- Runtime V2 governed dispatch 由 Orchestrator 通过其定义的 `OpenRuntimeGovernanceRequestCommand/Result` 投递；本 block 导入该 typed 合同并确保创建确定性 DecisionTicket，不接收跨组件 map context。
- `runtime_v2_governance` ticket resolved 后，本 block 通过 Orchestrator 定义的 `CommitRuntimeGovernanceTicketResolutionCommand` 提交 resolution identity、scope、outcome 与 summary；Orchestrator 仅提交 evidence/fact，不由 application scheduler 猜测 authority snapshot 或 dispatch、更不直接恢复 Run。
- Runtime V1 retirement apply 通过 typed update command 将关联 V1 Run 的 open ticket 标记为 `superseded`，并发布 ticket superseded 通知。

## 相关约束

- Ticket、Resolution 和 evidence command 必须携带 workspace/run/ticket scope，跨 workspace 重复 ID 不得误解析。
- 本 block 只访问 DecisionTicket 与 DecisionResolution repository，不得直接访问 EvidenceRecord、Run 或 Workspace repository。
- Resolution 必须符合 ticket 当前状态和允许的 resolution type；重复解决必须幂等或返回冲突。
- Decision 只表达治理选择，不直接修改 Run、Result 或 Orchestrator 收敛状态。
- Runtime V2 ticket 只拥有治理选择状态；authority snapshot、dispatch 与 ticket 的正式绑定由 Orchestrator 随 Run owner 状态持久化。
- 普通 Decision 可以继续使用 application continuation；`runtime_v2_governance` 只能走 typed ticket-resolution command，禁止同时触发通用 Resume。
- retirement 不创建 resolution，也不得触发 Resume 或 successor Run。
