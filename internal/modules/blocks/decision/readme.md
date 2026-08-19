# Decision Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-19

## 组件类型与职责

`blocks/decision` 是 DecisionTicket/DecisionResolution 聚合的唯一 owner。Ticket 是聚合根，Resolution 和
`RuntimeGovernanceResolutionProposal` 都只能通过 Decision-private lifecycle event/reducer 创建或迁移。本 Block
表达治理选择并保存审计身份，不拥有 SM-1～SM-4，也不恢复 Run、不修改 Runtime aggregate、不执行 capability。

本组件影响 SM-2 governance resolution 的提交，以及 SM-4 的重放、恢复和投影交付；相应 authority 仍分别属于
Orchestrator 与各资源 owner。

## 写入与恢复合同

- Runtime governance ticket 的唯一创建入口是 `OpenRuntimeGovernanceRequestCommand/Result`。Decision 在任何
  repository 访问前同时校验 EventHub source 必须为 `kernel/orchestrator`，并用 command `WorkspaceID` 绑定已有
  execution context；错误 source 或 workspace mismatch 必须零写入失败。Ticket ID、Run、dispatch、authority
  snapshot 和 allowed resolution type 都来自 sealed request，不接受通用 create/update 或 map context。
- 普通 resolution 使用 `EnsureResolvedCommand/EnsureResolvedResult`。持久化前生成的 canonical fingerprint 包含
  ResolutionID、TicketID、type、脱敏后的 payload、`ContinuationAction`、`ResumeRunID` 和 `SuccessorRunID`；完全相同
  的重放幂等，任一 sealed 字段漂移均 conflict。
- `runtime_governance` resolution 使用 owner-local 两阶段事务：先把 proposal 持久化为 `pending`，再向
  Orchestrator 发送 `CommitRuntimeGovernanceTicketResolutionCommand`，只有收到 scope/identity 完整且 disposition 为
  `committed` 或 `duplicate` 的 typed result 后，Decision 才创建正式 Resolution 并将 Ticket/proposal finalize 为
  resolved/committed。nil result、wrong result、missing subscriber 和 owner/transient error 都保留 pending；只有
  Orchestrator 明确返回 `stale_diagnostic` 才能标为 rejected。
- 已 committed 的相同 Runtime resolution 重放仍会向 Orchestrator 执行幂等 join，但不会再次 Update Ticket、刷新
  Ticket/proposal 时间戳或重写 Resolution。proposal 的 evidence/notification marker 只有在对应交付成功后由 private
  reducer 持久化；finalize 后进程中断或 evidence owner 失败时，启动/周期 reconciliation 会从正式 Resolution 重放
  未完成交付。
- 普通 resolution 的 `Resolution create -> Ticket update` 写入切点同样可恢复：若只留下 Resolution，owner
  reconciliation 使用持久化的完整 resolution fingerprint 触发 private recovery event，将 Ticket 收敛到 resolved 并补发
  evidence/notification；外部 caller 不得直接修复 Ticket state。

## 查询与删除合同

- 每个 EventHub topic 都使用独立 concrete result：`GetTicketResult`、`ListTicketsResult`、
  `ListTicketsByRunResult`、`ListTicketsByWorkspaceResult`、`GetResolutionResult`、`EnsureResolvedResult` 和
  `PurgeWorkspaceDecisionsResult`。不存在同时承载 Ticket、Resolution、列表和删除计数的通用 result envelope。
- 所有 list command 使用 workspace/run/ticket composite keyset cursor；默认单页 100、上限 200。全局列表必须显式
  指定 `TicketListScopeAll`，workspace/run 列表必须显式携带 WorkspaceID；run query 不允许只有 RunID。Application
  将 HTTP cursor/limit 直接交给 owner，不再先读取全量 Ticket 后本地分页。
- 兼容性的无 workspace Get/resolve 只允许解析全局唯一 TicketID；跨 workspace 重复 ID 必须 conflict，不能按状态、
  repository 顺序或“优先 open”猜测 owner scope。所有返回 entity 在离开 owner 前显式深拷贝。
- Ticket 与 Resolution 不再公开两个独立 delete command。唯一删除面是 workspace-scoped
  `PurgeWorkspaceDecisionsCommand`：owner 先持久化 `DeletionRequestedAt`，立即从所有 public read 隐藏聚合，再删除
  Resolution 和 Ticket。任一步失败只留下 owner-private tombstone，reconciliation 可继续完成，不能形成可见的
  resolved Ticket 无 Resolution、孤立 Resolution，或单独删除等待中的 Runtime Ticket。

## 通知与 evidence 合同

- Ticket created/resolved 分别发布 `TicketChangedData` 与 `TicketResolvedData`；不存在“任意 Ticket 状态变化”通知。
  两者携带 Decision owner 派生的 stable `NotificationID`，Eventstream 以该 ID 作为 EventRecord identity，重复 append
  只对相同通知视为幂等。
- Runtime ticket creation、Runtime resolution evidence/resolved notification，以及普通 resolution notification 都有
  owner-private delivery marker。调用失败和重启恢复不会把未完成交付误判成新治理选择；marker 完成后的相同 Runtime
  resolution 是 owner repository 零写入 duplicate。
- Resolution evidence 使用 Decision 定义的 `RecordDecisionEvidenceCommand/Result` 同步交给 Orchestrator；Decision
  不访问 EvidenceRecord repository。EvidenceRecordID 由 resolution identity 稳定派生，因此失败后的重放不会形成
  第二条 evidence。
- 通知与 evidence 都是 SM-4 audit/projection 交付，不反向决定 SM-2 authority，也不允许 Application 因通知成功而
  自动 Resume Run。

## 组件交互及合同

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | sealed Runtime governance open、typed resolution commit、Decision evidence record | 充分且必要；source/scope fail-closed，Decision 不代写 Runtime |
| `application/workorchd` / `kernel/governance` | consumer → Decision | concrete scoped query/page、普通/Runtime ticket resolution | 充分且必要；无全量后分页、无 repository/service 注入 |
| `blocks/run` | 间接消费 | Orchestrator 持久化的 immutable governance wait/continuation fact | 充分；Decision 不直接迁移 SM-1 或 Resume Run |
| `blocks/eventstream` | Decision → Eventstream | created/resolved typed notification + stable NotificationID | 充分；Eventstream 幂等记录，projection 不参与 authority |
| workspace cleanup | Application → Decision | workspace-scoped aggregate purge | 充分；不暴露 Ticket/Resolution 独立删除 |

## Owner 内部边界

- `owner.go` 只负责 Biz 构造、订阅与生命周期；`command_router.go` 对七个普通 topic 静态穷尽分派，未知 topic
  fail-closed。Runtime governance open/capability ingress 另由专用 adapter 处理并先校验 Orchestrator source。
- `ticket_commands.go` 只实现 scoped query、keyset page 与 workspace aggregate purge；`resolution_commands.go` 只实现
  resolution query/ensure；二者共享 workspace fence，但不形成第二个 owner 或 facade。
- `lifecycle_internal_event.go` 是 Ticket、proposal、delivery marker 和 deletion tombstone 的唯一状态 reducer。
  `runtime_governance_proposal_reconciliation.go` 只重放已持久化 fact，不选择新的 resolution 或 Runtime action。
- 生产依赖只包含 DecisionTicket repository、DecisionResolution repository、EventHub 和 framework background routine；
  不得注入 Run、Workspace、Evidence、Orchestrator repository/service 或 application callback。

## 已覆盖回归

- Runtime open 的错误 source、错误 workspace context、合法 scope 与确定性重复创建。
- nil/wrong/missing/owner-error commit 保持 pending，stale 显式拒绝，以及 committed/duplicate finalize。
- finalize 后 evidence 失败的重启补写、delivery marker 幂等、完成后的 Runtime replay 零 Ticket update/零时间戳漂移。
- 普通 resolution 脱敏 payload 重放，continuation/resume/successor drift conflict，以及 Resolution/Ticket 写入切点恢复。
- workspace aggregate purge 的部分失败 tombstone、public read 隔离和 reconciliation 完成。
- concrete result type、显式全局 scope、默认/最大 limit、composite cursor continuation、跨 workspace 重复 ID 和深拷贝。
- stable Decision notification identity 与 Eventstream duplicate append 去重。

## 待完善测试点

- [ ] 在 file backend 上加入真实进程 kill/fault injection，覆盖 proposal、delivery marker 和 aggregate purge 的每个 fsync
  切点；当前内存故障注入与 file repository 回归已分别覆盖语义和持久化格式。
- [ ] 增加 100k Ticket inventory/page 的 benchmark；单页上限、consumer page/cursor cycle guard 已有功能回归。
