# Run Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-29

## 组件类型

`blocks/run` 按职责属于 Block：它只拥有 Run lifecycle 聚合，并为 Orchestrator-owned Runtime aggregate 提供
opaque `RuntimeStateRecord` 物理 CAS/查询合同；不解释 SM-2/3/4 语义，也不组合多个 owner 选择下一动作。目录、module ID 和 EventHub
合同身份均为 `blocks/run`。

## 功能范围与边界

`blocks/run` 管理 Run lifecycle、Run-owned replacement intent，以及 Step、RuntimeCheckpoint 的持久化查询和 Workspace-scoped 清理，
并提供 Runtime aggregate 的 opaque 物理持久化、
expected-revision CAS 和查询合同。Runtime 的 reducer、invariant、candidate、dispatch、evidence、recovery 与
terminal decision/delivery handoff 均由 Orchestrator 负责；Run 不承担跨组件编排，但 Run terminal lifecycle 仍只由
本 owner 的 SM-1 private event 持久化。

## 组件交互及合同

- Run owner 通过 `TopicRunCommandRuntimeRecoveryInventory` 提供分页 typed recovery inventory；item 只包含本
  owner 的 SM-1 state、bootstrap/state presence、opaque revision/ref、实际 persisted schema 和 Run-owned
  administrative disposition，不读取 dispatch/governance/delivery ledger 做 SM-2/3/4 分类。该窄只读 topic，以及
  `TopicRunCommandRuntimeStateRecord` 和 readiness query，在 `Setup`（Configure 后）即订阅，早于常规 Run
  query/cleanup/CAS topic；state-record query 只能按 workspace/run 与可选的 opaque ref/revision 返回克隆的 record 或明确缺失，
  且三条 SM-4 read topic 都只接受 `kernel/orchestrator` source，不向 Application、projection 或其它 owner 暴露 opaque
  aggregate。Orchestrator 通过该 typed record query load 后在 owner-local SM-4 分类 recovery case，
  不直接扫描 Run repository。
  `TopicRunCommandWorkspaceRetirementInventory` 与 exact `TopicRunCommandWorkspaceRetirementFact` 也在 Setup phase
  提前订阅，但只接受 `kernel/orchestrator` source；前者列出 durable `draining/retired` fence，后者在 Run owner
  边界验证 Run、opaque Runtime record、lifecycle fence、terminal-delivery ref/revision/hash 的物理一致性。这样
  Orchestrator 以更早 framework weight 启动时仍能恢复删除 gate；`draining` 中 terminal 且仍有 Runtime record 的
  Run 会保留在 recovery inventory 供 terminal drain，`retired` Workspace 则不产生 recovery action。

- Run owner 另通过 `TopicRunCommandLifecycleFactInventory` 提供分页、稳定排序的 current non-terminal
  `RunStateChangedData` inventory，供 Workspace owner 补偿通知丢失或自身 CAS/存储失败。每项携带 Run owner 的
  `WorkspaceRunGeneration`、`RunRevision`、opaque `RuntimeStateRevision/RuntimeStateRef` 与由这些坐标精确派生的
  causation；opaque cursor 按 `WorkspaceID + generation + RunID` 排序，不从 RunID 字符串猜测先后。若同一 Workspace
  出现多个 non-terminal Run，inventory 必须 fail-closed。它只重放
  current SM-1 fact，不暴露 Runtime aggregate，也不接受目标 Run/Workspace state。terminal Run 不进入该 inventory，
  Workspace 的 terminal slot release 仍只消费 Orchestrator 的 sealed terminal delivery。

- 常规 Run command subscription 完成后，Run owner 发布一次 `TopicRunRuntimeCommandsReady` process-lifecycle
  fact。该 fact 的合同元数据为 `SM-4 + process scope + process-lifecycle identity`，不携带 workspace/run，也不迁移
  SM-1；它只释放 Orchestrator 已同步完成 inventory/load 的
  owner-local startup recovery actions。这样 recovery 不依赖 `Weight()`，且不会在 `StoreRuntimeStateRecordCommand` 尚未注册时
  由 BackgroundRoutine 提前驱动。重复 ready fact 只能观察已清空的同一 handoff，不能再次调度 startup drive。

- Run、Step、RuntimeCheckpoint 的查询、Workspace retirement/purge 与 opaque Runtime CAS 均由
  `blocks/run/pkg/events` 定义逐 topic 的具体 command/result。不存在跨 topic 的 `RunCommandResult` 联合 envelope；
  `GetRunResult`、两类 Run list result、三类 Step result、两类 Checkpoint result、opaque store result 与
  Workspace retirement/purge result 彼此不可替代，返回实体在离开 owner 前显式深拷贝。
- 常规命令保持唯一入口 `handleCommand -> executeCommand`，并在同一次 `commandMu` 读锁与 workspace scope
  fence 内，静态穷尽地委派到 Run query、Step query、Checkpoint query、opaque Runtime state store 和 Workspace
  retirement 五个 owner-private handler。公开 EventHub surface 不提供 create/ensure/update/delete topic；Run creation material
  与 activation-memory handoff 均为 Biz 私有值，不在 `pkg/events` 暴露伪 create command。
- ordinary Run creation 与 replacement successor materialization 共用唯一 `run.creation` owner-private
  dispatcher。两个入口只提交 immutable creation fact，不携带 `State`/时间戳；Run owner 选择
  initial SM-1 `pending`、命名 `RunLineageState` 的 initial lineage `active`、封存 `CreatedAt`，再调用唯一
  `CreateNextWorkspaceRun` persistence sink。repository 在共享 Workspace durable head 的同一临界区内原子分配
  `WorkspaceRunGeneration` 与同源 `RunID`；二者均不能由 caller 指定、不能从现存 row/时间戳推断，且 Run delete、
  runtime-directory purge、response loss 与进程重启均不复用。watermark 先于 Run row 持久化，因此写失败可以留下合法
  generation gap，但不能重复 identity；Workspace purge 后也保留 generation head 与永久 retirement fence。同一 data root 的进程内独立 repository set 也按 head path 串行。file backend
  当前合同为单进程 writer。
- `run.replacement` 不是独立 resource state machine，而是 SM-1 唯一 Run owner 的 subordinate transaction。它只推进
  clarification、administrative termination handoff acknowledgement 与 successor materialization 的私有 stage/lease；
  不能选择 SM-2～SM-4 authority，也不成为 Run lifecycle 的第二 owner。交接顺序固定为：Run-private replacement event
  claim termination intent → Orchestrator SM-4 CAS 保存 `RuntimeTerminationRecord` 并投递 Result/Workspace/Run → Run-private
  SM-1 terminal event → replacement transaction 消费 exact acknowledgement 并继续唯一 successor。
- Workspace 删除只接受 `blocks/workspace` 的 typed prepare/finalize/purge command，并推进独立的 Run-owned resource
  machine `run.workspace_retirement`，它不是 SM-1 transition，也不是第五个 Runtime SM。Prepare 在 Workspace generation-head
  同一锁内验证 terminal/replacement invariant 与完整 immutable deletion identity，只落 `draining`；该 phase 永久拒绝
  create/restart、普通 SM-1、Step 与 Checkpoint mutation，只为 terminal Run 保留 exact opaque Runtime-state CAS。
  Orchestrator durable quiescing gate 封闭该 CAS 后返回绑定 generation head、ordered Run/Runtime physical revisions 与
  hashes 的 typed token；Finalize 在同一 generation-head 锁内 exact compare 后才原子 `draining -> retired`，stale、duplicate
  different payload 与 CAS race 都 fail-closed。generation watermark 允许合法 gap，但 token 的每个 Run generation 必须严格
  递增且不越过 head；`retired` 后包括 opaque CAS 在内的所有 mutation 均永久关闭。
- purge 只能由 canonical `RunOwnerRepositories` bundle 的唯一三 surface transaction 执行；Run/Step/RuntimeCheckpoint
  repository 不再暴露可逐面调用的 purge lane。memory backend 在同一 owner lock、file backend 在同一 generation-head
  lock 中独立枚举并复核三面（包括 orphan resource），返回固定顺序的 `ObservedBefore/Deleted/Remaining` receipt。
  file backend 先持久化 named `pending` journal 与首次 observations，再物理删除并落 `completed`；response loss、重启或
  中间 cut 都重放相同 receipt/proof hash，而不是伪造新的 empty proof。永久 retirement/generation head 始终保留。
- retain 模式到达 `retired` 后，普通 Run/Step/RuntimeCheckpoint Get/List 在 owner query adapter 隐藏；专用 retirement
  fact/inventory 与 private purge surface 仍按其窄合同可读，SM-4 recovery inventory不再产生该 Workspace 的 item，exact
  Runtime-state record read返回 NotFound。`draining` 期间这些读取仅对 Orchestrator 保持可用，以完成 terminal drain。
- Runtime 正式状态使用 typed `RuntimeStateRecord` 随 Run 物理持久化；update command 必须携带 expected runtime
  revision，并返回唯一的 `accepted`、`aggregate_revision_mismatch`、`semantic_rejection` 或 `owner_transient`
  typed fact。只有 expected/observed aggregate revision 实际不匹配才是 CAS；Run 对无关物理 record write 自行
  rebase。Run 不解释其中的 facts、authority、candidate、dispatch、evidence 或 governance ledger。
- `StoreRuntimeStateRecordCommand` 不是完整 Run entity replacement；它只允许 `kernel/orchestrator` 提交 opaque
  `RuntimeStateRecord` 与 expected aggregate revision。Run owner 必须在进入 CAS 前验证 EventHub source，其他 caller
  即使构造出合法 record 也必须被拒绝；没有 current typed bootstrap 的 Run 也不能借该命令写入
  `RuntimeState` 或 lifecycle fence。
- `WorkspaceExecutionIntentCommand` 的 restart 分支携带 typed `RuntimeBootstrap`，保存 effective Goal/DoD definition、
  ordered focus queue 与 prerequisite requirements，并显式绑定 Workspace owner 封存的 `StartupInputHash`。RequestID 的幂等身份绑定 workspace、Goal、DoD、definition
  revision、bootstrap hash 与固定 replacement policy；同 ID、不同 sealed payload 必须 conflict。Run 不接受或持久化没有执行消费者的队列优先级输入。
- Restart 的 create 与 activation 是两个可恢复的 Run-private CAS 切点。Run 在暴露 Setup-phase Runtime inventory 前，
  先完整扫描并校验所有 non-terminal/pending records；只有全量通过后，才将
  `pending + valid sealed bootstrap + ActivationRequestID` 幂等激活。任何 malformed record、同 Workspace 多个
  non-terminal Run 或 CAS/storage failure 都使 startup fail-closed，不能边扫描边部分写入。同 RequestID 重放时，
  pending Run 继续 owner-private activation；active Run 补发 typed `RunContinuationRequestedData` 以修复 activation CAS
  已提交但首次 notification/response 丢失的切点；不同 RequestID 不能抢占仍 pending 的 activation。重复恢复不创建
  新 Run、不分配新 generation，也不重复推进已 active 的 SM-1 transition。
- `ReplacementIntent` identity 只哈希当前 sealed payload；caller `Priority` 不属于模型、持久化、EventHub result、
  API projection 或调度。带 `Priority` 或其他退役/未知字段的 Run/intent JSON 在 repository decode 时直接失败。
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
  supersede/delivery。若 Restart 的 Definition revision 或 `StartupInputHash` 任一仍与 exact-ticket predecessor 相同，Run owner
  必须在 private replacement event/CAS 前返回 typed `governance_required/definition_clarification_pending`，零 intent、零 recovery
  调度、零跨 owner command；只有两项均更新才可进入 replacement transaction。scope mismatch 进入
  `clarification_resolution_rejected`，不得创建 successor。只有用户提交不同
  RequestID 的显式 Restart 才能把 rejected intent 移入 immutable retirement history 并封存新 intent；相同 RequestID
  仍为 typed conflict，且 bound/deliverable/completed intent 均不可替换。`ticket_already_bound` 的不同 current identity/hash
  会随 rejection 持久化为 owner invariant breach，不能从 hash 重建 bootstrap 或由 Restart 覆盖。启动恢复只重放相同
  ticket/intent/hash，Decision acknowledgement 之前零 terminal delivery。
- Run 与 Runtime persistence 只接受一个 current shape。旧目录不再探测或导入；当前路径上的未知/退役字段、空文件或
  当前不变量缺失使 repository/owner load 直接失败。Run 不保存 schema status、不接收 invalid-data fact，也不把无效数据
  转换为 execution admission outcome。`fresh_activation_required` 仅表示一个可读 current Run 缺少 sealed bootstrap。
- current Run lineage 必须显式存在；需要 supersede 的 Run 只能从 `LineageState=active` 转换。空 lineage 直接冲突且
  不补写、不推断为 active，也不创建 replacement successor。
- governed blocker 与 continuation 必须各自恰好携带一个 DecisionTicket 或 Approval identity，并匹配已持久化的
  opaque governance fence；wait fence 的 resolution slot 必须为空，continuation 的 typed fact、Run-private reducer 与同步
  acknowledgement 必须匹配 fence 中 exact `ResolutionID`，即使 Run 已经 active 也要先完成该比较；opaque fence 的
  governance/terminal mode 必须恰选其一，terminal 不得夹带历史 governance identity。stale/错 resolution 零写入
  fail-closed；approval-only 路径与 ticket-only 路径同等合法。
- Run 创建及状态变化使用 `RunStateChangedData` typed notification；通知只包含 SM-1 state、Run owner 分配的
  `WorkspaceRunGeneration`、Run revision、causation 与 opaque RuntimeState revision/ref，不镜像 Orchestrator
  action/focus/repair。同一个 generation/Run/revision/runtime-revision tuple 具有唯一事实身份，消费者可据此拒绝
  stale、duplicate 或不可比较的投影输入。current Runtime record 同时持久化 Orchestrator-authored
  opaque lifecycle fence；Run 仅比较 scope/revision/ref/hash/bootstrap/fence，禁止解码 Decision、semantic、candidate、
  governance、evidence 或 drive 内容。通知不携带 schema-validity identity、不公开 aggregate 内容或选择 recovery action。
  Governance、Eventstream 和 application 按需订阅。
- Governance、Eventstream 和 application 按需导入 `blocks/run/pkg/events` 订阅或发送命令。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | recovery inventory/readiness、opaque Runtime record query/CAS、lifecycle/governance/terminal handoff facts | 完整；SM-2/3/4 语义只在 Orchestrator，Run terminal 只在 SM-1 |
| `blocks/workspace` | 双向 | Workspace-owner execution intent、retirement prepare/finalize/purge、Run state notification 与 current lifecycle fact inventory | 完整；application 不直连 Run mutation，SM-1 与 `run.workspace_retirement` 各自只在 Run private reducer |
| `blocks/memory` | Run → Memory | activation recall trace upsert | 完整；观测失败不回滚 Run |
| `application/workorchd` | 双向只读 | typed Run/Step/Checkpoint query、`RunStateChangedData` | 完整；不提交 lifecycle、删除或 Runtime CAS |
| `kernel/governance` / `blocks/eventstream` | Run → consumer（Governance 另有只读 query） | typed Run query 与 `RunStateChangedData` | 完整；无 service/repository 注入 |

## 相关约束

- Run、Step 和 Checkpoint 操作必须携带足够的 workspace/run/step scope。
- 生产装配只接收 Run 自身 repository 与 EventHub，不得注入 WorkspaceReader、Orchestrator service、callback 或其他 owner repository。
- `Stop`/`Teardown` 必须逆序、幂等地解除 recovery inventory 与常规 topic；recovery inventory 不得在 owner 停止后继续响应。
- Run 不负责 coverage、evidence quality、Result acceptance 或 LLM 输出正确性判断。
- current-baseline restart 由 SM-1 唯一 Run owner 的 subordinate `run.replacement` transaction 先在非终态 predecessor
  上 CAS 持久化 immutable `ReplacementIntent`；只有
  predecessor 的 `Result -> Workspace -> Run` terminal delivery 完成后，Run-private reducer 才幂等创建并激活
  successor。相同 RequestID 只有在完整 sealed activation payload 相同的情况下才是 duplicate；否则 fail-closed。
  Workspace 已无 non-terminal Run 时，同一 Restart 根据 Run-owned `WorkspaceRunGeneration` 选择最新 terminal predecessor；
  它必须已有 exact `TerminalDelivery` 与由 Result acknowledgement 封存的 `TerminalResultID`。同一 replacement transaction
  直接从 `termination_acknowledged/successor_materialization` 继续，禁止 terminal→active 或重复 terminal delivery。
  request fact 不携带 `State` 或 mutable progress；唯一 private-event dispatcher 根据 clarification binding 选择
  `pending` 或 `pending_clarification_resolution`，termination claim/ack/failure、resolution acknowledgement、successor
  materialization 与 lineage/activation 全部经同一 CAS reducer 推进，禁止 generic mutate callback。
  successor create、predecessor lineage CAS 和 successor activation 是可独立重放的持久化切点；successor 必须同时封存
  `PredecessorRunID + PredecessorResultID`，predecessor 必须先 CAS 封存 matching `SuccessorRunID` 与 superseded lineage，
  才能激活 successor。任一切点后重启都只收敛到同一 successor。无法按当前结构读取的 predecessor 在 admission 前失败，
  且不持久化 replacement intent。
  Run owner 以 Workspace-scoped private mutex 串行 restart admission 与 successor materialization；任何未完成
  replacement transaction 对该 Workspace 独占，同 RequestID 只做 exact replay，不同 RequestID fail-closed。repository
  在相同 Workspace identity 临界区内再原子拒绝第二个 non-terminal Run，形成 owner workflow 与 persistence 双 fence。
- `ReplacementIntent.Recovery` 是 Run owner-private durable successor：stage 只允许 clarification resolution、administrative
  termination 与 successor materialization，state 只允许 scheduled/running/exhausted/completed，并绑定 exact intent hash、
  generation、attempt、dueAt。每次 side-effect 前先 CAS claim；失败在同一 owner reducer 中续排下一 generation。进程内
  BackgroundRoutine 只唤醒 persisted lease，不保存权威 attempt/target。每个唤醒都绑定 Run owner lifecycle context/epoch；
  `Stop` 必须先禁止新挂载、取消并排空旧 epoch 的 callback，再解除 topic 和 repository 依赖。fresh owner 只能从 Run repository
  重挂同一 persisted lease，旧 callback 不能跨生命周期执行或删除新实例的 wakeup。
  其他组件不得替 Run 建 retry timer、补 successor 或按投影状态重放 replacement。
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
- Orchestrator opaque record 的 terminal-delivery counter/ack 只在 sealed terminal 或 administrative termination 合法；
  nonterminal aggregate 携带该 ledger 时不属于当前持久化结构，必须由当前 invariant 校验使 owner load 直接失败。
  不再为该旧形态定义兼容分类或升级结果；Run/Workorchd 不得归零、隐藏、升级或把它当 repair budget，运维侧清理数据后重新创建。
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
  Workspace-owner execution selection、Orchestrator governance/continuation/terminal fact，以及本 owner
  query/cleanup/CAS；禁止外部 Block/Fail/Complete 目标 state setter。
- Run biz 不再持有或构造 Orchestrator adapter；已删除仅用于 禁止的 lifecycle 转发的 service DTO alias 和 test-only
  constructor。Run owner 的生产依赖只包含自身 repository、EventHub、EventHub-backed Workspace typed read port 与
  状态通知能力。

## 回归覆盖

已覆盖：SM-1 全状态合法/非法边矩阵（含 administrative cancel/supersede、终态反向迁移与未知状态拒绝）；Run creation/replacement successor 共用 private dispatcher、caller-selected initial state 与直达 repository Create 拒绝；durable Workspace head 的并发分配、watermark-first gap、delete/purge/reopen 后 RunID/generation 不复用、独立 repository set 串行与 single-live guard；terminal predecessor 按 owner generation 选择、terminal Result lineage 必填、零 lifecycle reopen/重复 delivery；ticket/approval
两类 wait/continuation、terminal stale/duplicate、fresh-activation；restart RequestID sealed-payload 冲突、intent 先持久化、
runtime clarification 同 seal/仅单项 lineage 变化的 typed 零写入拒绝、两项均变化的 acknowledgement gate，以及
terminal→successor create→lineage CAS→activation 各持久化切点的崩溃恢复与
唯一 successor；普通 create→activation cut 的 fresh-owner Setup 前自动恢复、全量校验后 CAS、same-request
pending/active replay、typed continuation 补发、different-request pending conflict 与重复恢复零新 generation；Runtime CAS source/scope/hash、aggregate revision race、物理 rebase 与 retired-field rejection；Setup 阶段
inventory/record read、三条 SM-4 read forged-source 拒绝、draining 可读与 retired exact-record 隐藏、commands-ready 元数据与重复 handoff、typed result clone、workspace scope；activation Memory 完整
selection fact、missing subscriber/owner error 不回滚、Run 启动补写与 Memory owner exact replay、in-flight Stop 与重复
Start/Stop。当前无本 README 已知可执行缺口。
