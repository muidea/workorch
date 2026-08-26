# Decision Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-26

## 组件类型与职责

`blocks/decision` 是 DecisionTicket/DecisionResolution 聚合的唯一 owner。Ticket 是聚合根，Resolution 和
`RuntimeGovernanceResolutionProposal` 都只能通过 Decision-private lifecycle event/reducer 创建或迁移。本 Block
表达治理选择并保存审计身份，不拥有 SM-1～SM-4，也不恢复 Run、不修改 Runtime aggregate、不执行 capability。

本组件影响 SM-2 governance resolution 的提交，以及 SM-4 的重放、恢复和投影交付；相应 authority 仍分别属于
Orchestrator 与各资源 owner。

## 写入与恢复合同

- 所有 Ticket creation 入口只构造无 state/时间戳/进度的 immutable seed，再提交同一
  `opened` Decision-private lifecycle event。只有 `lifecycle_internal_event.go` 可选择 initial `open`、封存
  timestamps 并执行 Ticket repository Create；generic、Runtime governance 和两类 clarification open 都不得直写初始状态。
- Definition clarification 不复用普通 `EnsureResolvedCommand` 或 Runtime governance envelope。pre-seal Workspace 路径使用
  dedicated open/reconcile-candidate/resolve-by-admission command/result；execution-time 路径使用 dedicated runtime open 与
  Run-owner resolve-by-replacement command/result。generic resolver、auto-governance policy 和 continuation action 均拒绝这两类 ticket。
- pre-seal open/reconcile/resolve result 都提供 `ValidateFor(exact command)`：调用方必须重算 stable ticket/resolution identity、
  request/reconciliation/resolution binding hash 与允许的 created/duplicate/resolved disposition。非空 ID/hash、错误 scope 或
  不匹配的 Decision result 不能被映射为 Workspace admission fact。owner-neutral
  `internal/pkg/workorchdefinitioncontract` 只保存 immutable `Identity/Binding/Record` values 与 pure identity proof；Decision
  `pkg/events` 声明自己的 concrete named command/result roots 并显式转换这些 values，不使用 type alias。Workspace
  `pkg/events` 只依赖 neutral values，不导入 Decision owner package；Workspace Biz 作为正常 EventHub sender 可导入 Decision
  events，并在进入 private admission reducer/CAS 前调用 `QueryWorkspaceDefinitionClarificationFactCommand/Result`。该 read 由
  Decision ticket aggregate 与 resolution repository 重建 persisted record；Found=false、retired/superseded ticket、partial
  resolution、binding 漂移，或外层 Ticket 的 canonical DecisionKind/reason/question/failure/actions/context/governance 语义漂移均
  fail-closed，caller 自洽的 identity/binding pair 本身不构成 provenance。
- 静态 invalid candidate ticket 绑定完整 candidate revision/hash、typed violation、canonical signature bytes/hash 与修改动作；
  新 invalid revision 先建立确定性新 ticket，再以 `definition_candidate_revision_superseded` 幂等退休旧 ticket。新 candidate
  admitted 后只记录 `definition_candidate_adjusted` resolution；Application 只转交 immutable resolution fact，Workspace-private
  reducer 才决定 activation eligibility。
- Runtime clarification ticket 绑定 Run、Definition/StartupInput、authority、focus、semantic request/document 与 clarification
  dispatch。只有 `blocks/run` 已持久化的 replacement intent 可以绑定 resolution；重复 command 返回相同 resolution/binding
  hash，scope mismatch 或另一 intent 返回 typed stale/conflict，不生成 Runtime continuation。
- clarification resolution 不得在 handler 中先改 nested binding 再改 Ticket state。handler 只提交 bounded
  `definitionClarificationResolutionFact`，唯一 lifecycle reducer 在同一 owner Update 中原子绑定
  resolution/replacement/admission identity 并执行 `open -> resolved`；partial binding 或已解决 binding 重写 fail-closed。

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
- `RuntimeGovernanceResolutionProposal` 的 ingress seed 同样不携带 state、failure、timestamp 或 delivery progress；
  `proposal_prepared` private event 是唯一可选择 initial `pending` 并封存 proposal timestamps 的位置，caller-selected
  proposal lifecycle 字段必须 fail-closed。
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
- Ticket 与 Resolution 不公开独立 delete command。Workspace owner 先发送
  `SealWorkspaceDecisionDeletionCommand`；Decision 持久化永久、workspace-scoped tombstone 后，所有公共查询、恢复和普通
  Create/Update/Delete 都 fail closed。`retain` 保留物理数据但永久隐藏；`purge` 只能由同一 scope 的
  `PurgeWorkspaceDecisionsCommand` 翻译为 `purge_requested` owner-private event；只有同一个 private dispatcher 能在 exact
  `fenced` scope 下进入 privileged sink，fixed-point 清空 Resolution 与 Ticket，再提交 `purge_completed` CAS 将 tombstone
  转为 `purged`。command adapter 不直接调用 purge fence/物理 helper，伪造 `purge_completed` 也不能越过 dispatcher。
  seal/purge exact replay 返回稳定 FenceID/PurgeID/AckHash/PurgeProofHash。
- seal、purge、普通 mutation 和逐 Ticket reconciliation 共用 Decision owner-local workspace mutation lock；reconciliation
  进入锁后必须重新读取最新聚合。每一次发往 Orchestrator/Eventstream 的 commit、evidence 或 notification 都在
  Decision durable active-fence 回调内再次裁决，因而 tombstone 一旦持久化，当前进程和重启实例都不能补发 late delivery。

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
| `blocks/run` | 双向受限 | dedicated runtime-definition replacement resolution；普通 governance 仍经 Orchestrator fact | 充分；clarification ack 不迁移 SM-1、不生成 continuation |
| `blocks/eventstream` | Decision → Eventstream | created/resolved typed notification + stable NotificationID | 充分；Eventstream 幂等记录，projection 不参与 authority |
| workspace deletion | Workspace → Decision | seal fence + optional fixed-point purge | exact scope；不暴露 Ticket/Resolution 独立删除 |

## Owner 内部边界

- `owner.go` 只负责 Biz 构造、订阅与生命周期；`command_router.go` 对七个普通 topic 静态穷尽分派，未知 topic
  fail-closed。Runtime governance open/capability ingress 另由专用 adapter 处理并先校验 Orchestrator source。
- mutation implementation 与 constructor 均为 Decision owner-private；跨 package（包括测试）只能启动 owner Module
  后发送 `pkg/events` concrete command/result，或消费 owner 已发布的 immutable fact，不得直接构造 mutation service、
  导入 `biz/contracts` 或注入 Ticket/Resolution repository。
- `ticket_commands.go` 只实现 scoped query 与 keyset page；`workspace_deletion_internal_event.go` 独占 workspace fence/purge reducer；`resolution_commands.go` 只实现
  resolution query/ensure；二者共享 workspace fence，但不形成第二个 owner 或 facade。
- `lifecycle_internal_event.go` 是 Ticket creation/lifecycle、clarification binding、proposal、delivery marker 和 deletion tombstone 的唯一状态 reducer。
  `runtime_governance_proposal_reconciliation.go` 只重放已持久化 fact，不选择新的 resolution 或 Runtime action。
- 生产依赖只包含 DecisionTicket repository、DecisionResolution repository、EventHub 和 framework background routine；
  不得注入 Run、Workspace、Evidence、Orchestrator repository/service 或 application callback。

## 已覆盖回归

- Runtime open 的错误 source、错误 workspace context、合法 scope 与确定性重复创建。
- nil/wrong/missing/owner-error commit 保持 pending，stale 显式拒绝，以及 committed/duplicate finalize。
- finalize 后 evidence 失败的重启补写、delivery marker 幂等、完成后的 Runtime replay 零 Ticket update/零时间戳漂移。
- 普通 resolution 脱敏 payload 重放，continuation/resume/successor drift conflict，以及 Resolution/Ticket 写入切点恢复。
- workspace retain/purge tombstone、public read/recovery 隔离、普通 Delete-after-seal 拒绝、fixed-point purge 与 exact replay。
- deletion seal 与在途 reconciliation delivery 的线性化、seal 后零 Orchestrator/Eventstream delivery，以及重启实例对未完成
  resolved delivery checkpoint 的永久抑制。
- concrete result type、显式全局 scope、默认/最大 limit、composite cursor continuation、跨 workspace 重复 ID 和深拷贝。
- stable Decision notification identity 与 Eventstream duplicate append 去重。
- Workspace invalid-candidate replacement、admission resolution、owner-neutral Identity/Binding values 上的 open/reconcile/resolve
  deterministic pair validation、persisted-fact query 的 Found=false/retired/partial/binding mismatch 与外层 Ticket semantic
  mutation 拒绝、Workspace events 零
  Decision-owner import，以及 Runtime replacement resolution 的 exact
  duplicate、scope mismatch、orphan-resolution recovery。
- Ticket/proposal seed 均拒绝 caller-selected initial/progress state；clarification binding 与 `open -> resolved` 单次 owner write，静态门禁拒绝 handler 直写 binding、proposal initial state 或 Ticket repository Create。

## 闭合验证

- [x] file backend 使用独立 helper process 在原子 owner 写入完成后执行真实 SIGKILL，覆盖 pending proposal、formal
  Resolution 已写与 finalize 后双 delivery marker 等切点；workspace deletion 使用独立永久 tombstone/replay 回归，不复用逐实体 lifecycle；每个切点都由新 owner
  实例从持久化状态幂等收敛。
- [x] `BenchmarkDecisionTicketInventoryPage100K` 固定 100k Ticket inventory/page 基准；分页排序改为 typed composite-field
  比较，避免在 sort comparison 中反复拼接 cursor。单次基准分配由百万级降到仅 page clone/visible index 所需的有界分配。

当前没有与本合同对应的已知可执行缺口。
