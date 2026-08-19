# Approval Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-19

## 功能范围与边界

`blocks/approval` 是 Approval 聚合的唯一 owner。聚合 identity 为
`(WorkspaceID, RunID, ApprovalID)`，负责保存不可变授权目标、`pending -> approved|rejected`
生命周期、解决提案，以及 evidence/audit/notification 的交付检查点。

本 Block 不拥有 SM-1～SM-4，也不执行被授权的副作用。它通过 typed contract 参与 SM-2：
Orchestrator 创建 Runtime Approval 请求，Approval 在终态落库前将 Runtime resolution proposal
提交给 Orchestrator；崩溃后只从持久化提案和交付检查点重放，不重建 Runtime authority。

`Approval.Context` 是创建时封存的授权目标，approve/reject 不能修改；解决方补充的数据只进入
`ResolutionContext`，用于审计和说明，不能替换 action、provider、URL/query 或其他目标字段。

## 生命周期与恢复协议

1. Orchestrator 先从 Approval owner 获取 `approval/request` capability，再以
   `OpenRuntimeApprovalRequestCommand/Result` 创建 Runtime Approval。入口只接受 Orchestrator，
   相同 identity 的完全相同请求是幂等重放，不同 sealed payload 返回 conflict。
2. 解决入口只接受两个生产 producer：`application/workorchd/appservice` 的
   `human_operator` principal，或 `kernel/governance` 的 `auto_governance` principal；Event source、
   principal kind 和 subject 必须匹配。该 principal 表达进程内 producer authority，不替代 transport
   层的终端用户认证。
3. owner 先持久化 `ResolutionProposal`。Runtime Approval 随后同步提交
   `CommitRuntimeGovernanceApprovalResolutionCommand/Result`；只有 committed/duplicate 才能进入终态，
   stale proposal 会被持久化为 rejected 并 fail closed。普通 owner-local Approval 不触发 Runtime commit。
4. 终态落库后，owner 依次取得 Orchestrator evidence 的 exact-scope ack、Eventstream resolution record
   的 durable ack，再发布稳定 `NotificationID` 的 `TopicApprovalResolved` 展示通知并写入交付检查点。
   任一阶段失败都保留可恢复状态；后台 reconciliation 从 proposal/terminal checkpoint 继续，不重新解决
   Approval，也不把失败描述成跨 owner transaction。
5. Workspace purge 是唯一公开删除语义：先写 deletion tombstone，再按完整 scope 删除；不存在独立的
   Approval delete command。

## 组件交互及合同

| 交互组件 | 方向 | 具体 typed contract | 约束 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | Orchestrator → Approval | `AcquireRuntimeCapabilitiesCommand/Result`、`OpenRuntimeApprovalRequestCommand/Result` | 只接受 Orchestrator；请求绑定 workspace/run、dispatch、authority snapshot、action 与 authorization hash |
| `kernel/orchestrator` | Approval → Orchestrator | `CommitRuntimeGovernanceApprovalResolutionCommand/Result`、`RecordApprovalEvidenceCommand/Result` | 前者是终态前 SM-2 commit；后者是终态后 exact-scope evidence ack |
| `application/workorchd` | Application → Approval | `GetApprovalCommand/Result`、三类 bounded list result、`EnsureApprovedCommand/Result`、`EnsureRejectedCommand/Result`、`PurgeWorkspaceApprovalsCommand/Result` | ID-only HTTP 路由先做有界全局发现；重复 identity 必须报 conflict |
| `kernel/governance` | Governance → Approval | scoped get、`EnsureApprovedCommand/Result` | 仅以 `auto_governance` principal 解决当前 workspace/run Approval |
| `blocks/web` | Web → Approval | scoped `GetApprovalCommand/Result` | capability 只消费与 sealed target/hash 精确一致的 approved grant |
| `blocks/eventstream` | Approval → Eventstream | `RecordApprovalResolutionCommand/Result` | 同步持久化；相同 notification material 幂等，不同 material 冲突 |
| projection / display consumers | Approval → consumer | `TopicApprovalResolved` / `ApprovalChangedData` | 非 authority 通知；携带稳定 notification/resolution identity 和完整 scope |

所有 query/mutation 都有具体 result 类型，不再共用联合 envelope。列表命令必须显式 scope，使用稳定 cursor，
默认 limit 100、上限 200；`ListAllApprovalsCommand` 还必须显式声明 `Scope: all`。

## Owner 内部职责边界

- `owner.go` 只负责 Biz 构造、typed topic 订阅、reconciliation 生命周期和 teardown；重复 Start/Teardown
  不增加订阅或遗留后台任务。
- `command_router.go` 对 query、resolution、purge 三类 topic 做穷尽分派，未知 topic fail closed。
- `runtime_governance_adapter.go` 只负责可信 Runtime create ingress；`resolution_commands.go` 负责
  proposal/commit/finalize 协议；private lifecycle reducer 是聚合变更的唯一入口。
- `evidence_event.go`、`approval_notification.go` 只执行 scope-validated delivery；`reconciliation.go`
  只重放持久化 proposal/checkpoint。
- 本 Block 只持有 Approval repository，不直接访问 Run、Workspace、EvidenceRecord 或 capability implementation。

## 已覆盖的合同回归

- immutable authorization target 与独立 `ResolutionContext`；可信 source/principal 组合及伪造来源拒绝；
- Runtime request exact replay、sealed payload 冲突、SM-2 commit 的 nil/wrong/stale/duplicate 处理；
- commit 后崩溃、终态后 evidence 失败、Eventstream ack 失败及 notification 重放恢复；
- pending/approved/rejected 转换、同结果幂等、相反终态冲突、repository 写失败不提前发布；
- 同 Approval ID 跨 workspace/run 的 memory/file backend 隔离及错误 scope 拒绝；
- concrete typed result、显式 all scope、cursor/limit 边界与 detached deep-copy；
- workspace purge tombstone、重复 Teardown、capability/open ingress 的 Orchestrator-only 边界。

## 非本 Block 合同

- 终端用户的登录、角色和凭证验证属于 transport/application；Approval 只校验已经进入进程的 producer
  authority 与 typed principal。
- Eventstream 是 durable audit/display owner，不参与授权判定；projection 和普通 resolved notification
  丢失时可由持久化 audit/checkpoint 重放，不能作为 Runtime authority。
- 低层 file repository 只负责一个已解析 run path；跨 workspace/run 的定位与歧义拒绝由生产
  `workspaceScopedApprovalRepository` 提供。
