# Approval Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/approval` 管理 Approval 聚合。当前跨 owner 合同提供查询、列表、删除，以及已有 pending Approval 的
幂等 approve/reject；Approval 创建不作为公开 EventHub command，调用方也不能提交目标状态或完整 entity。

## 组件交互及合同

- Approval 的实际 command/query 由 `blocks/approval/pkg/events` 定义，并返回 `ApprovalCommandResult`。
- pending Approval 成功解决后由本 block在唯一 `TopicApprovalResolved` 发布 `ApprovalChangedData`；不存在公开的
  create/update/supersede notification family。
- `kernel/governance`、`kernel/orchestrator` 和 `application/workorchd` 通过 EventHub 查询或解决已有 Approval。
- Web capability 在外部访问授权场景查询 Approval；其他 capability owner 消费 sealed governance grant 或返回治理需求，不直接查询 Approval service/repository。
- Approval 需要记录 Orchestrator evidence 时，发送本 block 定义的 `RecordApprovalEvidenceCommand`，并接收 `RecordApprovalEvidenceResult`。
- approve/reject command 只提供不可变授权事实；目标状态、解决人、时间戳和 repository update 只允许经 Approval-private lifecycle event/reducer 完成。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` / `kernel/governance` | consumer → Approval | get/list/list-by-scope、ensure approved/rejected | 完整 |
| `kernel/orchestrator` | 双向 | Approval 查询；`RecordApprovalEvidenceCommand/Result` 由 Approval 定义、Orchestrator 消费 | 完整；evidence 状态仍归 Orchestrator |
| `blocks/web` | Web → Approval | 外部访问所需的 scoped Approval 查询 | 完整；其他 capability owner 无直连 |
| `blocks/eventstream` | Approval → Eventstream | `TopicApprovalResolved` / `ApprovalChangedData` | 完整；只在提交后发布 |

## Owner 内部职责边界

- `owner.go` 只保留 Approval Biz 的构造、订阅与生命周期；`command_router.go` 是唯一 command ingress，并用静态穷尽分类器将 Topic 委派到 query、resolution 或 delete handler，未知 Topic fail closed。
- `query_commands.go` 只读 Approval repository；`resolution_commands.go` 保持 scope admission、Approval-private reducer、evidence、notification 和返回 scope 校验的既有顺序；approve/reject 共享同一 owner transaction，不形成两个状态机。
- `approval_scope.go` 只负责 workspace/run/step fence 与无 scope 唯一对象解析；`approval_notification.go` 只发布 Owner 已提交的 immutable lifecycle fact。

## 相关约束

- Approval 操作必须携带 workspace/run/approval scope；同 ID 跨 workspace 或 run 冲突时不得误操作。
- 本 block 只访问 Approval repository，不得直接访问 EvidenceRecord、Run、Workspace 或技术 block 实现。
- 已终态 Approval 的重复解决必须幂等或返回冲突，不能覆盖既有授权结论。
- private reducer 只允许 `pending -> approved|rejected`，未知目标和反向/交叉终态迁移必须 fail closed。
- Approval 只表达授权结果，不直接执行副作用或修改 Run/Result 状态。

## 待完善测试点

- [ ] 覆盖同一 Approval ID 在不同 workspace/run/step 下的歧义解析和越权拒绝。
- [ ] 覆盖 pending 到 approved/rejected 的完整转换矩阵，以及同结果重复提交与相反终态冲突。
- [ ] 覆盖 repository 更新失败时不发布 resolved 通知，成功重放时只形成一个终态事实。
- [ ] 覆盖 evidence command 的缺失 subscriber、nil/wrong result、重复与 stale scope。
- [ ] 覆盖查询结果深拷贝，调用方修改证据、metadata 或 scope 后不得污染 owner 状态。
