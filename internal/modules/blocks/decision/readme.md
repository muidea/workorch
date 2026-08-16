# Decision Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-16

## 功能说明

`blocks/decision` 管理 DecisionTicket 与 DecisionResolution 聚合。当前跨 owner 合同提供 ticket/resolution 查询和
删除、普通 ticket 的幂等解决，以及 Runtime governance ticket 的确定性创建与 resolution 提交；不暴露通用 ticket
create/update/supersede state setter。

## 组件交互及合同

- Ticket 与 Resolution 的实际 command/query 由 `blocks/decision/pkg/events` 定义，并返回
  `DecisionCommandResult`；Runtime governance open 使用同一 owner 定义的 concrete command/result。
- Runtime governance ticket 创建发布 `TicketChangedData`，解决成功发布 `TicketResolvedData`；不存在通用
  “任意 ticket 状态变化”通知。
- `kernel/governance` 和 `application/workorchd` 通过 EventHub 查询或解决 ticket；Runtime governance ticket 只能由
  Orchestrator 的 sealed request 经 Decision owner 创建。
- `blocks/run` 与 `kernel/orchestrator` 使用 ticket scope 表达治理事实；Orchestrator 不恢复 Run，Run owner消费其 immutable wait/continuation fact 后在自身 reducer 中迁移 `blocked_on_decision`。
- Decision 需要记录 Orchestrator evidence 时，发送本 block 定义的 `RecordDecisionEvidenceCommand`；Orchestrator 幂等写入自己的 EvidenceRecord 后返回 `RecordDecisionEvidenceResult`。
- Runtime governed dispatch 由 Orchestrator 通过其定义的 `OpenRuntimeGovernanceRequestCommand/Result` 投递；本 block 导入该 typed 合同并确保创建确定性 DecisionTicket，不接收跨组件 map context。
- `runtime_governance` ticket resolved 后，本 block 通过 Orchestrator 定义的 `CommitRuntimeGovernanceTicketResolutionCommand` 提交 resolution identity、scope、outcome 与 summary；Orchestrator 仅提交 evidence/fact，不由 application scheduler 猜测 authority snapshot 或 dispatch、更不直接恢复 Run。
- EventHub command 与应用调用只能提交不可变 resolution/proposal fact；ticket 及 Runtime governance proposal 的状态选择、时间戳和 repository update 只允许经 Decision-private lifecycle event/reducer 完成。

## Owner 内部职责边界

- `owner.go` 只保留 Decision Biz 的构造、订阅与生命周期；`command_router.go` 是普通 Decision command 的唯一入口，使用静态穷尽分类器将 Topic 委派到 ticket query/mutation 或 resolution handler，未知 Topic fail closed。
- `ticket_commands.go` 只处理 ticket 查询与删除，`resolution_commands.go` 只处理 resolution 查询、删除和普通解决事务；二者共享 `decision_scope.go` 的 workspace/run/step fence，不建立第二个 Owner 或 repository facade。
- `runtime_governance_adapter.go` 只负责 Runtime governance request、proposal 提交和 disposition 映射；proposal 状态仍由 `lifecycle_internal_event.go` 的 Decision-private reducer 选择，reconciliation 仍重放同一 adapter。
- `decision_notification.go` 只在 Owner transaction 成功后发布 ticket lifecycle 通知；resolution evidence、repository 写入和通知的既有顺序不变。

## 相关约束

- Ticket、Resolution 和 evidence command 必须携带 workspace/run/ticket scope，跨 workspace 重复 ID 不得误解析。
- 本 block 只访问 DecisionTicket 与 DecisionResolution repository，不得直接访问 EvidenceRecord、Run 或 Workspace repository。
- Resolution 必须符合 ticket 当前状态和允许的 resolution type；重复解决必须幂等或返回冲突。
- Resolution 与 ticket 使用同一 Decision owner 的可恢复两阶段写入；若 resolution 已持久化而 ticket 更新中断，只能由同一 private recovery event 修复 ticket，外部组件不得代写状态。
- Decision 只表达治理选择，不直接修改 Run、Result 或 Orchestrator 收敛状态。
- Runtime ticket 只拥有治理选择状态；authority snapshot、dispatch 与 ticket 的正式绑定由 Orchestrator 随 Run owner 状态持久化。
- 普通 Decision resolution 与 `runtime_governance` resolution 都不能由 Application 自动触发 Resume；后者只能走
  typed ticket-resolution command 回到 Orchestrator，Run continuation 仍由 persisted governance fact 与 Run-private reducer 决定。
