# Run Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-23

## 组件类型

`blocks/run` 按职责属于 Block：它只拥有 Run lifecycle 聚合，并为 Orchestrator-owned Runtime aggregate 提供
opaque `RuntimeStateRecord` 物理 CAS/查询合同；不解释 SM-2/3/4 语义，也不组合多个 owner 选择下一动作。目录、module ID 和 EventHub
合同身份均为 `blocks/run`。

## 功能范围与边界

`blocks/run` 管理 Run lifecycle、Run-owned replacement intent，以及 Step、RuntimeCheckpoint 的持久化查询/清理，
并提供 Runtime aggregate 的 opaque 物理持久化、
expected-revision CAS 和查询合同。Runtime 的 reducer、invariant、candidate、dispatch、evidence、recovery 与
terminal seal 均由 Orchestrator 负责；Run 不承担跨组件编排。

## 组件交互及合同

- Run owner 通过 `TopicRunCommandRuntimeRecoveryInventory` 提供分页 typed recovery inventory；item 只包含本
  owner 的 SM-1 state、bootstrap/state presence、opaque revision/ref、实际 persisted schema 和 Run-owned
  administrative disposition，不读取 dispatch/governance/delivery ledger 做 SM-2/3/4 分类。该窄只读 topic，以及
  `TopicRunCommandRuntimeStateRecord` 和 readiness query，在 `Setup`（Configure 后）即订阅，早于常规 Run
  query/cleanup/CAS topic；state-record query 只能按 workspace/run 与可选的 opaque ref/revision 返回克隆的 record 或明确缺失，
  不解释其 aggregate 语义。Orchestrator 通过该 typed record query load 后在 owner-local SM-4 分类 recovery case，
  不直接扫描 Run repository。

- 常规 Run command subscription 完成后，Run owner 发布一次 `TopicRunRuntimeCommandsReady` process-lifecycle
  fact。该 fact 的合同元数据为 `SM-4 + process scope + process-lifecycle identity`，不携带 workspace/run，也不迁移
  SM-1；它只释放 Orchestrator 已同步完成 inventory/load 的
  owner-local startup recovery actions。这样 recovery 不依赖 `Weight()`，且不会在 `StoreRuntimeStateRecordCommand` 尚未注册时
  由 BackgroundRoutine 提前驱动。重复 ready fact 只能观察已清空的同一 handoff，不能再次调度 startup drive。

- Run、Step、RuntimeCheckpoint 的查询、Run 删除、runtime resource purge 与 opaque Runtime CAS 均由
  `blocks/run/pkg/events` 定义逐 topic 的具体 command/result。不存在跨 topic 的 `RunCommandResult` 联合 envelope；
  `GetRunResult`、两类 Run list result、三类 Step result、两类 Checkpoint result、`PurgeRuntimeResourcesResult`、
  `StoreRuntimeStateRecordResult` 和 `DeleteRunResult` 彼此不可替代，返回实体在离开 owner 前显式深拷贝。
- 常规命令保持唯一入口 `handleCommand -> executeCommand`，并在同一次 `commandMu` 读锁与 workspace scope
  fence 内，静态穷尽地委派到 Run query、Step query、Checkpoint query、opaque Runtime state store 和 resource
  cleanup 五个 owner-private handler。公开 EventHub surface 不提供 create/ensure/update topic；Run creation material
  与 activation-memory handoff 均为 Biz 私有值，不在 `pkg/events` 暴露伪 create command。
- ordinary Run creation 与 replacement successor materialization 共用唯一 `run.creation` owner-private
  dispatcher。两个入口只提交 immutable creation fact，不携带 `State`/时间戳；Run owner 选择
  initial SM-1 `pending`、initial lineage `active`、封存 `CreatedAt` 并执行唯一 repository Create。
- workspace/run purge 通过 `PurgeRuntimeResourcesCommand` 由 Run owner 删除该 scope 下的 Step 与 RuntimeCheckpoint；命令必须先确认 Run 属于指定 workspace，并返回实际删除数量。
- Runtime 正式状态使用 typed `RuntimeStateRecord` 随 Run 物理持久化；update command 必须携带 expected runtime
  revision，并返回唯一的 `accepted`、`aggregate_revision_mismatch`、`semantic_rejection` 或 `owner_transient`
  typed fact。只有 expected/observed aggregate revision 实际不匹配才是 CAS；Run 对无关物理 record write 自行
  rebase。Run 不解释其中的 facts、authority、candidate、dispatch、evidence 或 governance ledger。
- `StoreRuntimeStateRecordCommand` 不是完整 Run entity replacement；它只允许 `kernel/orchestrator` 提交 opaque
  `RuntimeStateRecord` 与 expected aggregate revision。Run owner 必须在进入 CAS 前验证 EventHub source，其他 caller
  即使构造出合法 record 也必须被拒绝；没有 typed bootstrap 的 legacy/non-Runtime Run 也不能借该命令写入
  `RuntimeState` 或 lifecycle fence。
- `WorkspaceExecutionIntentCommand` 的 restart 分支携带 typed `RuntimeBootstrap`，保存 effective Goal/DoD definition、
  ordered focus queue 与 prerequisite requirements，并显式绑定 Workspace owner 封存的 `StartupInputHash`。RequestID 的幂等身份绑定 workspace、Goal、DoD、definition
  revision、bootstrap hash 与固定 replacement policy；同 ID、不同 sealed payload 必须 conflict。Run 不接受或持久化没有执行消费者的队列优先级输入。
- file-backed v1 clarification intent 的 immutable hash 曾包含现已退役的 caller `Priority` slot。Run model 只在磁盘
  解码时验证非零 slot 与既有 `ReplacementIntentHash` 精确匹配后，把它保存在 unexported persistence-only identity
  slot 中，以维持旧 Decision binding、后续 CAS 和 fresh-process duplicate Restart 的同一 owner identity；篡改或缺失
  slot 必须 fail closed。该 slot 不进入 command/EventHub result/API projection/调度，新建 intent 始终使用中性 `0`
  hash slot 且不序列化 `Priority`；只有已经验证的旧记录在持久化 round-trip 时保留原 v1 字段。
- 当 bootstrap 含有 activation Memory selection 时，Run owner 在 Run record 成功持久化后向 `blocks/memory` 发送一次 typed `UpsertMemoryRecallTraceCommand`。它使用稳定的 Run CreatedAt，并透传 sealed candidate/selected/excluded、quota、score/hit 与 byte/token budget；该消息只是不可变 activation audit，Memory owner 独占 create-once RecallTrace 持久化与首次 `MemoryRecallRecorded` 通知。固定 trace ID + 完整内容比较使 exact replay 零写入、内容漂移 conflict；memory observability 失败不得回滚已成功的 Run 创建。Run 启动时会从已持久化 bootstrap 重放该 fact，补偿 Run 提交后、Memory 交接前的崩溃窗口。
- 生产入口由 application 向 Workspace owner 提交 workspace-scoped resume/restart；Workspace 校验并选择 canonical
  definition 后，只有 `blocks/workspace` 可向 Run owner 发送 `WorkspaceExecutionIntentCommand`。Run Biz 在自身 reducer
  中完成 activation、continuation 或 replacement 准入、CAS 持久化并发布 typed fact。Runtime Resume 不接受 loaded
  Run 或上下文 patch。
- Resume 只使用所选 Run 自己的 immutable bootstrap、`DefinitionRevision` 与 `StartupInputHash`；Workspace 后续 candidate、
  promotion 或 Goal/DoD 变化不能改写该 Run。Restart 缺少 eligible admission、definition mismatch 或 startup hash mismatch
  时在 Workspace ingress fail-closed，Run 不接受半封存 startup input。
- Runtime-definition clarification 的 successor replacement 先持久化 immutable intent，再同步提交 Decision owner 专用
  resolution command。intent 按 `pending_clarification_resolution -> clarification_resolution_bound` 后才允许 administrative
  supersede/delivery；scope mismatch 进入 `clarification_resolution_rejected`，不得创建 successor。只有用户提交不同
  RequestID 的显式 Restart 才能把 rejected intent 移入 immutable retirement history 并封存新 intent；相同 RequestID
  仍为 typed conflict，且 bound/deliverable/completed intent 均不可替换。`ticket_already_bound` 的不同 current identity/hash
  会随 rejection 持久化为 owner invariant breach，不能从 hash 重建 bootstrap 或由 Restart 覆盖。启动恢复只重放相同
  ticket/intent/hash，Decision acknowledgement 之前零 terminal delivery。
- Orchestrator 发现不支持的 persisted Runtime schema 时，只能发送带 scope、opaque state ref、actual/supported
  schema 和 safe original diagnostic 的 `RuntimeStateIncompatibleFact`。Run reducer 将它持久化为
  `fresh_activation_required`；该 Run 的 workspace resume admission 返回 typed non-resumable disposition，既不发 continuation
  fact，也不触发 drive。开发基线不兼容时必须清理旧数据，再由用户提交显式 fresh activation；
  不能用对不兼容 predecessor 的 restart 做 schema 迁移。
- governed blocker 与 continuation 必须各自恰好携带一个 DecisionTicket 或 Approval identity，并匹配已持久化的
  opaque governance fence；wait fence 的 resolution slot 必须为空，continuation 的 typed fact、Run-private reducer 与同步
  acknowledgement 必须匹配 fence 中 exact `ResolutionID`，即使 Run 已经 active 也要先完成该比较；opaque fence 的
  governance/terminal mode 必须恰选其一，terminal 不得夹带历史 governance identity。旧/错 resolution 零写入
  fail-closed；approval-only 路径与 ticket-only 路径同等合法。
- Run 创建及状态变化使用 `RunStateChangedData` typed notification；通知只包含 SM-1 state、causation 与 opaque
  RuntimeState revision/ref，不镜像 Orchestrator action/focus/repair。current development baseline 的 Runtime record 同时持久化 Orchestrator-authored
  opaque lifecycle fence；Run 仅比较 scope/schema/revision/ref/hash/bootstrap/fence，禁止解码 Decision、semantic、candidate、
  governance、evidence 或 drive 内容。若为 `fresh_activation_required`，通知额外携带
  Run-owned actual/supported schema identity；Workspace 必须保留同一 Run 的 active slot，直到用户显式 reset 后重新
  activation，不能把该非终态 disposition 当作 terminal release。通知仍不公开 aggregate 内容或选择 recovery action。
  Governance、Eventstream 和 application 按需订阅。
- Governance、Eventstream 和 application 按需导入 `blocks/run/pkg/events` 订阅或发送命令。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | recovery inventory/readiness、opaque Runtime record query/CAS、lifecycle/governance/terminal facts | 完整；SM-2/3/4 语义只在 Orchestrator |
| `blocks/workspace` | 双向 | Workspace-owner `WorkspaceExecutionIntentCommand`、Run state notification | 完整；application 不直连 Run mutation，SM-1 转换只在 Run reducer |
| `blocks/memory` | Run → Memory | activation recall trace upsert | 完整；观测失败不回滚 Run |
| `application/workorchd` | 双向只读/清理 | typed Run/Step/Checkpoint query、Run delete、`RunStateChangedData` | 完整；不提交 lifecycle 或 Runtime CAS |
| `kernel/governance` / `blocks/eventstream` | Run → consumer（Governance 另有只读 query） | typed Run query 与 `RunStateChangedData` | 完整；无 service/repository 注入 |

## 相关约束

- Run、Step 和 Checkpoint 操作必须携带足够的 workspace/run/step scope。
- 生产装配只接收 Run 自身 repository 与 EventHub，不得注入 WorkspaceReader、Orchestrator service、callback 或其他 owner repository。
- `Stop`/`Teardown` 必须逆序、幂等地解除 recovery inventory 与常规 topic；recovery inventory 不得在 owner 停止后继续响应。
- Run 不负责 coverage、evidence quality、Result acceptance 或 LLM 输出正确性判断。
- current-baseline restart 由 Run owner 先在非终态 predecessor 上 CAS 持久化 immutable `ReplacementIntent`；只有
  predecessor 的 `Result -> Workspace -> Run` terminal delivery 完成后，Run-private reducer 才幂等创建并激活
  successor。相同 RequestID 只有在完整 sealed activation payload 相同的情况下才是 duplicate；否则 fail-closed。
  request fact 不携带 `State` 或 mutable progress；唯一 private-event dispatcher 根据 clarification binding 选择
  `pending` 或 `pending_clarification_resolution`，termination claim/ack/failure、resolution acknowledgement、successor
  materialization 与 lineage/activation 全部经同一 CAS reducer 推进，禁止 generic mutate callback。
  successor create、predecessor lineage CAS 和 successor activation 是可独立重放的持久化切点；任一切点后重启都只
  收敛到同一 successor。非当前 development baseline 返回 `data_reset_required`，且不持久化 replacement intent。
- 不允许形成 active Run 无合法 wait、无 scheduled drive 且 Runtime aggregate 非终态的悬挂状态；该恢复分类来自
  Orchestrator/Run 的 typed inventory，不从 Step/nextAction 推断。
- Step 合同失败不得先记录 completed 再回滚为 failed。
- `RuntimeStateRecord` 的 SM-2/3/4 canonical invariant 由 Orchestrator 在提交前和 reload 后校验；Run owner 只校验
  schema envelope、workspace/run scope 与 expected revision。所有 Runtime state update 都必须携带 expected revision，
  缺失 CAS 或 stale revision 必须拒绝，不能覆盖更新后的 record。
- Run 的 immutable definition/bootstrap 字段不通过 update command 暴露：GoalID、DoDID、DefinitionRevision、
  InitialStepKind、CreatedAt 与 RuntimeBootstrap 创建后不可漂移；RuntimeStateRecord 只能由 Orchestrator 按 expected
  revision 整体 CAS，不能降级为无记录或
  覆盖为更低 revision；blocker 字段必须与 SM-1 RunState 一致。Run 不检查 RuntimeStateRecord 内部 definition、
  activation 或 decision 语义。
- candidate attempt、sealed dispatch、governance binding/resolution、focus/evidence 等均作为 opaque RuntimeStateRecord
  payload 原样 CAS 持久化；execution-budget allocation/consumption、SM-2 repair、SM-4 recovery和bounded recoverable wait
  同样只作为opaque aggregate sub-ledger保存。其合法性、去重、deadline/lease与状态迁移只由Orchestrator owner
  reducer/invariant负责，Run不汇总余额、不claim probe也不把wait解释成SM-1 blocked。
- RuntimeBootstrap 的顶层 definition identity/revision 必须与 Run 的 GoalID、DoDID 和 DefinitionRevision 一致；
  不一致的 owner-private creation fact 或 workspace restart selection 必须返回 conflict。RuntimeStateRecord 内部 definition context、facts 与
  activation 的完整性只由 Orchestrator invariant 校验，Run owner 不据此选择 CAS/recovery/lifecycle 路径。
- Resume 的状态准入与迁移完全由 Run owner reducer 完成；成功后才发布不可变 Run state fact。不得通过 Run command
  传递 `map[string]any`、指定外部 owner state 或调用 Orchestrator service facade。
- Run 不提供 Validate/Complete service command；禁止 application 或其他 module 绕过 Runtime reducer/evidence gate 直接把 active Run 标记为 validated/completed。
- Run owner handler 直接调用本 owner `biz`；不保留跨 owner `Service()` 或 `service/` forwarding facade。跨组件只接受
  Workspace-owner execution selection、Orchestrator governance/continuation/incompatibility/terminal fact，以及本 owner
  query/cleanup/CAS；禁止外部 Block/Fail/Complete 目标 state setter。
- Run biz 不再持有或构造 Orchestrator adapter；已删除仅用于 禁止的 lifecycle 转发的 service DTO alias 和 test-only
  constructor。Run owner 的生产依赖只包含自身 repository、EventHub、EventHub-backed Workspace typed read port 与
  状态通知能力。

## 回归覆盖

已覆盖：SM-1 全状态合法/非法边矩阵（含 administrative cancel/supersede、终态反向迁移与未知状态拒绝）；Run creation/replacement successor 共用 private dispatcher、caller-selected initial state 与直达 repository Create 拒绝；ticket/approval
两类 wait/continuation、terminal stale/duplicate、fresh-activation；restart RequestID sealed-payload 冲突、intent 先持久化、
runtime clarification acknowledgement gate，以及
terminal→successor create→lineage CAS→activation 各持久化切点的崩溃恢复与
唯一 successor；Runtime CAS source/scope/schema/hash、aggregate revision race、物理 rebase 与 legacy bypass；Setup 阶段
inventory/record read、commands-ready 元数据与重复 handoff、typed result clone、workspace scope；activation Memory 完整
selection fact、missing subscriber/owner error 不回滚、Run 启动补写与 Memory owner exact replay、in-flight Stop 与重复
Start/Stop。当前无本 README 已知可执行缺口。
