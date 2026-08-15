# Run Block-class Owner

## 组件类型

`blocks/run` 按职责属于 Block：它只拥有 Run lifecycle 聚合，并为 Orchestrator-owned Runtime aggregate 提供
opaque `RuntimeStateRecord` 物理 CAS/查询合同；不解释 SM-2/3/4 语义，也不组合多个 owner 选择下一动作。目录、module ID 和 EventHub
合同身份均为 `blocks/run`。

## 功能说明

`blocks/run` 管理 Run、Step、RuntimeCheckpoint 的正式状态，以及 Runtime aggregate 的 opaque 物理持久化、
expected-revision CAS 和查询合同。Runtime 的 reducer、invariant、candidate、dispatch、evidence、recovery 与
terminal seal 均由 Orchestrator 负责；Run 不承担跨组件编排。

## 组件交互及合同

- Run owner 通过 `TopicRunCommandRuntimeRecoveryInventory` 提供分页 typed recovery inventory；item 只包含本
  owner 的 SM-1 state、bootstrap/state presence、opaque revision/ref、实际 persisted schema 和 Run-owned
  administrative disposition，不读取 dispatch/governance/delivery ledger 做 SM-2/3/4 分类。该窄只读 topic，以及
  `TopicRunCommandRuntimeStateRecord` 和 readiness query，在 `Setup`（Configure 后）即订阅，早于常规 Run
  CRUD topic；state-record query 只能按 workspace/run 与可选的 opaque ref/revision 返回克隆的 record 或明确缺失，
  不解释其 aggregate 语义。Orchestrator 通过该 typed record query load 后在 owner-local SM-4 分类 recovery case，
  不直接扫描 Run repository。

- 常规 Run command subscription 完成后，Run owner 发布一次 `TopicRunRuntimeCommandsReady` process-lifecycle
  fact。该 fact 不携带 workspace/run，也不迁移 SM-1；它只释放 Orchestrator 已同步完成 inventory/load 的
  owner-local startup recovery actions。这样 recovery 不依赖 `Weight()`，且不会在 `StoreRuntimeStateRecordCommand` 尚未注册时
  由 BackgroundRoutine 提前驱动。

- Run 自身的 create/ensure/get/list/update/delete、Step 查询和 Checkpoint 查询由 `blocks/run/pkg/events` 定义具体 command 与 `RunCommandResult`；所有返回实体在离开 owner 前显式深拷贝。
- 常规命令保持唯一入口 `handleCommand -> executeCommand`，并在同一次 `commandMu` 读锁与 workspace scope
  fence 内，静态穷尽地委派到 create、Run query、Step query、Checkpoint query、opaque Runtime state store 和
  resource cleanup 六个 owner-private handler。它们不构成新的 Owner、状态机、CAS authority 或 EventHub facade。
- Run create 只接收 predecessor Run/Result 引用，不接收完整 `RunLineage`。Run owner 自行派生 active lineage state
  并清空 successor；调用方不能选择 owner 私有 lineage 状态。
- workspace/run purge 通过 `PurgeRuntimeResourcesCommand` 由 Run owner 删除该 scope 下的 Step 与 RuntimeCheckpoint；命令必须先确认 Run 属于指定 workspace，并返回实际删除数量。
- Runtime 正式状态使用 typed `RuntimeStateRecord` 随 Run 物理持久化；update command 必须携带 expected runtime
  revision，并返回唯一的 `accepted`、`aggregate_revision_mismatch`、`semantic_rejection` 或 `owner_transient`
  typed fact。只有 expected/observed aggregate revision 实际不匹配才是 CAS；Run 对无关物理 record write 自行
  rebase。Run 不解释其中的 facts、authority、candidate、dispatch、evidence 或 governance ledger。
- `StoreRuntimeStateRecordCommand` 不是完整 Run entity replacement；它只允许 `kernel/orchestrator` 提交 opaque
  `RuntimeStateRecord` 与 expected aggregate revision。Run owner 必须在进入 CAS 前验证 EventHub source，其他 caller
  即使构造出合法 record 也必须被拒绝。
- Run 创建合同可携带 typed `RuntimeBootstrap`，保存 effective Goal/DoD definition、ordered focus queue 与 prerequisite requirements；bootstrap 在 RuntimeState 初始化前保持不可变。
- 当 bootstrap 含有 activation 选择的 Memory context 时，Run owner 在 Run record 成功持久化后向 `blocks/memory` 发送一次 typed `UpsertMemoryRecallTraceCommand`。该消息只是不可变的 activation fact，Memory owner 独占 RecallTrace 持久化与 `MemoryRecallRecorded` 通知；固定 trace ID 使重复创建/恢复幂等，memory observability 失败不得回滚已成功的 Run 创建。
- 创建或启动 Run 前，通过 EventHub 读取 `blocks/workspace` 的有效 workspace definition 和 revision。
- 启动和恢复由 application 向 Run owner发送具体 activation / continuation intent；Run Biz 在自身 reducer 中完成状态准入、
  CAS 持久化并发布 `RunStateChangedData`。Orchestrator 只能消费该通知触发 drive，不能投影 Run lifecycle。
  Runtime Resume 只携带 workspace/run scope，不接受 loaded Run 或上下文 patch。
- Orchestrator 发现不支持的 persisted Runtime schema 时，只能发送带 scope、opaque state ref、actual/supported
  schema 和 safe original diagnostic 的 `RuntimeStateIncompatibleFact`。Run reducer 将它持久化为
  `fresh_activation_required`；该 Run 的 `ContinueRun` 返回 typed non-resumable disposition，既不发 continuation
  fact，也不触发 drive。只有用户显式 fresh activation/restart 才可创建 current-schema Run。
- governed blocker 必须携带对应 DecisionTicket 或 Approval 的 workspace/run scope。
- Run 创建及状态变化使用 `RunStateChangedData` typed notification；通知只包含 SM-1 state、causation 与 opaque
  RuntimeState revision/ref，不镜像 Orchestrator action/focus/repair。schema v8 Runtime record 同时持久化 Orchestrator-authored
  opaque lifecycle fence；Run 仅比较 scope/schema/revision/ref/hash/bootstrap/fence，禁止解码 Decision、semantic、candidate、
  governance、evidence 或 drive 内容。若为 `fresh_activation_required`，通知额外携带
  Run-owned actual/supported schema identity，以便 Workspace 安全释放同一 Run 的 active slot；它仍不公开 aggregate
  内容或选择 recovery action。Governance、Eventstream 和 application 按需订阅。
- Governance、Eventstream 和 application 按需导入 `blocks/run/pkg/events` 订阅或发送命令。

## 相关约束

- Run、Step 和 Checkpoint 操作必须携带足够的 workspace/run/step scope。
- 生产装配只接收 Run 自身 repository 与 EventHub，不得注入 WorkspaceReader、Orchestrator service、callback 或其他 owner repository。
- `Stop`/`Teardown` 必须逆序、幂等地解除 recovery inventory 与常规 topic；recovery inventory 不得在 owner 停止后继续响应。
- Run 不负责 coverage、evidence quality、Result acceptance 或 LLM 输出正确性判断。
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
  payload 原样 CAS 持久化；其合法性、去重与状态迁移只由 Orchestrator owner reducer/invariant 负责。
- RuntimeBootstrap 的顶层 definition identity/revision 必须与 Run 的 GoalID、DoDID 和 DefinitionRevision 一致；
  不一致的 create/ensure 必须返回 typed semantic rejection。RuntimeStateRecord 内部 definition context、facts 与
  activation 的完整性只由 Orchestrator invariant 校验，Run owner 不据此选择 CAS/recovery/lifecycle 路径。
- Resume 的状态准入与迁移完全由 Run owner reducer 完成；成功后才发布不可变 Run state fact。不得通过 Run command
  传递 `map[string]any`、指定外部 owner state 或调用 Orchestrator service facade。
- Run 不提供 Validate/Complete service command；禁止 application 或其他 module 绕过 Runtime reducer/evidence gate 直接把 active Run 标记为 validated/completed。
- Run owner handler 直接调用本 owner `biz`；不保留 `Service()` 或 `service/` forwarding facade。跨组件只接受
  activation/continuation/cancel/supersede intent、governance continuation fact 或 sealed terminal outcome；禁止外部
  Block/Fail/Complete 目标 state setter。
- Run biz 不再持有或构造 Orchestrator adapter；已删除仅用于 禁止的 lifecycle 转发的 service DTO alias 和 test-only
  constructor。Run owner 的生产依赖只包含自身 repository、EventHub、EventHub-backed Workspace typed read port 与
  状态通知能力。
